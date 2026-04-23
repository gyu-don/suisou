package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"suisou/router/internal/config"
	"suisou/router/internal/dns"
	"suisou/router/internal/proxy"
	"suisou/router/internal/wgstack"

	"golang.org/x/crypto/curve25519"
	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"

	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/adapters/gonet"
	"gvisor.dev/gvisor/pkg/tcpip/header"
	"gvisor.dev/gvisor/pkg/tcpip/link/channel"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv4"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
	"gvisor.dev/gvisor/pkg/tcpip/transport/tcp"
	"gvisor.dev/gvisor/pkg/tcpip/transport/udp"
	"gvisor.dev/gvisor/pkg/waiter"
)

const (
	configPath = "/etc/suisou/config.toml"
	publicDir  = "/data/public"
	privateDir = "/data/private"
	mtu        = 1420
	nicID      = 1
	listenPort = 51820
	serverAddr = "10.0.0.2"
)

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		var exitErr *exitCodeError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.code)
		}
		slog.Error("router failed", "err", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) > 0 && args[0] == "--healthcheck" {
		return runHealthcheck()
	}

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	policy, err := config.LoadPolicy(configPath)
	if err != nil {
		return err
	}
	for _, dir := range []string{publicDir, privateDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("creating data dir %q: %w", dir, err)
		}
	}

	certStore, err := proxy.NewCertStore(privateDir, publicDir)
	if err != nil {
		return fmt.Errorf("initializing cert store: %w", err)
	}
	resolver := dns.NewResolver()
	httpProxy := proxy.New(policy, certStore)

	serverPriv, _ := generateKey()
	clientPriv, _ := generateKey()
	serverPub := publicKeyFromPrivate(serverPriv)
	if err := writeWireGuardConf(publicDir, serverPub, clientPriv); err != nil {
		return fmt.Errorf("writing wireguard.conf: %w", err)
	}

	s := stack.New(stack.Options{
		NetworkProtocols:   []stack.NetworkProtocolFactory{ipv4.NewProtocol},
		TransportProtocols: []stack.TransportProtocolFactory{tcp.NewProtocol, udp.NewProtocol},
	})
	ep := channel.New(512, uint32(mtu), "")

	if tcpErr := s.CreateNIC(nicID, ep); tcpErr != nil {
		return fmt.Errorf("creating NIC: %v", tcpErr)
	}

	parsed := netip.MustParseAddr(serverAddr).As4()
	addr := tcpip.AddrFromSlice(parsed[:])
	protoAddr := tcpip.ProtocolAddress{
		Protocol:          ipv4.ProtocolNumber,
		AddressWithPrefix: tcpip.AddressWithPrefix{Address: addr, PrefixLen: 32},
	}
	if tcpErr := s.AddProtocolAddress(nicID, protoAddr, stack.AddressProperties{}); tcpErr != nil {
		return fmt.Errorf("adding address: %v", tcpErr)
	}

	s.SetPromiscuousMode(nicID, true)
	s.SetSpoofing(nicID, true)
	s.SetRouteTable([]tcpip.Route{{Destination: header.IPv4EmptySubnet, NIC: nicID}})

	tcpFwd := tcp.NewForwarder(s, 0, 65535, func(r *tcp.ForwarderRequest) {
		id := r.ID()
		var wq waiter.Queue
		endpoint, tcpErr := r.CreateEndpoint(&wq)
		if tcpErr != nil {
			r.Complete(true)
			return
		}
		r.Complete(false)

		clientConn := gonet.NewTCPConn(&wq, endpoint)
		dstAddr := net.TCPAddr{
			IP:   net.IP(id.LocalAddress.AsSlice()),
			Port: int(id.LocalPort),
		}
		go httpProxy.HandleTCP(clientConn, dstAddr)
	})
	s.SetTransportProtocolHandler(tcp.ProtocolNumber, tcpFwd.HandlePacket)

	udpFwd := udp.NewForwarder(s, func(r *udp.ForwarderRequest) {
		id := r.ID()
		if id.LocalPort == 53 {
			handleDNSRequest(r, resolver)
			return
		}
		slog.Warn("blocked UDP connection", "dst", fmt.Sprintf("%s:%d", id.LocalAddress, id.LocalPort))
	})
	s.SetTransportProtocolHandler(udp.ProtocolNumber, udpFwd.HandlePacket)

	tunDev := wgstack.NewChannelTUN(ep, mtu)
	logger := device.NewLogger(device.LogLevelError, "wireguard: ")
	wgDev := device.NewDevice(tunDev, conn.NewDefaultBind(), logger)
	defer wgDev.Close()
	defer tunDev.Close()

	clientPub := publicKeyFromPrivate(clientPriv)
	ipcConfig := fmt.Sprintf("private_key=%s\nlisten_port=%d\npublic_key=%s\nallowed_ip=0.0.0.0/0\n",
		hex.EncodeToString(serverPriv[:]),
		listenPort,
		hex.EncodeToString(clientPub[:]),
	)
	if err := wgDev.IpcSet(ipcConfig); err != nil {
		return fmt.Errorf("configuring wireguard: %w", err)
	}
	if err := wgDev.Up(); err != nil {
		return fmt.Errorf("bringing up wireguard: %w", err)
	}

	slog.Info("suisou router started",
		"listen", fmt.Sprintf(":%d/udp", listenPort),
		"endpoints", len(policy.Endpoints),
		"credentials", len(policy.Credentials),
		"dns_resolver", resolver.Address(),
	)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sig)

	select {
	case <-ctx.Done():
	case <-sig:
	}

	slog.Info("shutting down")
	return nil
}

