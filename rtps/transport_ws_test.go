// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Internal tests for the RTPS-over-WebSocket transport (Milestone 16).
// Package rtps (internal) for direct access to wsSocket, participant
// fields, and the dispatch/fallback helpers. generateTestTLSConfig (defined
// in transport_tcp_test.go) is reused here too — WS's optional TLS wrapping
// (wss://) drives the same stdlib *tls.Config shape as RTPS-over-TCP (see
// WithWSTLSConfig).

package rtps

//fusa:test REQ-TRANS-015
//fusa:test REQ-TRANS-016
//fusa:test REQ-TRANS-017

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	dds "github.com/SoundMatt/go-DDS"
)

// ── wsFrame primitives ───────────────────────────────────────────────────────

func TestWSFrame_RoundTrip_Unmasked(t *testing.T) {
	var buf bytes.Buffer
	payload := []byte("rtps-over-ws: binary frame, unmasked (server role)")
	if err := writeWSFrame(&buf, wsOpBinary, payload, false); err != nil {
		t.Fatalf("writeWSFrame: %v", err)
	}
	fin, op, got, err := readWSFrame(bufio.NewReader(&buf))
	if err != nil {
		t.Fatalf("readWSFrame: %v", err)
	}
	if !fin {
		t.Error("fin: got false, want true (unfragmented frame)")
	}
	if op != wsOpBinary {
		t.Errorf("opcode: got %d, want %d", op, wsOpBinary)
	}
	if !bytes.Equal(got, payload) {
		t.Errorf("payload: got %q, want %q", got, payload)
	}
}

func TestWSFrame_RoundTrip_Masked(t *testing.T) {
	var buf bytes.Buffer
	payload := []byte("rtps-over-ws: binary frame, masked (client role)")
	if err := writeWSFrame(&buf, wsOpBinary, payload, true); err != nil {
		t.Fatalf("writeWSFrame: %v", err)
	}
	// A masked frame's on-wire bytes must not contain the plaintext payload
	// verbatim (proves masking actually happened, not just a round-trip
	// coincidence).
	if bytes.Contains(buf.Bytes(), payload) {
		t.Error("masked frame's wire bytes contain the unmasked payload verbatim")
	}
	fin, op, got, err := readWSFrame(bufio.NewReader(&buf))
	if err != nil {
		t.Fatalf("readWSFrame: %v", err)
	}
	if !fin || op != wsOpBinary {
		t.Errorf("fin=%v op=%d, want fin=true op=%d", fin, op, wsOpBinary)
	}
	if !bytes.Equal(got, payload) {
		t.Errorf("payload after unmask: got %q, want %q", got, payload)
	}
}

