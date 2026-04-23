package proxy

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type CertStore struct {
	caCert *x509.Certificate
	caKey  *ecdsa.PrivateKey

	mu    sync.Mutex
	cache map[string]*tls.Certificate
}

func NewCertStore(privateDir, publicDir string) (*CertStore, error) {
	certPath := filepath.Join(publicDir, "mitmproxy-ca-cert.pem")
	keyPath := filepath.Join(privateDir, "ca-key.pem")

	cs := &CertStore{cache: make(map[string]*tls.Certificate)}
	if err := cs.loadOrGenerateCA(certPath, keyPath); err != nil {
		return nil, err
	}
	return cs, nil
}

func (cs *CertStore) loadOrGenerateCA(certPath, keyPath string) error {
	certPEM, certErr := os.ReadFile(certPath)
	keyPEM, keyErr := os.ReadFile(keyPath)
	if certErr == nil && keyErr == nil {
		certBlock, _ := pem.Decode(certPEM)
		keyBlock, _ := pem.Decode(keyPEM)
		if certBlock != nil && keyBlock != nil {
			cert, certParseErr := x509.ParseCertificate(certBlock.Bytes)
			key, keyParseErr := x509.ParseECPrivateKey(keyBlock.Bytes)
			if certParseErr == nil && keyParseErr == nil {
				cs.caCert = cert
				cs.caKey = key
				return nil
			}
		}
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("generating CA key: %w", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return fmt.Errorf("generating CA serial: %w", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   "suisou CA",
			Organization: []string{"suisou"},
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            0,
	}
	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return fmt.Errorf("creating CA cert: %w", err)
	}
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return fmt.Errorf("parsing CA cert: %w", err)
	}

	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER}), 0o644); err != nil {
		return fmt.Errorf("writing CA cert: %w", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return fmt.Errorf("marshaling CA key: %w", err)
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		return fmt.Errorf("writing CA key: %w", err)
	}

	cs.caCert = cert
	cs.caKey = key
	return nil
}

func (cs *CertStore) GetCert(hostname string) (*tls.Certificate, error) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if cert, ok := cs.cache[hostname]; ok {
		return cert, nil
	}

	cert, err := cs.generateLeaf(hostname)
	if err != nil {
		return nil, err
	}
	cs.cache[hostname] = cert
	return cert, nil
}

func (cs *CertStore) generateLeaf(hostname string) (*tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, err
	}
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: hostname},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{hostname},
	}
	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, cs.caCert, &key.PublicKey, cs.caKey)
	if err != nil {
		return nil, err
	}
	return &tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  key,
	}, nil
}

func (cs *CertStore) TLSConfig() *tls.Config {
	return &tls.Config{
		GetCertificate: func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
			return cs.GetCert(hello.ServerName)
		},
	}
}
