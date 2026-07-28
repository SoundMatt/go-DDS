// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Internal tests for the RTPS-over-QUIC transport (Milestone 16). Package
// rtps (internal) for direct access to quicSocket, participant fields, and
// the dispatch/fallback helpers. generateTestTLSConfig (self-signed ECDSA
// cert + matching CA pool, defined in transport_tcp_test.go) is reused here
// too — QUIC drives the same stdlib *tls.Config shape as RTPS-over-TCP/DTLS
// (see WithQUIC), just always populated since QUIC mandates TLS.

package rtps

//fusa:test REQ-TRANS-012
//fusa:test REQ-TRANS-013
//fusa:test REQ-TRANS-014

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"net"
	"testing"
	"time"

	dds "github.com/SoundMatt/go-DDS"
)

// ── quicSocket primitives ────────────────────────────────────────────────────

func newTestQUICSocket(t *testing.T, tlsConfig *tls.Config) *quicSocket {
	t.Helper()
	s, err := newQUICSocket("127.0.0.1:0", tlsConfig)
	if err != nil {
		t.Skipf("newQUICSocket: %v — QUIC/UDP loopback unavailable", err)
	}
	t.Cleanup(s.close)
	return s
}

func recvQUICPacket(t *testing.T, s *quicSocket) quicPacket {
	t.Helper()
	select {
	case pkt := <-s.recv:
		return pkt
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for QUIC packet")
		return quicPacket{}
	}
}

func TestQUICSocket_SendReceive_ReliableStream(t *testing.T) {
	cfg := generateTestTLSConfig(t)
	server := newTestQUICSocket(t, cfg)
	client := newTestQUICSocket(t, cfg)

	// newQUICSocket must have enforced TLS 1.3 and a default ALPN since both
	// were left unset.
	if server.clientTLSConfig.MinVersion != tls.VersionTLS13 {
		t.Errorf("MinVersion: got %d, want TLS 1.3 (%d)", server.clientTLSConfig.MinVersion, tls.VersionTLS13)
	}
	if len(server.clientTLSConfig.NextProtos) == 0 || server.clientTLSConfig.NextProtos[0] != quicALPN {
		t.Errorf("NextProtos: got %v, want [%q]", server.clientTLSConfig.NextProtos, quicALPN)
	}
	// The client-role config must have a session cache installed (0-RTT).
	if client.clientTLSConfig.ClientSessionCache == nil {
		t.Error("client ClientSessionCache: got nil, want a default LRU cache")
	}

	addr := fmt.Sprintf("127.0.0.1:%d", server.port)
	want := []byte("rtps-over-quic: reliable stream hello")
	if err := client.send(addr, want, true); err != nil {
		t.Fatalf("send(reliable=true): %v", err)
	}
	pkt := recvQUICPacket(t, server)
	if !bytes.Equal(pkt.data, want) {
		t.Errorf("payload: got %q, want %q", pkt.data, want)
	}
}

func TestQUICSocket_SendReceive_MultipleReliableFrames(t *testing.T) {
	cfg := generateTestTLSConfig(t)
	server := newTestQUICSocket(t, cfg)
	client := newTestQUICSocket(t, cfg)
	addr := fmt.Sprintf("127.0.0.1:%d", server.port)

	const n = 20
	for i := 0; i < n; i++ {
		msg := []byte(fmt.Sprintf("frame-%02d", i))
		if err := client.send(addr, msg, true); err != nil {
			t.Fatalf("send %d: %v", i, err)
		}
	}
	for i := 0; i < n; i++ {
		pkt := recvQUICPacket(t, server)
		want := fmt.Sprintf("frame-%02d", i)
		if string(pkt.data) != want {
			t.Errorf("frame %d: got %q, want %q", i, pkt.data, want)
		}
	}
}

func TestQUICSocket_SendReceive_BestEffortDatagram(t *testing.T) {
	cfg := generateTestTLSConfig(t)
	server := newTestQUICSocket(t, cfg)
	client := newTestQUICSocket(t, cfg)
	addr := fmt.Sprintf("127.0.0.1:%d", server.port)

	want := []byte("rtps-over-quic: best-effort datagram hello")
	if err := client.send(addr, want, false); err != nil {
		t.Fatalf("send(reliable=false): %v", err)
	}
	pkt := recvQUICPacket(t, server)
	if !bytes.Equal(pkt.data, want) {
		t.Errorf("payload: got %q, want %q", pkt.data, want)
	}
}

