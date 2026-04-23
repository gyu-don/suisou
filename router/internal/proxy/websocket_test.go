package proxy

import (
	"bytes"
	"testing"
)

func TestWriteWSFrameMasksClientPayload(t *testing.T) {
	frame := wsFrame{firstByte: 0x81, payload: []byte("hello")}
	var buf bytes.Buffer
	if err := writeWSFrame(&buf, frame, true); err != nil {
		t.Fatalf("writeWSFrame: %v", err)
	}

	written := buf.Bytes()
	if written[0] != frame.firstByte {
		t.Fatalf("first byte = %x, want %x", written[0], frame.firstByte)
	}
	if written[1]&0x80 == 0 {
		t.Fatal("expected client frame to be masked")
	}

	decoded, err := readWSFrame(bytes.NewReader(written))
	if err != nil {
		t.Fatalf("readWSFrame: %v", err)
	}
	if string(decoded.payload) != "hello" {
		t.Fatalf("payload = %q, want %q", string(decoded.payload), "hello")
	}
	if decoded.firstByte != frame.firstByte {
		t.Fatalf("decoded first byte = %x, want %x", decoded.firstByte, frame.firstByte)
	}
}

func TestWriteWSFrameLeavesServerPayloadUnmasked(t *testing.T) {
	frame := wsFrame{firstByte: 0x82, payload: []byte{1, 2, 3}}
	var buf bytes.Buffer
	if err := writeWSFrame(&buf, frame, false); err != nil {
		t.Fatalf("writeWSFrame: %v", err)
	}
	if got := buf.Bytes()[1] & 0x80; got != 0 {
		t.Fatalf("mask bit = %x, want 0", got)
	}

	decoded, err := readWSFrame(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("readWSFrame: %v", err)
	}
	if !bytes.Equal(decoded.payload, frame.payload) {
		t.Fatalf("payload = %v, want %v", decoded.payload, frame.payload)
	}
}
