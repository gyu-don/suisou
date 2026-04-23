package wgstack

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

type ChannelTUN struct {
	ep     *channel.Endpoint
	mtu    int
	events chan tun.Event
	ctx    context.Context
	cancel context.CancelFunc
	once   sync.Once
}

func NewChannelTUN(ep *channel.Endpoint, mtu int) *ChannelTUN {
	ctx, cancel := context.WithCancel(context.Background())
	t := &ChannelTUN{
		ep:     ep,
		mtu:    mtu,
		events: make(chan tun.Event, 1),
		ctx:    ctx,
		cancel: cancel,
	}
	t.events <- tun.EventUp
	return t
}

func (t *ChannelTUN) File() *os.File { return nil }

func (t *ChannelTUN) Read(bufs [][]byte, sizes []int, offset int) (int, error) {
	pkt := t.ep.ReadContext(t.ctx)
	if pkt == nil {
		select {
		case <-t.ctx.Done():
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

func (t *ChannelTUN) Write(bufs [][]byte, offset int) (int, error) {
	for _, buf := range bufs {
		if offset >= len(buf) {
			continue
		}
		data := buf[offset:]
		if len(data) == 0 {
			continue
		}

		var proto tcpip.NetworkProtocolNumber
		switch data[0] >> 4 {
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
	}
	return len(bufs), nil
}

func (t *ChannelTUN) MTU() (int, error) {
	return t.mtu, nil
}

func (t *ChannelTUN) Name() (string, error) {
	return "suisou0", nil
}

func (t *ChannelTUN) Events() <-chan tun.Event {
	return t.events
}

func (t *ChannelTUN) Close() error {
	t.once.Do(t.cancel)
	return nil
}

func (t *ChannelTUN) BatchSize() int {
	return 1
}