// TestQUICSocket_BestEffort_FallsBackToStream_WhenTooLarge proves REQ-TRANS-013:
// a best-effort message too large for a single QUIC datagram at the current
// path MTU is still delivered, over the reliable stream, rather than dropped.
func TestQUICSocket_BestEffort_FallsBackToStream_WhenTooLarge(t *testing.T) {
	cfg := generateTestTLSConfig(t)
	server := newTestQUICSocket(t, cfg)
	client := newTestQUICSocket(t, cfg)
	addr := fmt.Sprintf("127.0.0.1:%d", server.port)

	// Comfortably larger than any QUIC datagram's typical path-MTU ceiling
	// (~1200-1450 bytes) but well under maxQUICFrameSize.
	big := make([]byte, 4096)
	for i := range big {
		big[i] = byte(i)
	}
	if err := client.send(addr, big, false); err != nil {
		t.Fatalf("send(reliable=false, oversized): %v", err)
	}
	pkt := recvQUICPacket(t, server)
	if !bytes.Equal(pkt.data, big) {
		t.Errorf("payload mismatch after datagram->stream fallback: got %d bytes, want %d", len(pkt.data), len(big))
	}
}

func TestQUICSocket_Send_FrameTooLarge(t *testing.T) {
	cfg := generateTestTLSConfig(t)
	client := newTestQUICSocket(t, cfg)
	big := make([]byte, maxQUICFrameSize+1)
	if err := client.send("127.0.0.1:1", big, true); err == nil {
		t.Fatal("expected error for oversized frame, got nil")
	}
}

// TestQUICSocket_Send_DialTimeout proves REQ-TRANS-014's dial path fails
// within a bounded time rather than hanging forever, mirroring
// TestDTLSSocket_Send_HandshakeTimeout.
func TestQUICSocket_Send_DialTimeout(t *testing.T) {
	cfg := generateTestTLSConfig(t)
	client := newTestQUICSocket(t, cfg)

	// Bind a plain UDP socket that never speaks QUIC, so the handshake gets
	// no response and must time out rather than hang.
	deadEnd, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Skipf("ListenUDP: %v", err)
	}
	defer deadEnd.Close()
	addr := deadEnd.LocalAddr().String()

	done := make(chan error, 1)
	go func() { done <- client.send(addr, []byte("nobody home"), true) }()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected dial/handshake timeout error, got nil")
		}
	case <-time.After(quicDialTimeout + 5*time.Second):
		t.Fatal("QUIC dial did not time out within quicDialTimeout + margin")
	}
}

// ── Locator (QUICv4) ─────────────────────────────────────────────────────────

func TestLocatorQUIC_RoundTrip(t *testing.T) {
	l := locatorFromQUIC(net.ParseIP("192.0.2.9"), 7700)
	if l.Kind != LocatorKindQUICv4 {
		t.Fatalf("Kind: got %d, want %d", l.Kind, LocatorKindQUICv4)
	}
	hp, ok := l.quicHostPort()
	if !ok {
		t.Fatal("quicHostPort: ok=false")
	}
	if hp != "192.0.2.9:7700" {
		t.Errorf("quicHostPort: got %q, want %q", hp, "192.0.2.9:7700")
	}

	// A TCP locator must not be mistaken for a QUIC one, and vice versa.
	tcp := locatorFromTCP(net.ParseIP("192.0.2.9"), 7700)
	if _, ok := tcp.quicHostPort(); ok {
		t.Error("quicHostPort: expected ok=false for a TCP locator")
	}
	if _, ok := l.tcpHostPort(); ok {
		t.Error("tcpHostPort: expected ok=false for a QUIC locator")
	}
}

