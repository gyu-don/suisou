package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
)

const (
	wsOpText  = 1
	wsOpClose = 8
)

type wsStats struct {
	clientMessages   int
	serverMessages   int
	injectedMessages int
}

func relayWebSocket(client, upstream net.Conn, host string, policy *Policy) {
	allowedEnvs := policy.AllowedEnvsForHost(host)

	var stats wsStats
	var wg sync.WaitGroup
	wg.Add(2)

	// client → upstream (with credential injection)
	go func() {
		defer wg.Done()
		for {
			opcode, payload, err := readWSFrame(client)
			if err != nil {
				upstream.Close()
				return
			}
			stats.clientMessages++

			if opcode == wsOpText && strings.Contains(string(payload), dummyPrefix) {
				if json.Valid(payload) {
					replaced := policy.ReplaceMarkers(string(payload), allowedEnvs)
					if replaced != string(payload) {
						payload = []byte(replaced)
						stats.injectedMessages++
					}
				}
			}

			if err := writeWSFrame(upstream, opcode, payload, false); err != nil {
				return
			}
		}
	}()

	// upstream → client
	go func() {
		defer wg.Done()
		for {
			opcode, payload, err := readWSFrame(upstream)
			if err != nil {
				client.Close()
				return
			}
			stats.serverMessages++

			if err := writeWSFrame(client, opcode, payload, true); err != nil {
				return
			}
		}
	}()

	wg.Wait()
	slog.Info("websocket closed",
		"host", host,
		"client_messages", stats.clientMessages,
		"server_messages", stats.serverMessages,
		"injected_messages", stats.injectedMessages,
	)
}

func readWSFrame(r io.Reader) (opcode byte, payload []byte, err error) {
	header := make([]byte, 2)
	if _, err = io.ReadFull(r, header); err != nil {
		return 0, nil, err
	}

	opcode = header[0] & 0x0f
	masked := header[1]&0x80 != 0
	length := uint64(header[1] & 0x7f)

	switch {
	case length == 126:
		ext := make([]byte, 2)
		if _, err = io.ReadFull(r, ext); err != nil {
			return 0, nil, err
		}
		length = uint64(binary.BigEndian.Uint16(ext))
	case length == 127:
		ext := make([]byte, 8)
		if _, err = io.ReadFull(r, ext); err != nil {
			return 0, nil, err
		}
		length = binary.BigEndian.Uint64(ext)
	}

	var mask [4]byte
	if masked {
		if _, err = io.ReadFull(r, mask[:]); err != nil {
			return 0, nil, err
		}
	}

	if length > 16*1024*1024 {
		return 0, nil, fmt.Errorf("websocket frame too large: %d", length)
	}

	payload = make([]byte, length)
	if _, err = io.ReadFull(r, payload); err != nil {
		return 0, nil, err
	}

	if masked {
		for i := range payload {
			payload[i] ^= mask[i%4]
		}
	}

	return opcode, payload, nil
}

func writeWSFrame(w io.Writer, opcode byte, payload []byte, masked bool) error {
	var header []byte
	fin := byte(0x80)

	plen := len(payload)
	switch {
	case plen <= 125:
		header = make([]byte, 2)
		header[0] = fin | opcode
		header[1] = byte(plen)
	case plen <= 65535:
		header = make([]byte, 4)
		header[0] = fin | opcode
		header[1] = 126
		binary.BigEndian.PutUint16(header[2:], uint16(plen))
	default:
		header = make([]byte, 10)
		header[0] = fin | opcode
		header[1] = 127
		binary.BigEndian.PutUint64(header[2:], uint64(plen))
	}

	if masked {
		header[1] |= 0x80
		mask := [4]byte{0, 0, 0, 0} // zero mask for simplicity on server→client
		header = append(header, mask[:]...)
		data := make([]byte, len(payload))
		copy(data, payload)
		// zero mask = no-op XOR
		payload = data
	}

	if _, err := w.Write(header); err != nil {
		return err
	}
	if _, err := w.Write(payload); err != nil {
		return err
	}
	return nil
}
