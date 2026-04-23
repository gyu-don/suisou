package main

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteWireGuardConfExposesOnlyServerPublicKey(t *testing.T) {
	dir := t.TempDir()

	serverPriv, serverPub := generateKey()
	clientPriv, _ := generateKey()

	if err := writeWireGuardConf(dir, serverPub, clientPriv); err != nil {
		t.Fatalf("writeWireGuardConf: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "wireguard.conf"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	var conf map[string]string
	if err := json.Unmarshal(data, &conf); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if _, ok := conf["server_key"]; ok {
		t.Fatal("server private key should not be exposed")
	}
	if got := conf["server_public_key"]; got != base64.StdEncoding.EncodeToString(serverPub[:]) {
		t.Fatalf("server_public_key = %q, want %q", got, base64.StdEncoding.EncodeToString(serverPub[:]))
	}
	if got := conf["client_private_key"]; got != base64.StdEncoding.EncodeToString(clientPriv[:]) {
		t.Fatalf("client_private_key = %q, want %q", got, base64.StdEncoding.EncodeToString(clientPriv[:]))
	}
	if got := conf["server_public_key"]; got == base64.StdEncoding.EncodeToString(serverPriv[:]) {
		t.Fatal("server_public_key unexpectedly contains private key bytes")
	}
}