func runHealthcheck() error {
	required := []string{
		filepath.Join(publicDir, "wireguard.conf"),
		filepath.Join(publicDir, "mitmproxy-ca-cert.pem"),
		filepath.Join(privateDir, "ca-key.pem"),
	}
	for _, path := range required {
		if _, err := os.Stat(path); err != nil {
			return &exitCodeError{code: 1, err: err}
		}
	}
	return &exitCodeError{code: 0}
}

func handleDNSRequest(r *udp.ForwarderRequest, resolver *dns.Resolver) {
	var wq waiter.Queue
	endpoint, tcpErr := r.CreateEndpoint(&wq)
	if tcpErr != nil {
		return
	}

	conn := gonet.NewUDPConn(&wq, endpoint)
	defer conn.Close()

	buf := make([]byte, 4096)
	n, addr, err := conn.ReadFrom(buf)
	if err != nil {
		return
	}

	resp := resolver.Forward(buf[:n])
	if len(resp) == 0 {
		return
	}
	if _, err := conn.WriteTo(resp, addr); err != nil {
		slog.Debug("DNS writeback failed", "err", err)
	}
}

type wgKey [32]byte

func generateKey() (private, public wgKey) {
	var priv wgKey
	if _, err := rand.Read(priv[:]); err != nil {
		panic(err)
	}
	priv[0] &= 248
	priv[31] = (priv[31] & 127) | 64
	return priv, publicKeyFromPrivate(priv)
}

func publicKeyFromPrivate(priv wgKey) wgKey {
	var pub wgKey
	pubSlice, _ := curve25519.X25519(priv[:], curve25519.Basepoint)
	copy(pub[:], pubSlice)
	return pub
}

func writeWireGuardConf(dir string, serverPub, clientPriv wgKey) error {
	conf := map[string]string{
		"server_public_key":  base64.StdEncoding.EncodeToString(serverPub[:]),
		"client_private_key": base64.StdEncoding.EncodeToString(clientPriv[:]),
	}
	data, err := json.Marshal(conf)
	if err != nil {
		return err
	}
	path := filepath.Join(dir, "wireguard.conf")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return err
	}
	slog.Info("wrote wireguard.conf", "path", path)
	return nil
}

type exitCodeError struct {
	code int
	err  error
}

func (e *exitCodeError) Error() string {
	if e.err != nil {
		return e.err.Error()
	}
	return fmt.Sprintf("exit %d", e.code)
}
