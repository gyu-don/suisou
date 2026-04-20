package main

import (
	"context"
	"os"
	"sync"

	"golang.zx2c4.com/wireguard/tun"
	"gvisor.dev/gvisor/pkg/buffer"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/header"
	"gvisor.dev/gvisor/pkg/tcpip/link/channel"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
)

type channelTUN struct {
	ep     *channel.Endpoint
	mtu    int
	events chan tun.Event
	closed chan struct{}
	once   sync.Once
}

func newChannelTUN(ep *channel.Endpoint, mtu int) *channelTUN {
	t := &channelTUN{
		ep:     ep,
		mtu:    mtu,
		events: make(chan tun.Event, 1),
		closed: make(chan struct{}),
	}
	t.events <- tun.EventUp
	return t
}

func (t *channelTUN) File() *os.File { return nil }

func (t *channelTUN) Read(bufs [][]byte, sizes []int, offset int) (int, error) {
	// Read packets from the gvisor stack (outgoing → wireguard)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		select {
		case <-t.closed:
			cancel()
		case <-ctx.Done():
		}
	}()
	defer cancel()

	pkt := t.ep.ReadContext(ctx)
	if pkt == nil {
		select {
		case <-t.closed:
			return 0, os.ErrClosed
		default:
			return 0, nil
		}
	}
	defer pkt.DecRef()

	buf := bufs[0][offset:]
	view := pkt.ToView()
	n, err := view.Read(buf)
	sizes[0] = n
	return 1, err
}

func (t *channelTUN) Write(bufs [][]byte, offset int) (int, error) {
	// Write packets from wireguard into the gvisor stack
	for i, buf := range bufs {
		if offset >= len(buf) {
			continue
		}
		data := buf[offset:]
		if len(data) == 0 {
			continue
		}

		// Determine network protocol from IP version
		proto := tcpip.NetworkProtocolNumber(0)
		version := data[0] >> 4
		switch version {
		case 4:
			proto = header.IPv4ProtocolNumber
		case 6:
			proto = header.IPv6ProtocolNumber
		default:
			continue
		}

		pkt := stack.NewPacketBuffer(stack.PacketBufferOptions{
			Payload: buffer.MakeWithData(data),
		})
		t.ep.InjectInbound(proto, pkt)
		pkt.DecRef()
		_ = i
	}
	return len(bufs), nil
}

func (t *channelTUN) MTU() (int, error) {
	return t.mtu, nil
}

func (t *channelTUN) Name() (string, error) {
	return "suisou0", nil
}

func (t *channelTUN) Events() <-chan tun.Event {
	return t.events
}

func (t *channelTUN) Close() error {
	t.once.Do(func() { close(t.closed) })
	return nil
}

func (t *channelTUN) BatchSize() int {
	return 1
}
