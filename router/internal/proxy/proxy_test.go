package proxy

import (
	"bufio"
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestIsWebSocketUpgradeWithTokenList(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "https://gateway.discord.gg", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Connection", "keep-alive, Upgrade")
	req.Header.Set("Upgrade", "websocket")

	if !isWebSocketUpgrade(req) {
		t.Fatal("expected websocket upgrade to be detected")
	}
}

func TestRequestPathPreservesQueryString(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "https://github.com/info/refs?service=git-upload-pack", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	if got, want := requestPath(req), "/info/refs?service=git-upload-pack"; got != want {
		t.Fatalf("requestPath() = %q, want %q", got, want)
	}
}

func TestHeaderContainsToken(t *testing.T) {
	header := http.Header{"Connection": []string{"keep-alive, Upgrade"}}
	if !headerContainsToken(header, "Connection", "upgrade") {
		t.Fatal("expected token lookup to be case-insensitive and comma aware")
	}
}

func TestWriteHTTPErrorIncludesBody(t *testing.T) {
	req := &http.Request{Proto: "HTTP/1.1", ProtoMajor: 1, ProtoMinor: 1}
	var buf bytes.Buffer
	writeHTTPError(&buf, req, http.StatusForbidden, "denied")

	resp, err := http.ReadResponse(bufio.NewReader(strings.NewReader(buf.String())), req)
	if err != nil {
		t.Fatalf("ReadResponse: %v", err)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if got := string(body); got != "denied" {
		t.Fatalf("body = %q, want %q", got, "denied")
	}
}