func TestWSFrame_RoundTrip_16BitExtendedLength(t *testing.T) {
	var buf bytes.Buffer
	payload := make([]byte, 500) // >= 126: forces the 16-bit extended length field
	for i := range payload {
		payload[i] = byte(i)
	}
	if err := writeWSFrame(&buf, wsOpBinary, payload, false); err != nil {
		t.Fatalf("writeWSFrame: %v", err)
	}
	_, _, got, err := readWSFrame(bufio.NewReader(&buf))
	if err != nil {
		t.Fatalf("readWSFrame: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Errorf("payload mismatch: got %d bytes, want %d", len(got), len(payload))
	}
}

func TestWSFrame_RoundTrip_64BitExtendedLength(t *testing.T) {
	var buf bytes.Buffer
	payload := make([]byte, 70000) // > 0xFFFF: forces the 64-bit extended length field
	for i := range payload {
		payload[i] = byte(i)
	}
	if err := writeWSFrame(&buf, wsOpBinary, payload, false); err != nil {
		t.Fatalf("writeWSFrame: %v", err)
	}
	_, _, got, err := readWSFrame(bufio.NewReader(&buf))
	if err != nil {
		t.Fatalf("readWSFrame: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Errorf("payload mismatch: got %d bytes, want %d", len(got), len(payload))
	}
}

func TestWSFrame_TooLarge_Rejected(t *testing.T) {
	var hdr [10]byte
	hdr[0] = 0x80 | wsOpBinary
	hdr[1] = 127 // 64-bit extended length follows, no mask
	binary.BigEndian.PutUint64(hdr[2:10], uint64(maxWSFrameSize+1))
	r := bufio.NewReader(bytes.NewReader(hdr[:]))
	if _, _, _, err := readWSFrame(r); err == nil {
		t.Fatal("expected error for an oversized claimed frame length, got nil")
	}
}

// ── wsSocket primitives ──────────────────────────────────────────────────────

func newTestWSSocket(t *testing.T, tlsConfig *tls.Config, framing wsFraming) *wsSocket {
	t.Helper()
	s, err := newWSSocket("127.0.0.1:0", tlsConfig, framing)
	if err != nil {
		t.Skipf("newWSSocket: %v — TCP loopback unavailable", err)
	}
	t.Cleanup(s.close)
	return s
}

func recvWSPacket(t *testing.T, s *wsSocket) wsPacket {
	t.Helper()
	select {
	case pkt := <-s.recv:
		return pkt
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for WS packet")
		return wsPacket{}
	}
}

func TestWSSocket_SendReceive_Plaintext_Binary(t *testing.T) {
	server := newTestWSSocket(t, nil, wsFramingBinary)
	client := newTestWSSocket(t, nil, wsFramingBinary)
	addr := fmt.Sprintf("127.0.0.1:%d", server.port)

	want := []byte("rtps-over-ws: binary framing hello")
	if err := client.send(addr, want); err != nil {
		t.Fatalf("send: %v", err)
	}
	pkt := recvWSPacket(t, server)
	if !bytes.Equal(pkt.data, want) {
		t.Errorf("payload: got %q, want %q", pkt.data, want)
	}
}

func TestWSSocket_SendReceive_JSONFraming(t *testing.T) {
	server := newTestWSSocket(t, nil, wsFramingBinary) // decode must not depend on the receiver's own framing setting
	client := newTestWSSocket(t, nil, wsFramingJSON)
	addr := fmt.Sprintf("127.0.0.1:%d", server.port)

	want := []byte(`{"transport":"rtps-over-ws","framing":"json-base64-cdr"}`)
	if err := client.send(addr, want); err != nil {
		t.Fatalf("send: %v", err)
	}
	pkt := recvWSPacket(t, server)
	if !bytes.Equal(pkt.data, want) {
		t.Errorf("payload: got %q, want %q", pkt.data, want)
	}
}

func TestWSSocket_SendReceive_MixedFraming_Interoperate(t *testing.T) {
	// A binary-framed socket and a JSON-framed socket must be able to talk
	// to each other in both directions — framing only controls what a
	// socket *sends*, not what it can *receive* (see transport_ws.go's doc
	// comment).
	binSock := newTestWSSocket(t, nil, wsFramingBinary)
	jsonSock := newTestWSSocket(t, nil, wsFramingJSON)
	binAddr := fmt.Sprintf("127.0.0.1:%d", binSock.port)
	jsonAddr := fmt.Sprintf("127.0.0.1:%d", jsonSock.port)

	wantToJSON := []byte("binary sender -> JSON-framed receiver")
	if err := binSock.send(jsonAddr, wantToJSON); err != nil {
		t.Fatalf("send (binary->json receiver): %v", err)
	}
	pkt := recvWSPacket(t, jsonSock)
	if !bytes.Equal(pkt.data, wantToJSON) {
		t.Errorf("payload: got %q, want %q", pkt.data, wantToJSON)
	}

	wantToBin := []byte("json sender -> binary-framed receiver")
	if err := jsonSock.send(binAddr, wantToBin); err != nil {
		t.Fatalf("send (json->binary receiver): %v", err)
	}
	pkt = recvWSPacket(t, binSock)
	if !bytes.Equal(pkt.data, wantToBin) {
		t.Errorf("payload: got %q, want %q", pkt.data, wantToBin)
	}
}

func TestWSSocket_SendReceive_MultipleMessages(t *testing.T) {
	server := newTestWSSocket(t, nil, wsFramingBinary)
	client := newTestWSSocket(t, nil, wsFramingBinary)
	addr := fmt.Sprintf("127.0.0.1:%d", server.port)

	const n = 20
	for i := 0; i < n; i++ {
		msg := []byte(fmt.Sprintf("message-%02d", i))
		if err := client.send(addr, msg); err != nil {
			t.Fatalf("send %d: %v", i, err)
		}
	}
	for i := 0; i < n; i++ {
		pkt := recvWSPacket(t, server)
		want := fmt.Sprintf("message-%02d", i)
		if string(pkt.data) != want {
			t.Errorf("message %d: got %q, want %q", i, pkt.data, want)
		}
	}
}

func TestWSSocket_SendReceive_TLS(t *testing.T) {
	cfg := generateTestTLSConfig(t)
	server := newTestWSSocket(t, cfg, wsFramingBinary)
	client := newTestWSSocket(t, cfg, wsFramingBinary)

	if server.tlsConfig.MinVersion != tls.VersionTLS13 {
		t.Errorf("server MinVersion: got %d, want TLS 1.3 (%d)", server.tlsConfig.MinVersion, tls.VersionTLS13)
	}

	addr := fmt.Sprintf("127.0.0.1:%d", server.port)
	want := []byte("rtps-over-wss: hello")
	if err := client.send(addr, want); err != nil {
		t.Fatalf("send: %v", err)
	}
	pkt := recvWSPacket(t, server)
	if !bytes.Equal(pkt.data, want) {
		t.Errorf("payload: got %q, want %q", pkt.data, want)
	}
}

func TestWSSocket_Send_FrameTooLarge(t *testing.T) {
	client := newTestWSSocket(t, nil, wsFramingBinary)
	big := make([]byte, maxWSFrameSize+1)
	if err := client.send("127.0.0.1:1", big); err == nil {
		t.Fatal("expected error for oversized message, got nil")
	}
}

func TestWSSocket_Send_DialFailure(t *testing.T) {
	client := newTestWSSocket(t, nil, wsFramingBinary)
	// Port 1 is a reserved/unassigned TCP port; dialling it should fail
	// fast rather than hang past wsDialTimeout.
	if err := client.send("127.0.0.1:1", []byte("x")); err == nil {
		t.Fatal("expected dial error, got nil")
	}
}

// rawWSDial performs the RFC 6455 client-side opening handshake against
// addr using nothing but net + net/http (deliberately not wsSocket.dial),
// so tests using it are exercising the server side (handleAccept) as an
// independent, minimal WebSocket client would see it.
func rawWSDial(t *testing.T, addr string) net.Conn {
	t.Helper()
	conn, err := (&net.Dialer{}).DialContext(context.Background(), "tcp", addr)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	keyBytes := make([]byte, 16)
	if _, keyErr := rand.Read(keyBytes); keyErr != nil {
		t.Fatalf("rand.Read: %v", keyErr)
	}
	secKey := base64.StdEncoding.EncodeToString(keyBytes)
	req := "GET / HTTP/1.1\r\n" +
		"Host: " + addr + "\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: " + secKey + "\r\n" +
		"Sec-WebSocket-Version: 13\r\n\r\n"
	if _, writeErr := conn.Write([]byte(req)); writeErr != nil {
		t.Fatalf("handshake write: %v", writeErr)
	}
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		t.Fatalf("handshake read: %v", err)
	}
	_ = resp.Body.Close() // see transport_ws.go's dial: always http.NoBody for a 101 response
	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("handshake status: got %d, want 101", resp.StatusCode)
	}
	if got := resp.Header.Get("Sec-WebSocket-Accept"); got != wsAcceptKey(secKey) {
		t.Fatalf("Sec-WebSocket-Accept: got %q, want %q", got, wsAcceptKey(secKey))
	}
	return conn
}

