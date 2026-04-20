package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

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
	dataDir    = "/data"
	mtu        = 1420
	nicID      = 1
	listenPort = 51820
	// mitmproxy compat: server=10.0.0.1 is not used; client=10.0.0.1, dns=10.0.0.53
	serverAddr = "10.0.0.2"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--healthcheck" {
		if _, err := os.Stat(filepath.Join(dataDir, "wireguard.conf")); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	policy, err := LoadConfig(configPath)
	if err != nil {
		slog.Error("failed to load config", "err", err)
		os.Exit(1)
	}

	if err := os.MkdirAll(dataDir, 0755); err != nil {
		slog.Error("failed to create data dir", "err", err)
		os.Exit(1)
	}

	certStore, err := NewCertStore(dataDir)
	if err != nil {
		slog.Error("failed to initialize cert store", "err", err)
		os.Exit(1)
	}

	proxy := NewProxy(policy, certStore)

	serverPriv, _ := generateKey()
	clientPriv, _ := generateKey()

	if err := writeWireGuardConf(dataDir, serverPriv, clientPriv); err != nil {
		slog.Error("failed to write wireguard.conf", "err", err)
		os.Exit(1)
	}

	// Set up gvisor netstack
	s := stack.New(stack.Options{
		NetworkProtocols:   []stack.NetworkProtocolFactory{ipv4.NewProtocol},
		TransportProtocols: []stack.TransportProtocolFactory{tcp.NewProtocol, udp.NewProtocol},
	})

	ep := channel.New(512, uint32(mtu), "")

	if tcperr := s.CreateNIC(nicID, ep); tcperr != nil {
		slog.Error("failed to create NIC", "err", tcperr)
		os.Exit(1)
	}

	parsed := netip.MustParseAddr(serverAddr).As4()
	addr := tcpip.AddrFromSlice(parsed[:])
	protoAddr := tcpip.ProtocolAddress{
		Protocol:          ipv4.ProtocolNumber,
		AddressWithPrefix: tcpip.AddressWithPrefix{Address: addr, PrefixLen: 32},
	}
	if tcperr := s.AddProtocolAddress(nicID, protoAddr, stack.AddressProperties{}); tcperr != nil {
		slog.Error("failed to add address", "err", tcperr)
		os.Exit(1)
	}

	// Enable promiscuous mode so the NIC accepts packets for any destination
	s.SetPromiscuousMode(nicID, true)
	// Enable spoofing so the stack can send packets with any source address
	s.SetSpoofing(nicID, true)

	s.SetRouteTable([]tcpip.Route{
		{Destination: header.IPv4EmptySubnet, NIC: nicID},
	})

	// TCP forwarder — intercepts all TCP connections
	tcpFwd := tcp.NewForwarder(s, 0, 65535, func(r *tcp.ForwarderRequest) {
		id := r.ID()
		var wq waiter.Queue
		ep, tcperr := r.CreateEndpoint(&wq)
		if tcperr != nil {
			r.Complete(true)
			return
		}
		r.Complete(false)

		c := gonet.NewTCPConn(&wq, ep)
		dstAddr := net.TCPAddr{
			IP:   net.IP(id.LocalAddress.AsSlice()),
			Port: int(id.LocalPort),
		}
		go proxy.HandleTCP(c, dstAddr)
	})
	s.SetTransportProtocolHandler(tcp.ProtocolNumber, tcpFwd.HandlePacket)

	// UDP forwarder — forward DNS, block everything else
	udpFwd := udp.NewForwarder(s, func(r *udp.ForwarderRequest) {
		id := r.ID()
		if id.LocalPort == 53 {
			handleDNSRequest(s, r)
			return
		}
		slog.Warn("blocked UDP connection",
			"dst", fmt.Sprintf("%s:%d", id.LocalAddress, id.LocalPort))
		// Don't create endpoint — just drop
	})
	s.SetTransportProtocolHandler(udp.ProtocolNumber, udpFwd.HandlePacket)

	// Create WireGuard device
	tunDev := newChannelTUN(ep, mtu)
	logger := device.NewLogger(device.LogLevelError, "wireguard: ")
	wgDev := device.NewDevice(tunDev, conn.NewDefaultBind(), logger)

	clientPub := publicKeyFromPrivate(clientPriv)
	ipcConfig := fmt.Sprintf(`private_key=%s
listen_port=%d
public_key=%s
allowed_ip=0.0.0.0/0
`,
		hex.EncodeToString(serverPriv[:]),
		listenPort,
		hex.EncodeToString(clientPub[:]),
	)

	if err := wgDev.IpcSet(ipcConfig); err != nil {
		slog.Error("failed to configure wireguard", "err", err)
		os.Exit(1)
	}
	if err := wgDev.Up(); err != nil {
		slog.Error("failed to bring up wireguard", "err", err)
		os.Exit(1)
	}

	slog.Info("suisou router started",
		"listen", fmt.Sprintf(":%d/udp", listenPort),
		"endpoints", len(policy.Endpoints),
		"credentials", len(policy.Credentials),
	)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	slog.Info("shutting down")
	wgDev.Close()
}

func handleDNSRequest(s *stack.Stack, r *udp.ForwarderRequest) {
	id := r.ID()
	var wq waiter.Queue
	ep, tcperr := r.CreateEndpoint(&wq)
	if tcperr != nil {
		return
	}

	c := gonet.NewUDPConn(&wq, ep)
	defer c.Close()

	buf := make([]byte, 4096)
	n, addr, err := c.ReadFrom(buf)
	if err != nil {
		return
	}

	resp := forwardDNS(buf[:n], addr)
	if resp != nil {
		c.WriteTo(resp, addr)
	}

	_ = id
}

type wgKey [32]byte

func generateKey() (private, public wgKey) {
	var priv wgKey
	if _, err := rand.Read(priv[:]); err != nil {
		panic(err)
	}
	// Clamp for Curve25519
	priv[0] &= 248
	priv[31] = (priv[31] & 127) | 64

	pub := publicKeyFromPrivate(priv)
	return priv, pub
}

func publicKeyFromPrivate(priv wgKey) wgKey {
	var pub wgKey
	pubSlice, _ := curve25519.X25519(priv[:], curve25519.Basepoint)
	copy(pub[:], pubSlice)
	return pub
}

func writeWireGuardConf(dir string, serverPriv, clientPriv wgKey) error {
	conf := map[string]string{
		"server_key": base64.StdEncoding.EncodeToString(serverPriv[:]),
		"client_key": base64.StdEncoding.EncodeToString(clientPriv[:]),
	}
	data, err := json.Marshal(conf)
	if err != nil {
		return err
	}
	path := filepath.Join(dir, "wireguard.conf")
	if err := os.WriteFile(path, data, 0644); err != nil {
		return err
	}
	slog.Info("wrote wireguard.conf", "path", path)
	return nil
}
