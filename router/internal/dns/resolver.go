package dns

import (
	"log/slog"
	"net"
	"os"
	"time"
)

const defaultResolver = "127.0.0.11:53"

type Resolver struct {
	address string
	timeout time.Duration
}

func NewResolver() *Resolver {
	address := os.Getenv("SUISOU_DNS_RESOLVER")
	if address == "" {
		address = defaultResolver
	}
	return &Resolver{
		address: address,
		timeout: 5 * time.Second,
	}
}

func (r *Resolver) Address() string {
	return r.address
}

func (r *Resolver) Forward(payload []byte) []byte {
	conn, err := net.DialTimeout("udp", r.address, r.timeout)
	if err != nil {
		slog.Error("DNS dial failed", "resolver", r.address, "err", err)
		return nil
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(r.timeout)); err != nil {
		slog.Error("DNS deadline failed", "resolver", r.address, "err", err)
		return nil
	}
	if _, err := conn.Write(payload); err != nil {
		slog.Error("DNS write failed", "resolver", r.address, "err", err)
		return nil
	}

	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		slog.Error("DNS read failed", "resolver", r.address, "err", err)
		return nil
	}
	return buf[:n]
}
