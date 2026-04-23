package proxy

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"sync/atomic"

	"suisou/router/internal/policy"
)

const wsOpText = 1

type wsStats struct {
	clientMessages   atomic.Int64
	serverMessages   atomic.Int64
	injectedMessages atomic.Int64
}

type wsFrame struct {
	firstByte byte
	payload   []byte
}

func relayWebSocket(upstream, client net.Conn, host string, p *policy.Policy) {
	allowedEnvs := p.AllowedEnvsForHost(host)

	var stats wsStats
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for {
			frame, err := readWSFrame(client)
			if err != nil {
				_ = upstream.Close()
				return
			}
			stats.clientMessages.Add(1)

			if frame.firstByte&0x0f == wsOpText && strings.Contains(string(frame.payload), policy.DummyPrefix) && json.Valid(frame.payload) {
				replaced := p.ReplaceMarkers(string(frame.payload), allowedEnvs)
				if replaced != string(frame.payload) {
					frame.payload = []byte(replaced)
					stats.injectedMessages.Add(1)
				}
			}

			if err := writeWSFrame(upstream, frame, true); err != nil {
				return
			}
		}
	}()

	go func() {
		defer wg.Done()
		for {
			frame, err := readWSFrame(upstream)
			if err != nil {
				_ = client.Close()
				return
			}
			stats.serverMessages.Add(1)

			if err := writeWSFrame(client, frame, false); err != nil {
				return
			}
		}
	}()

	wg.Wait()
	slog.Info("websocket closed",
		"host", host,
		"client_messages", stats.clientMessages.Load(),
		"server_messages", stats.serverMessages.Load(),
		"injected_messages", stats.injectedMessages.Load(),
	)
}

func readWSFrame(r io.Reader) (wsFrame, error) {
	header := make([]byte, 2)
	if _, err := io.ReadFull(r, header); err != nil {
		return wsFrame{}, err
	}

	payloadLen := uint64(header[1] & 0x7f)
	switch payloadLen {
	case 126:
		ext := make([]byte, 2)
		if _, err := io.ReadFull(r, ext); err != nil {
			return wsFrame{}, err
		}
		payloadLen = uint64(binary.BigEndian.Uint16(ext))
	case 127:
		ext := make([]byte, 8)
		if _, err := io.ReadFull(r, ext); err != nil {
			return wsFrame{}, err
		}
		payloadLen = binary.BigEndian.Uint64(ext)
	}

	var mask [4]byte
	masked := header[1]&0x80 != 0
	if masked {
		if _, err := io.ReadFull(r, mask[:]); err != nil {
			return wsFrame{}, err
		}
	}

	if payloadLen > 16*1024*1024 {
		return wsFrame{}, fmt.Errorf("websocket frame too large: %d", payloadLen)
	}

	payload := make([]byte, payloadLen)
	if _, err := io.ReadFull(r, payload); err != nil {
		return wsFrame{}, err
	}
	if masked {
		applyMask(payload, mask)
	}

	return wsFrame{firstByte: header[0], payload: payload}, nil
}

func writeWSFrame(w io.Writer, frame wsFrame, masked bool) error {
	header := make([]byte, 0, 14)
	header = append(header, frame.firstByte)

	payloadLen := len(frame.payload)
	switch {
	case payloadLen <= 125:
		header = append(header, byte(payloadLen))
	case payloadLen <= 65535:
		header = append(header, 126)
		ext := make([]byte, 2)
		binary.BigEndian.PutUint16(ext, uint16(payloadLen))
		header = append(header, ext...)
	default:
		header = append(header, 127)
		ext := make([]byte, 8)
		binary.BigEndian.PutUint64(ext, uint64(payloadLen))
		header = append(header, ext...)
	}

	payload := frame.payload
	if masked {
		header[1] |= 0x80
		var mask [4]byte
		if _, err := rand.Read(mask[:]); err != nil {
			return err
		}
		header = append(header, mask[:]...)
		payload = append([]byte(nil), frame.payload...)
		applyMask(payload, mask)
	}

	if _, err := w.Write(header); err != nil {
		return err
	}
	_, err := w.Write(payload)
	return err
}

func applyMask(payload []byte, mask [4]byte) {
	for i := range payload {
		payload[i] ^= mask[i%4]
	}
}
