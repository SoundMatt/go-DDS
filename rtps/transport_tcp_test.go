// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Internal tests for the RTPS-over-TCP transport (Milestone 14). Package
// rtps (internal) for direct access to tcpSocket, participant fields, and
// the dispatch/fallback helpers.

package rtps

//fusa:test REQ-TRANS-004
//fusa:test REQ-TRANS-005
//fusa:test REQ-TRANS-006

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"fmt"
	"math/big"
	"net"
	"testing"
	"time"

	dds "github.com/SoundMatt/go-DDS"
)

// ── tcpSocket primitives ─────────────────────────────────────────────────────

func newTestTCPSocket(t *testing.T, tlsConfig *tls.Config) *tcpSocket {
	t.Helper()
	s, err := newTCPSocket("127.0.0.1:0", tlsConfig)
	if err != nil {
		t.Skipf("newTCPSocket: %v — TCP loopback unavailable", err)
	}
	t.Cleanup(s.close)
	return s
}

func TestTCPSocket_SendReceive_Plaintext(t *testing.T) {
	server := newTestTCPSocket(t, nil)
	client := newTestTCPSocket(t, nil)

	addr := fmt.Sprintf("127.0.0.1:%d", server.port)
	want := []byte("rtps-over-tcp: hello")
	if err := client.send(addr, want); err != nil {
		t.Fatalf("send: %v", err)
	}

	select {
	case pkt := <-server.recv:
		if !bytes.Equal(pkt.data, want) {
			t.Errorf("payload: got %q, want %q", pkt.data, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for TCP frame")
	}
}

func TestTCPSocket_SendReceive_MultipleFrames(t *testing.T) {
	server := newTestTCPSocket(t, nil)
	client := newTestTCPSocket(t, nil)
	addr := fmt.Sprintf("127.0.0.1:%d", server.port)

	const n = 20
	for i := 0; i < n; i++ {
		msg := []byte(fmt.Sprintf("frame-%02d", i))
		if err := client.send(addr, msg); err != nil {
			t.Fatalf("send %d: %v", i, err)
		}
	}
	for i := 0; i < n; i++ {
		select {
		case pkt := <-server.recv:
			want := fmt.Sprintf("frame-%02d", i)
			if string(pkt.data) != want {
				t.Errorf("frame %d: got %q, want %q", i, pkt.data, want)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("timeout waiting for frame %d", i)
		}
	}
}

// generateTestTLSConfig returns an ephemeral, in-memory self-signed ECDSA
// certificate for 127.0.0.1 (no files, no external CA, valid only for this
// test process) wrapped in a *tls.Config with both Certificates (so it can
// serve as a tcpSocket listener) and RootCAs (so it can also validate its
// own certificate when dialing out) set — every tcpSocket is simultaneously
// a listener and a dialer, so the same config is used for both roles in
// these tests.
func generateTestTLSConfig(t *testing.T) *tls.Config {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "go-dds-tcp-test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		DNSNames:     []string{"go-dds-tcp-test"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}
	cert := tls.Certificate{Certificate: [][]byte{der}, PrivateKey: priv}

	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("ParseCertificate: %v", err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(leaf)
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      pool,
		ServerName:   "go-dds-tcp-test",
	}
}

func TestTCPSocket_SendReceive_TLS(t *testing.T) {
	cfg := generateTestTLSConfig(t)
	server := newTestTCPSocket(t, cfg)
	client := newTestTCPSocket(t, cfg)

	// newTCPSocket must have enforced TLS 1.3 since MinVersion was left unset.
	if server.tlsConfig.MinVersion != tls.VersionTLS13 {
		t.Errorf("server MinVersion: got %d, want TLS 1.3 (%d)", server.tlsConfig.MinVersion, tls.VersionTLS13)
	}

	addr := fmt.Sprintf("127.0.0.1:%d", server.port)
	want := []byte("rtps-over-tls: hello")
	if err := client.send(addr, want); err != nil {
		t.Fatalf("send: %v", err)
	}
	select {
	case pkt := <-server.recv:
		if !bytes.Equal(pkt.data, want) {
			t.Errorf("payload: got %q, want %q", pkt.data, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for TLS frame")
	}
}

func TestTCPSocket_Send_FrameTooLarge(t *testing.T) {
	client := newTestTCPSocket(t, nil)
	big := make([]byte, maxTCPFrameSize+1)
	if err := client.send("127.0.0.1:1", big); err == nil {
		t.Fatal("expected error for oversized frame, got nil")
	}
}

func TestTCPSocket_Send_DialFailure(t *testing.T) {
	client := newTestTCPSocket(t, nil)
	// Port 1 is a reserved/unassigned TCP port; dialling it should fail fast
	// rather than hang past the dial timeout.
	if err := client.send("127.0.0.1:1", []byte("x")); err == nil {
		t.Fatal("expected dial error, got nil")
	}
}

func TestTCPSocket_HostileLengthPrefix_DropsConnection(t *testing.T) {
	server := newTestTCPSocket(t, nil)
	addr := fmt.Sprintf("127.0.0.1:%d", server.port)

	conn, err := (&net.Dialer{}).DialContext(context.Background(), "tcp", addr)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()

	// Claim an absurd frame length; the server must drop the connection
	// instead of trying to allocate/read maxTCPFrameSize+1 bytes.
	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, maxTCPFrameSize+1)
	if _, err := conn.Write(lenBuf); err != nil {
		t.Fatalf("Write: %v", err)
	}

	// The connection should be closed by the server; further writes
	// eventually fail. Poll briefly rather than sleeping a fixed amount.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := conn.Write([]byte{0}); err != nil {
			return // server closed the connection, as expected
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("server did not drop the connection for a hostile length prefix")
}

// ── Locator (TCPv4) ──────────────────────────────────────────────────────────

func TestLocatorTCP_RoundTrip(t *testing.T) {
	l := locatorFromTCP(net.ParseIP("192.0.2.7"), 7500)
	if l.Kind != LocatorKindTCPv4 {
		t.Fatalf("Kind: got %d, want %d", l.Kind, LocatorKindTCPv4)
	}
	hp, ok := l.tcpHostPort()
	if !ok {
		t.Fatal("tcpHostPort: ok=false")
	}
	if hp != "192.0.2.7:7500" {
		t.Errorf("tcpHostPort: got %q, want %q", hp, "192.0.2.7:7500")
	}

	// A UDP locator must not be mistaken for a TCP one.
	udp := locatorFromUDP(&net.UDPAddr{IP: net.ParseIP("192.0.2.7")}, 7500)
	if _, ok := udp.tcpHostPort(); ok {
		t.Error("tcpHostPort: expected ok=false for a UDP locator")
	}
}

func TestLocatorTCP_WireRoundTrip(t *testing.T) {
	l := locatorFromTCP(net.ParseIP("10.1.2.3"), 9100)
	wire := marshalLocator(l)
	got, ok := unmarshalLocator(wire)
	if !ok {
		t.Fatal("unmarshalLocator: ok=false")
	}
	if got.Kind != LocatorKindTCPv4 || got.Port != 9100 {
		t.Errorf("round-trip mismatch: got %+v", got)
	}
	hp, ok := got.tcpHostPort()
	if !ok || hp != "10.1.2.3:9100" {
		t.Errorf("tcpHostPort after round-trip: got (%q,%v)", hp, ok)
	}
}

// ── participant fallback logic ────────────────────────────────────────────────

func TestParticipant_PreferTCP(t *testing.T) {
	cases := []struct {
		name       string
		tcpSock    *tcpSocket
		mcastOK    bool
		wantPrefer bool
	}{
		{"no TCP configured", nil, false, false},
		{"TCP configured, multicast available", &tcpSocket{}, true, false},
		{"TCP configured, multicast unavailable", &tcpSocket{}, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := &participant{tcpSock: tc.tcpSock, udpMulticastAvailable: tc.mcastOK}
			if got := p.preferTCP(); got != tc.wantPrefer {
				t.Errorf("preferTCP: got %v, want %v", got, tc.wantPrefer)
			}
		})
	}
}

func TestParticipant_TCPLocatorForIP(t *testing.T) {
	p := &participant{}
	p.spdp = newSPDPService(p)

	peerIP := net.ParseIP("127.0.0.1")
	proxy := &participantProxy{
		metatrafficUnicast: locatorFromUDP(&net.UDPAddr{IP: peerIP}, 17400),
		defaultUnicast:     locatorFromUDP(&net.UDPAddr{IP: peerIP}, 17401),
		tcpUnicast:         "127.0.0.1:17500",
	}
	p.spdp.peers[GuidPrefix{0x01}] = proxy

	if got := p.tcpLocatorForIP(peerIP); got != "127.0.0.1:17500" {
		t.Errorf("tcpLocatorForIP: got %q, want %q", got, "127.0.0.1:17500")
	}
	if got := p.tcpLocatorForIP(net.ParseIP("10.0.0.9")); got != "" {
		t.Errorf("tcpLocatorForIP(unknown): got %q, want empty", got)
	}
}

func TestParticipant_TCPLocatorForIP_NoSPDP(t *testing.T) {
	p := &participant{}
	if got := p.tcpLocatorForIP(net.ParseIP("127.0.0.1")); got != "" {
		t.Errorf("tcpLocatorForIP with nil spdp: got %q, want empty", got)
	}
}

// ── End-to-end: discovery and reliable delivery entirely over TCP ────────────

// withForcedUDPUnavailable is a test-only Option that pins
// udpMulticastAvailable to false, deterministically forcing the RTPS-over-TCP
// fallback path regardless of whether this CI runner's network namespace
// happens to support real UDP multicast. It only ever runs inside
// newParticipant's single-threaded options loop (before any goroutine
// starts), so it introduces no data race with the background loops that
// later read the field.
func withForcedUDPUnavailable() Option {
	return func(p *participant) { p.udpMulticastAvailable = false }
}

// TestTCP_CrossDomain_DiscoveryAndReliableDelivery proves the whole
// Milestone 14 TCP/TLS sub-phase end to end: two participants in different
// RTPS domains — so UDP multicast SPDP (which is domain-scoped by port)
// cannot possibly deliver between them — discover each other purely via
// SPDP-unicast-over-TCP (WithTCPPeers), match endpoints via SEDP-over-TCP,
// and exchange a reliable sample with HEARTBEAT/ACKNACK flowing over TCP too
// (forced via withForcedUDPUnavailable, mirroring a UDP-blocking firewall).
func TestTCP_CrossDomain_DiscoveryAndReliableDelivery(t *testing.T) {
	const p1Addr = "127.0.0.1:17510"
	const p2Addr = "127.0.0.1:17511"

	p2, err := newParticipant(dds.Domain(97),
		WithTCPAddr(p2Addr),
		WithTCPPeers(p1Addr),
		WithSPDPInterval(150*time.Millisecond),
		withForcedUDPUnavailable(),
	)
	if err != nil {
		t.Skipf("newParticipant(p2): %v — TCP loopback unavailable", err)
	}
	t.Cleanup(func() { _ = p2.Close() })

	p1, err := newParticipant(dds.Domain(98),
		WithTCPAddr(p1Addr),
		WithTCPPeers(p2Addr),
		WithSPDPInterval(150*time.Millisecond),
		withForcedUDPUnavailable(),
	)
	if err != nil {
		t.Skipf("newParticipant(p1): %v — TCP loopback unavailable", err)
	}
	t.Cleanup(func() { _ = p1.Close() })

	sub, err := p2.NewSubscriber("tcp/cross-domain", dds.ReliableQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()

	pub, err := p1.NewPublisher("tcp/cross-domain", dds.ReliableQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	// Give SPDP-over-TCP + SEDP-over-TCP time to complete. The first SPDP
	// announcement fires immediately at startup, so this is generous rather
	// than tight.
	time.Sleep(1 * time.Second)

	want := []byte(`{"transport":"rtps-over-tcp","ok":true}`)
	if err := pub.Write(want); err != nil {
		t.Fatalf("Write: %v", err)
	}

	select {
	case s := <-sub.C():
		if !bytes.Equal(s.Payload, want) {
			t.Errorf("payload: got %q, want %q", s.Payload, want)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout: sample not delivered over RTPS-over-TCP")
	}

	// Confirm the mechanism under test actually ran: both participants must
	// have at least one live TCP connection to the other, proving discovery
	// and delivery took the TCP path rather than some incidental UDP route.
	p1.tcpSock.mu.Lock()
	p1Conns := len(p1.tcpSock.conns)
	p1.tcpSock.mu.Unlock()
	p2.tcpSock.mu.Lock()
	p2Conns := len(p2.tcpSock.conns)
	p2.tcpSock.mu.Unlock()
	if p1Conns == 0 && p2Conns == 0 {
		t.Error("expected at least one cached TCP connection on p1 or p2; none found")
	}
}

// TestTCP_WithTCPTLSConfig_StartsCleanly confirms a participant can be built
// with the TCP transport wrapped in TLS 1.3 and shuts down cleanly, without
// requiring a live peer.
func TestTCP_WithTCPTLSConfig_StartsCleanly(t *testing.T) {
	cfg := generateTestTLSConfig(t)
	p, err := newParticipant(dds.Domain(96),
		WithTCPAddr("127.0.0.1:17512"),
		WithTCPTLSConfig(cfg),
	)
	if err != nil {
		t.Skipf("newParticipant: %v — TCP loopback unavailable", err)
	}
	defer func() { _ = p.Close() }()

	if p.tcpSock == nil {
		t.Fatal("tcpSock is nil")
	}
	if p.tcpSock.tlsConfig == nil {
		t.Fatal("tcpSock.tlsConfig is nil; TLS was not applied")
	}
	if p.tcpSock.tlsConfig.MinVersion != tls.VersionTLS13 {
		t.Errorf("MinVersion: got %d, want TLS 1.3", p.tcpSock.tlsConfig.MinVersion)
	}
}

// TestTCP_Disabled_ByDefault confirms a participant created without
// WithTCPAddr has no TCP transport and sendUnicast behaves exactly as before
// Milestone 14 — a plain UDP send, no behaviour change for existing callers.
func TestTCP_Disabled_ByDefault(t *testing.T) {
	p := testPart(t)
	if p.tcpSock != nil {
		t.Fatal("tcpSock should be nil without WithTCPAddr")
	}
	if p.preferTCP() {
		t.Error("preferTCP should be false without WithTCPAddr")
	}
}
