package proxy

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

	"suisou/router/internal/policy"
)

type Proxy struct {
	policy    *policy.Policy
	certStore *CertStore
	transport *http.Transport
}

func New(policy *policy.Policy, certStore *CertStore) *Proxy {
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

	buffered := newBufferedConn(clientConn, br)
	if first[0] == 0x16 {
		p.handleTLS(buffered, dstAddr)
		return
	}
	p.handlePlainHTTP(buffered, dstAddr)
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

		host := requestHost(req)
		port := requestPort(dstAddr.Port, isTLS)

		if isTLS && sni != "" && sni != host {
			slog.Warn("Host/SNI mismatch", "host", host, "sni", sni)
			writeHTTPError(clientConn, req, http.StatusForbidden, "Blocked by suisou: untrusted Host header")
			return
		}
		if !isTLS && !p.policy.PlainHTTPAllowed(host) {
			slog.Warn("plain HTTP not allowed", "host", host)
			writeHTTPError(clientConn, req, http.StatusForbidden, "Blocked by suisou: untrusted Host header")
			return
		}

		method := strings.ToUpper(req.Method)
		path := requestPath(req)
		target := fmt.Sprintf("%s:%d%s", host, port, path)
		if !p.policy.EndpointAllowed(host, method, path, port) {
			slog.Warn("blocked request", "method", method, "target", target)
			writeHTTPError(clientConn, req, http.StatusForbidden, fmt.Sprintf("Blocked by suisou allowlist: %s %s:%d", method, host, port))
			return
		}

		slog.Info("allowed request", "method", method, "target", target)
		p.policy.InjectCredentials(req, host)

		if isWebSocketUpgrade(req) {
			p.handleWebSocketUpgrade(clientConn, req, host, port, isTLS)
			return
		}

		req.URL.Scheme = "http"
		if isTLS {
			req.URL.Scheme = "https"
		}
		req.URL.Host = fmt.Sprintf("%s:%d", host, port)
		req.RequestURI = ""

		resp, err := p.transport.RoundTrip(req)
		if err != nil {
			slog.Error("upstream error", "err", err, "target", target)
			writeHTTPError(clientConn, req, http.StatusBadGateway, "Bad Gateway")
			return
		}

		keepAlive := resp.ProtoAtLeast(1, 1) && !resp.Close && !headerContainsToken(resp.Header, "Connection", "close")
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
	addr := fmt.Sprintf("%s:%d", host, port)
	upstreamConn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		slog.Error("websocket upstream dial failed", "err", err, "addr", addr)
		writeHTTPError(clientConn, req, http.StatusBadGateway, "Bad Gateway")
		return
	}
	defer upstreamConn.Close()

	if isTLS {
		tlsConn := tls.Client(upstreamConn, &tls.Config{ServerName: host})
		if err := tlsConn.Handshake(); err != nil {
			slog.Error("websocket upstream TLS failed", "err", err)
			writeHTTPError(clientConn, req, http.StatusBadGateway, "Bad Gateway")
			return
		}
		upstreamConn = tlsConn
	}

	req.URL.Host = addr
	req.URL.Scheme = "http"
	if isTLS {
		req.URL.Scheme = "https"
	}
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
	if resp.StatusCode != http.StatusSwitchingProtocols {
		return
	}

	slog.Info("websocket opened", "host", host)
	relayWebSocket(newBufferedConn(upstreamConn, upBr), clientConn, host, p.policy)
}

func isWebSocketUpgrade(req *http.Request) bool {
	return headerContainsToken(req.Header, "Connection", "upgrade") &&
		headerContainsToken(req.Header, "Upgrade", "websocket")
}

func headerContainsToken(header http.Header, key, want string) bool {
	for _, raw := range header.Values(key) {
		for _, token := range strings.Split(raw, ",") {
			if strings.EqualFold(strings.TrimSpace(token), want) {
				return true
			}
		}
	}
	return false
}

func requestHost(req *http.Request) string {
	host := req.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		return h
	}
	return host
}

func requestPath(req *http.Request) string {
	path := req.URL.RequestURI()
	if path == "" {
		return "/"
	}
	return path
}

func requestPort(port int, isTLS bool) int {
	if port != 0 {
		return port
	}
	if isTLS {
		return 443
	}
	return 80
}

func writeHTTPError(w io.Writer, req *http.Request, status int, msg string) {
	resp := &http.Response{
		StatusCode:    status,
		Status:        fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Proto:         req.Proto,
		ProtoMajor:    req.ProtoMajor,
		ProtoMinor:    req.ProtoMinor,
		ContentLength: int64(len(msg)),
		Header:        http.Header{"Content-Type": {"text/plain"}, "Connection": {"close"}},
		Body:          io.NopCloser(strings.NewReader(msg)),
	}
	_ = resp.Write(w)
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
