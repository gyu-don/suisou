package main

import (
	"log/slog"
	"net"
	"time"
)

func forwardDNS(payload []byte, srcAddr net.Addr) []byte {
	conn, err := net.DialTimeout("udp", systemResolver(), 5*time.Second)
	if err != nil {
		slog.Error("DNS dial failed", "err", err)
		return nil
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Second))

	if _, err := conn.Write(payload); err != nil {
		slog.Error("DNS write failed", "err", err)
		return nil
	}

	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		slog.Error("DNS read failed", "err", err)
		return nil
	}
	return buf[:n]
}

func systemResolver() string {
	// In Docker, the embedded DNS is at 127.0.0.11
	// Fall back to Google DNS
	addrs := []string{"127.0.0.11:53", "8.8.8.8:53"}
	for _, addr := range addrs {
		conn, err := net.DialTimeout("udp", addr, 500*time.Millisecond)
		if err == nil {
			conn.Close()
			return addr
		}
	}
	return "8.8.8.8:53"
}