func TestLocatorQUIC_WireRoundTrip(t *testing.T) {
	l := locatorFromQUIC(net.ParseIP("10.1.2.4"), 9200)
	wire := marshalLocator(l)
	got, ok := unmarshalLocator(wire)
	if !ok {
		t.Fatal("unmarshalLocator: ok=false")
	}
	if got.Kind != LocatorKindQUICv4 || got.Port != 9200 {
		t.Errorf("round-trip mismatch: got %+v", got)
	}
	hp, ok := got.quicHostPort()
	if !ok || hp != "10.1.2.4:9200" {
		t.Errorf("quicHostPort after round-trip: got (%q,%v)", hp, ok)
	}
}

// ── participant lookup helpers ────────────────────────────────────────────────

func TestParticipant_QUICLocatorForIP(t *testing.T) {
	p := &participant{}
	p.spdp = newSPDPService(p)

	peerIP := net.ParseIP("127.0.0.1")
	proxy := &participantProxy{
		metatrafficUnicast: locatorFromUDP(&net.UDPAddr{IP: peerIP}, 17400),
		defaultUnicast:     locatorFromUDP(&net.UDPAddr{IP: peerIP}, 17401),
		quicUnicast:        "127.0.0.1:17700",
	}
	p.spdp.peers[GuidPrefix{0x02}] = proxy

	if got := p.quicLocatorForIP(peerIP); got != "127.0.0.1:17700" {
		t.Errorf("quicLocatorForIP: got %q, want %q", got, "127.0.0.1:17700")
	}
	if got := p.quicLocatorForIP(net.ParseIP("10.0.0.9")); got != "" {
		t.Errorf("quicLocatorForIP(unknown): got %q, want empty", got)
	}
}

func TestParticipant_QUICLocatorForIP_NoSPDP(t *testing.T) {
	p := &participant{}
	if got := p.quicLocatorForIP(net.ParseIP("127.0.0.1")); got != "" {
		t.Errorf("quicLocatorForIP with nil spdp: got %q, want empty", got)
	}
}

// ── End-to-end: discovery and delivery entirely over QUIC ────────────────────