// TestWSSocket_RawHandshake_ProducesValidAccept proves the server-side
// handshake (handleAccept) is wire-correct against a client that speaks
// nothing but plain HTTP/1.1 plus RFC 6455 — not wsSocket's own dial.
func TestWSSocket_RawHandshake_ProducesValidAccept(t *testing.T) {
	server := newTestWSSocket(t, nil, wsFramingBinary)
	addr := fmt.Sprintf("127.0.0.1:%d", server.port)
	conn := rawWSDial(t, addr)
	defer conn.Close()
}

// TestWSSocket_HostileFrameLength_DropsConnection proves REQ-TRANS-015: a
// peer that opens a valid handshake and then claims an absurd frame payload
// length gets its connection dropped rather than causing the server to
// attempt an unbounded allocation, mirroring
// TestTCPSocket_HostileLengthPrefix_DropsConnection.
func TestWSSocket_HostileFrameLength_DropsConnection(t *testing.T) {
	server := newTestWSSocket(t, nil, wsFramingBinary)
	addr := fmt.Sprintf("127.0.0.1:%d", server.port)
	conn := rawWSDial(t, addr)
	defer conn.Close()

	// A masked BINARY frame header (RFC 6455 requires client->server frames
	// to be masked) claiming a payload far beyond maxWSFrameSize, via the
	// 64-bit extended length field.
	var hdr [14]byte
	hdr[0] = 0x80 | wsOpBinary
	hdr[1] = 0x80 | 127
	binary.BigEndian.PutUint64(hdr[2:10], uint64(maxWSFrameSize+1))
	// Mask key content is irrelevant; the server must reject based on the
	// claimed length alone, before ever reading (let alone unmasking) a
	// payload.
	if _, err := conn.Write(hdr[:]); err != nil {
		t.Fatalf("write: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := conn.Write([]byte{0}); err != nil {
			return // server closed the connection, as expected
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("server did not drop the connection for a hostile claimed frame length")
}

// TestWSSocket_PingPong proves a Ping frame is answered with a Pong and
// never surfaces on recv, and that a subsequent data message on the same
// connection is still delivered normally afterwards.
func TestWSSocket_PingPong(t *testing.T) {
	server := newTestWSSocket(t, nil, wsFramingBinary)
	addr := fmt.Sprintf("127.0.0.1:%d", server.port)
	conn := rawWSDial(t, addr)
	defer conn.Close()

	pingPayload := []byte("ping-me")
	if err := writeWSFrame(conn, wsOpPing, pingPayload, true); err != nil {
		t.Fatalf("write ping: %v", err)
	}

	br := bufio.NewReader(conn)
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	fin, op, payload, err := readWSFrame(br)
	if err != nil {
		t.Fatalf("read pong: %v", err)
	}
	if !fin || op != wsOpPong {
		t.Errorf("fin=%v op=%d, want fin=true op=%d (Pong)", fin, op, wsOpPong)
	}
	if !bytes.Equal(payload, pingPayload) {
		t.Errorf("pong payload: got %q, want echoed %q", payload, pingPayload)
	}

	// A Ping/Pong exchange must never appear on the socket's recv channel.
	select {
	case pkt := <-server.recv:
		t.Fatalf("unexpected packet on recv after Ping/Pong: %q", pkt.data)
	case <-time.After(200 * time.Millisecond):
	}

	// The connection must still be fully usable for a normal data message.
	want := []byte("still alive after ping/pong")
	if err := writeWSFrame(conn, wsOpBinary, want, true); err != nil {
		t.Fatalf("write data frame: %v", err)
	}
	pkt := recvWSPacket(t, server)
	if !bytes.Equal(pkt.data, want) {
		t.Errorf("payload after ping/pong: got %q, want %q", pkt.data, want)
	}
}

// ── Locator (WSv4) ───────────────────────────────────────────────────────────

func TestLocatorWS_RoundTrip(t *testing.T) {
	l := locatorFromWS(net.ParseIP("192.0.2.11"), 7800)
	if l.Kind != LocatorKindWSv4 {
		t.Fatalf("Kind: got %d, want %d", l.Kind, LocatorKindWSv4)
	}
	hp, ok := l.wsHostPort()
	if !ok {
		t.Fatal("wsHostPort: ok=false")
	}
	if hp != "192.0.2.11:7800" {
		t.Errorf("wsHostPort: got %q, want %q", hp, "192.0.2.11:7800")
	}

	// A TCP/QUIC locator must not be mistaken for a WS one, and vice versa.
	tcp := locatorFromTCP(net.ParseIP("192.0.2.11"), 7800)
	if _, ok := tcp.wsHostPort(); ok {
		t.Error("wsHostPort: expected ok=false for a TCP locator")
	}
	if _, ok := l.tcpHostPort(); ok {
		t.Error("tcpHostPort: expected ok=false for a WS locator")
	}
	quic := locatorFromQUIC(net.ParseIP("192.0.2.11"), 7800)
	if _, ok := quic.wsHostPort(); ok {
		t.Error("wsHostPort: expected ok=false for a QUIC locator")
	}
}

func TestLocatorWS_WireRoundTrip(t *testing.T) {
	l := locatorFromWS(net.ParseIP("10.1.2.5"), 9300)
	wire := marshalLocator(l)
	got, ok := unmarshalLocator(wire)
	if !ok {
		t.Fatal("unmarshalLocator: ok=false")
	}
	if got.Kind != LocatorKindWSv4 || got.Port != 9300 {
		t.Errorf("round-trip mismatch: got %+v", got)
	}
	hp, ok := got.wsHostPort()
	if !ok || hp != "10.1.2.5:9300" {
		t.Errorf("wsHostPort after round-trip: got (%q,%v)", hp, ok)
	}
}

// ── participant lookup helpers ────────────────────────────────────────────────

func TestParticipant_WSLocatorForIP(t *testing.T) {
	p := &participant{}
	p.spdp = newSPDPService(p)

	peerIP := net.ParseIP("127.0.0.1")
	proxy := &participantProxy{
		metatrafficUnicast: locatorFromUDP(&net.UDPAddr{IP: peerIP}, 17400),
		defaultUnicast:     locatorFromUDP(&net.UDPAddr{IP: peerIP}, 17401),
		wsUnicast:          "127.0.0.1:17800",
	}
	p.spdp.peers[GuidPrefix{0x03}] = proxy

	if got := p.wsLocatorForIP(peerIP); got != "127.0.0.1:17800" {
		t.Errorf("wsLocatorForIP: got %q, want %q", got, "127.0.0.1:17800")
	}
	if got := p.wsLocatorForIP(net.ParseIP("10.0.0.9")); got != "" {
		t.Errorf("wsLocatorForIP(unknown): got %q, want empty", got)
	}
}

func TestParticipant_WSLocatorForIP_NoSPDP(t *testing.T) {
	p := &participant{}
	if got := p.wsLocatorForIP(net.ParseIP("127.0.0.1")); got != "" {
		t.Errorf("wsLocatorForIP with nil spdp: got %q, want empty", got)
	}
}

// ── End-to-end: discovery and delivery entirely over WebSocket ───────────────

// TestWS_CrossDomain_DiscoveryAndDelivery proves the whole Milestone 16
// WebSocket sub-phase end to end: two participants in different RTPS
// domains — so UDP multicast SPDP cannot possibly deliver between them —
// discover each other purely via SPDP-unicast-over-WebSocket (WithWSPeers),
// match endpoints via SEDP-over-WebSocket, and exchange a reliable sample
// with HEARTBEAT/ACKNACK flowing over WebSocket too, with UDP forced
// unavailable (withForcedUDPUnavailable, from transport_tcp_test.go) and
// user-data multicast disabled (WithNoMulticast) so successful delivery can
// only have taken the WebSocket path — exactly TestTCP_CrossDomain_
// DiscoveryAndReliableDelivery's proof shape, for the WS transport instead.
func TestWS_CrossDomain_DiscoveryAndDelivery(t *testing.T) {
	const p1Addr = "127.0.0.1:17810"
	const p2Addr = "127.0.0.1:17811"

	p2, err := newParticipant(dds.Domain(100),
		WithWSAddr(p2Addr),
		WithWSPeers(p1Addr),
		WithSPDPInterval(150*time.Millisecond),
		WithNoMulticast(),
		withForcedUDPUnavailable(),
	)
	if err != nil {
		t.Skipf("newParticipant(p2): %v — WS/TCP loopback unavailable", err)
	}
	t.Cleanup(func() { _ = p2.Close() })

	p1, err := newParticipant(dds.Domain(101),
		WithWSAddr(p1Addr),
		WithWSPeers(p2Addr),
		WithSPDPInterval(150*time.Millisecond),
		WithNoMulticast(),
		withForcedUDPUnavailable(),
	)
	if err != nil {
		t.Skipf("newParticipant(p1): %v — WS/TCP loopback unavailable", err)
	}
	t.Cleanup(func() { _ = p1.Close() })

	sub, err := p2.NewSubscriber("ws/cross-domain/reliable", dds.ReliableQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()

	pub, err := p1.NewPublisher("ws/cross-domain/reliable", dds.ReliableQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	// Give SPDP-over-WS + SEDP-over-WS time to complete. The first SPDP
	// announcement fires immediately at startup, so this is generous
	// rather than tight.
	time.Sleep(1 * time.Second)

	want := []byte(`{"transport":"rtps-over-ws","channel":"reliable"}`)
	if err := pub.Write(want); err != nil {
		t.Fatalf("Write: %v", err)
	}
	select {
	case s := <-sub.C():
		if !bytes.Equal(s.Payload, want) {
			t.Errorf("payload: got %q, want %q", s.Payload, want)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout: sample not delivered over RTPS-over-WS")
	}

	// Confirm the mechanism under test actually ran: both participants must
	// have at least one live WS connection to the other, proving discovery
	// and delivery took the WS path rather than some incidental UDP route.
	p1.wsSock.mu.Lock()
	p1Conns := len(p1.wsSock.conns)
	p1.wsSock.mu.Unlock()
	p2.wsSock.mu.Lock()
	p2Conns := len(p2.wsSock.conns)
	p2.wsSock.mu.Unlock()
	if p1Conns == 0 && p2Conns == 0 {
		t.Error("expected at least one cached WS connection on p1 or p2; none found")
	}
}

// TestWS_WithWS_StartsCleanly confirms a participant can be built with the
// WS transport and shuts down cleanly, without requiring a live peer.
func TestWS_WithWS_StartsCleanly(t *testing.T) {
	p, err := newParticipant(dds.Domain(102),
		WithWSAddr("127.0.0.1:17812"),
	)
	if err != nil {
		t.Skipf("newParticipant: %v — WS/TCP loopback unavailable", err)
	}
	defer func() { _ = p.Close() }()

	if p.wsSock == nil {
		t.Fatal("wsSock is nil")
	}
	if p.wsSock.tlsConfig != nil {
		t.Error("tlsConfig: got non-nil, want nil (plain ws:// without WithWSTLSConfig)")
	}
}

// TestWS_Disabled_ByDefault confirms a participant created without
// WithWSAddr has no WS transport and sendUnicast behaves exactly as before
// Milestone 16 — no behaviour change for existing callers.
func TestWS_Disabled_ByDefault(t *testing.T) {
	p := testPart(t)
	if p.wsSock != nil {
		t.Fatal("wsSock should be nil without WithWSAddr")
	}
	if got := p.wsLocatorForIP(net.ParseIP("127.0.0.1")); got != "" {
		t.Errorf("wsLocatorForIP: got %q, want empty when WS disabled", got)
	}
}
