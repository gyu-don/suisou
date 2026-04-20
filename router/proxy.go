package main

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"
)

type Proxy struct {
	policy    *Policy
	certStore *CertStore
	transport *http.Transport
}

func NewProxy(policy *Policy, certStore *CertStore) *Proxy {
	return &Proxy{
		policy:    policy,
		certStore: certStore,
		transport: &http.Transport{
			TLSClientConfig:     &tls.Config{},
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
		},
	}
}

func (p *Proxy) HandleTCP(clientConn net.Conn, dstAddr net.TCPAddr) {
	defer clientConn.Close()

	br := bufio.NewReader(clientConn)
	first, err := br.Peek(1)
	if err != nil {
		return
	}

	if first[0] == 0x16 {
		p.handleTLS(newBufferedConn(clientConn, br), dstAddr)
	} else {
		p.handlePlainHTTP(newBufferedConn(clientConn, br), dstAddr)
	}
}

func (p *Proxy) handleTLS(clientConn net.Conn, dstAddr net.TCPAddr) {
	tlsConfig := p.certStore.TLSConfig()
	tlsConfig.NextProtos = []string{"http/1.1"}
	tlsConn := tls.Server(clientConn, tlsConfig)
	if err := tlsConn.Handshake(); err != nil {
		slog.Debug("TLS handshake failed", "err", err)
		return
	}
	defer tlsConn.Close()

	sni := tlsConn.ConnectionState().ServerName
	p.serveHTTP(tlsConn, dstAddr, true, sni)
}

func (p *Proxy) handlePlainHTTP(clientConn net.Conn, dstAddr net.TCPAddr) {
	p.serveHTTP(clientConn, dstAddr, false, "")
}

func (p *Proxy) serveHTTP(clientConn net.Conn, dstAddr net.TCPAddr, isTLS bool, sni string) {
	br := bufio.NewReader(clientConn)

	for {
		req, err := http.ReadRequest(br)
		if err != nil {
			return
		}

		host := req.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}

		port := dstAddr.Port
		if isTLS && port == 0 {
			port = 443
		} else if !isTLS && port == 0 {
			port = 80
		}

		// Host/SNI validation for TLS
		if isTLS && sni != "" && sni != host {
			slog.Warn("Host/SNI mismatch", "host", host, "sni", sni)
			writeHTTPError(clientConn, req, 403, "Blocked by suisou: untrusted Host header")
			return
		}

		// Plain HTTP validation
		if !isTLS && !p.policy.PlainHTTPAllowed(host) {
			slog.Warn("plain HTTP not allowed", "host", host)
			writeHTTPError(clientConn, req, 403, "Blocked by suisou: untrusted Host header")
			return
		}

		method := strings.ToUpper(req.Method)
		path := req.URL.Path
		if path == "" {
			path = "/"
		}

		target := fmt.Sprintf("%s:%d%s", host, port, path)

		if !p.policy.EndpointAllowed(host, method, path, port) {
			slog.Warn("blocked request", "method", method, "target", target)
			writeHTTPError(clientConn, req, 403,
				fmt.Sprintf("Blocked by suisou allowlist: %s %s:%d", method, host, port))
			return
		}

		slog.Info("allowed request", "method", method, "target", target)

		p.policy.InjectCredentials(req, host)

		// WebSocket upgrade
		if isWebSocketUpgrade(req) {
			p.handleWebSocketUpgrade(clientConn, req, host, port, isTLS)
			return
		}

		scheme := "http"
		if isTLS {
			scheme = "https"
		}
		req.URL.Scheme = scheme
		req.URL.Host = fmt.Sprintf("%s:%d", host, port)
		req.RequestURI = ""

		resp, err := p.transport.RoundTrip(req)
		if err != nil {
			slog.Error("upstream error", "err", err, "target", target)
			writeHTTPError(clientConn, req, 502, "Bad Gateway")
			return
		}

		keepAlive := resp.ProtoAtLeast(1, 1) && !resp.Close &&
			!strings.EqualFold(resp.Header.Get("Connection"), "close")

		if err := resp.Write(clientConn); err != nil {
			resp.Body.Close()
			return
		}
		resp.Body.Close()

		if !keepAlive {
			return
		}
	}
}

func (p *Proxy) handleWebSocketUpgrade(clientConn net.Conn, req *http.Request, host string, port int, isTLS bool) {
	scheme := "ws"
	dialScheme := "tcp"
	if isTLS {
		scheme = "wss"
		_ = scheme
	}

	addr := fmt.Sprintf("%s:%d", host, port)
	upstreamConn, err := net.DialTimeout(dialScheme, addr, 10*time.Second)
	if err != nil {
		slog.Error("websocket upstream dial failed", "err", err, "addr", addr)
		writeHTTPError(clientConn, req, 502, "Bad Gateway")
		return
	}
	defer upstreamConn.Close()

	if isTLS {
		tlsConn := tls.Client(upstreamConn, &tls.Config{ServerName: host})
		if err := tlsConn.Handshake(); err != nil {
			slog.Error("websocket upstream TLS failed", "err", err)
			writeHTTPError(clientConn, req, 502, "Bad Gateway")
			return
		}
		upstreamConn = tlsConn
	}

	req.URL.Host = addr
	req.URL.Scheme = "http"
	req.RequestURI = ""
	if err := req.Write(upstreamConn); err != nil {
		return
	}

	upBr := bufio.NewReader(upstreamConn)
	resp, err := http.ReadResponse(upBr, req)
	if err != nil {
		return
	}

	if err := resp.Write(clientConn); err != nil {
		return
	}

	if resp.StatusCode != 101 {
		return
	}

	slog.Info("websocket opened", "host", host)
	relayWebSocket(clientConn, upstreamConn, host, p.policy)
}

func isWebSocketUpgrade(req *http.Request) bool {
	return strings.EqualFold(req.Header.Get("Connection"), "upgrade") &&
		strings.EqualFold(req.Header.Get("Upgrade"), "websocket")
}

func writeHTTPError(w io.Writer, req *http.Request, status int, msg string) {
	resp := &http.Response{
		StatusCode: status,
		Status:     fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Proto:      req.Proto,
		ProtoMajor: req.ProtoMajor,
		ProtoMinor: req.ProtoMinor,
		Header:     http.Header{"Content-Type": {"text/plain"}, "Connection": {"close"}},
		Body:       io.NopCloser(strings.NewReader(msg)),
	}
	resp.ContentLength = int64(len(msg))
	resp.Write(w)
}

type bufferedConn struct {
	net.Conn
	br *bufio.Reader
}

func newBufferedConn(conn net.Conn, br *bufio.Reader) *bufferedConn {
	return &bufferedConn{Conn: conn, br: br}
}

func (c *bufferedConn) Read(b []byte) (int, error) {
	return c.br.Read(b)
}