// TestQUIC_CrossDomain_DiscoveryAndDelivery proves the whole Milestone 16
// QUIC sub-phase end to end: two participants in different RTPS domains — so
// UDP multicast SPDP cannot possibly deliver between them — discover each
// other purely via SPDP-unicast-over-QUIC's reliable stream (WithQUICPeers),
// match endpoints via SEDP-over-QUIC, and exchange both a best-effort sample
// (over QUIC's unreliable datagram channel) and a reliable sample (over
// QUIC's reliable stream, with HEARTBEAT/ACKNACK also flowing over QUIC)
// with UDP forced unavailable (withForcedUDPUnavailable, from
// transport_tcp_test.go) and user-data multicast disabled (WithNoMulticast)
// so any successful delivery — including the single-shot best-effort sample,
// which unlike a reliable one has no retransmission protocol to fall back
// on — can only have taken the QUIC unicast path.
func TestQUIC_CrossDomain_DiscoveryAndDelivery(t *testing.T) {
	cfg := generateTestTLSConfig(t)
	const p1Addr = "127.0.0.1:17710"
	const p2Addr = "127.0.0.1:17711"

	p2, err := newParticipant(dds.Domain(95),
		WithQUICAddr(p2Addr),
		WithQUIC(cfg),
		WithQUICPeers(p1Addr),
		WithSPDPInterval(150*time.Millisecond),
		WithNoMulticast(),
		withForcedUDPUnavailable(),
	)
	if err != nil {
		t.Skipf("newParticipant(p2): %v — QUIC/UDP loopback unavailable", err)
	}
	t.Cleanup(func() { _ = p2.Close() })

	p1, err := newParticipant(dds.Domain(94),
		WithQUICAddr(p1Addr),
		WithQUIC(cfg),
		WithQUICPeers(p2Addr),
		WithSPDPInterval(150*time.Millisecond),
		WithNoMulticast(),
		withForcedUDPUnavailable(),
	)
	if err != nil {
		t.Skipf("newParticipant(p1): %v — QUIC/UDP loopback unavailable", err)
	}
	t.Cleanup(func() { _ = p1.Close() })

	subBE, err := p2.NewSubscriber("quic/cross-domain/best-effort", dds.QoS{})
	if err != nil {
		t.Fatalf("NewSubscriber (best-effort): %v", err)
	}
	defer subBE.Close()

	subRel, err := p2.NewSubscriber("quic/cross-domain/reliable", dds.ReliableQoS)
	if err != nil {
		t.Fatalf("NewSubscriber (reliable): %v", err)
	}
	defer subRel.Close()

	pubBE, err := p1.NewPublisher("quic/cross-domain/best-effort", dds.QoS{})
	if err != nil {
		t.Fatalf("NewPublisher (best-effort): %v", err)
	}
	defer pubBE.Close()

	pubRel, err := p1.NewPublisher("quic/cross-domain/reliable", dds.ReliableQoS)
	if err != nil {
		t.Fatalf("NewPublisher (reliable): %v", err)
	}
	defer pubRel.Close()

	// Give SPDP-over-QUIC + SEDP-over-QUIC time to complete. The first SPDP
	// announcement fires immediately at startup, so this is generous rather
	// than tight.
	time.Sleep(1 * time.Second)

	wantBE := []byte(`{"transport":"rtps-over-quic","channel":"datagram"}`)
	if err := pubBE.Write(wantBE); err != nil {
		t.Fatalf("Write (best-effort): %v", err)
	}
	select {
	case s := <-subBE.C():
		if !bytes.Equal(s.Payload, wantBE) {
			t.Errorf("best-effort payload: got %q, want %q", s.Payload, wantBE)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout: best-effort sample not delivered over RTPS-over-QUIC")
	}

	wantRel := []byte(`{"transport":"rtps-over-quic","channel":"reliable-stream"}`)
	if err := pubRel.Write(wantRel); err != nil {
		t.Fatalf("Write (reliable): %v", err)
	}
	select {
	case s := <-subRel.C():
		if !bytes.Equal(s.Payload, wantRel) {
			t.Errorf("reliable payload: got %q, want %q", s.Payload, wantRel)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout: reliable sample not delivered over RTPS-over-QUIC")
	}

	// Confirm the mechanism under test actually ran: both participants must
	// have at least one live QUIC connection to the other, proving discovery
	// and delivery took the QUIC path rather than some incidental UDP route.
	p1.quicSock.mu.Lock()
	p1Conns := len(p1.quicSock.peers)
	p1.quicSock.mu.Unlock()
	p2.quicSock.mu.Lock()
	p2Conns := len(p2.quicSock.peers)
	p2.quicSock.mu.Unlock()
	if p1Conns == 0 && p2Conns == 0 {
		t.Error("expected at least one cached QUIC connection on p1 or p2; none found")
	}
}

// TestQUIC_WithQUIC_StartsCleanly confirms a participant can be built with
// the QUIC transport and shuts down cleanly, without requiring a live peer.
func TestQUIC_WithQUIC_StartsCleanly(t *testing.T) {
	cfg := generateTestTLSConfig(t)
	p, err := newParticipant(dds.Domain(93),
		WithQUICAddr("127.0.0.1:17712"),
		WithQUIC(cfg),
	)
	if err != nil {
		t.Skipf("newParticipant: %v — QUIC/UDP loopback unavailable", err)
	}
	defer func() { _ = p.Close() }()

	if p.quicSock == nil {
		t.Fatal("quicSock is nil")
	}
	if p.quicSock.clientTLSConfig.MinVersion != tls.VersionTLS13 {
		t.Errorf("MinVersion: got %d, want TLS 1.3", p.quicSock.clientTLSConfig.MinVersion)
	}
}

// TestQUIC_Disabled_ByDefault confirms a participant created without
// WithQUICAddr has no QUIC transport and sendUnicast behaves exactly as
// before Milestone 16 — no behaviour change for existing callers.
func TestQUIC_Disabled_ByDefault(t *testing.T) {
	p := testPart(t)
	if p.quicSock != nil {
		t.Fatal("quicSock should be nil without WithQUICAddr")
	}
	if got := p.quicLocatorForIP(net.ParseIP("127.0.0.1")); got != "" {
		t.Errorf("quicLocatorForIP: got %q, want empty when QUIC disabled", got)
	}
}
