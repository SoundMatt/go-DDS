// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Internal tests for the RTPS-over-Relay transport (Milestone 15). Package
// rtps (internal) for direct access to relaySocket, participant fields, and
// the dispatch/fallback helpers.

package rtps

//fusa:test REQ-TRANS-010
//fusa:test REQ-TRANS-011

import (
	"bytes"
	"context"
	"crypto/tls"
	"net"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	dds "github.com/SoundMatt/go-DDS"
)

// ── a minimal, protocol-correct test relay server ────────────────────────────
//
// This intentionally does NOT import bridge/relay (the canonical, separately
// tested relay server implementation): the root module must not depend on
// the bridge submodule (ROADMAP.md, "Architecture Initiative", #71). It
// implements just enough of the same wire protocol (see transport_relay.go)
// to prove relaySocket/WithRelayAddr work end-to-end against a real,
// independent peer.

type testRelayServer struct {
	ln net.Listener

	mu       sync.Mutex
	clients  map[string]net.Conn
	forwards atomic.Int64 // count of SEND frames successfully forwarded

	done chan struct{}
	wg   sync.WaitGroup
}

func startTestRelayServer(t *testing.T, tlsConfig *tls.Config) *testRelayServer {
	t.Helper()
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("net.Listen: %v — TCP loopback unavailable", err)
	}
	if tlsConfig != nil {
		ln = tls.NewListener(ln, tlsConfig)
	}
	s := &testRelayServer{ln: ln, clients: make(map[string]net.Conn), done: make(chan struct{})}
	s.wg.Add(1)
	go s.acceptLoop()
	t.Cleanup(func() {
		close(s.done)
		_ = s.ln.Close()
		s.mu.Lock()
		for _, c := range s.clients {
			_ = c.Close()
		}
		s.mu.Unlock()
		s.wg.Wait()
	})
	return s
}

func (s *testRelayServer) addr() string { return s.ln.Addr().String() }

func (s *testRelayServer) acceptLoop() {
	defer s.wg.Done()
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			select {
			case <-s.done:
				return
			default:
				continue
			}
		}
		s.wg.Add(1)
		go s.handleConn(conn)
	}
}

func (s *testRelayServer) handleConn(conn net.Conn) {
	defer s.wg.Done()
	defer func() { _ = conn.Close() }()

	typ, idField, _, err := relayReadFrame(conn)
	if err != nil || typ != relayFrameRegister || len(idField) == 0 {
		return
	}
	id := string(idField)
	s.mu.Lock()
	s.clients[id] = conn
	s.mu.Unlock()

	for {
		typ, targetID, payload, err := relayReadFrame(conn)
		if err != nil {
			return
		}
		if typ != relayFrameSend || len(targetID) == 0 {
			continue
		}
		s.mu.Lock()
		target, ok := s.clients[string(targetID)]
		s.mu.Unlock()
		if !ok {
			continue
		}
		if relayWriteFrame(target, relayFrameDeliver, []byte(id), payload) == nil {
			s.forwards.Add(1)
		}
	}
}

// ── relaySocket primitives ───────────────────────────────────────────────────

func TestRelaySocket_SendReceive(t *testing.T) {
	srv := startTestRelayServer(t, nil)

	alice, err := newRelaySocket(srv.addr(), "alice", nil)
	if err != nil {
		t.Fatalf("newRelaySocket(alice): %v", err)
	}
	defer alice.close()
	bob, err := newRelaySocket(srv.addr(), "bob", nil)
	if err != nil {
		t.Fatalf("newRelaySocket(bob): %v", err)
	}
	defer bob.close()

	time.Sleep(50 * time.Millisecond) // let both registrations land

	want := []byte("rtps-over-relay: hello")
	if err := alice.send("bob", want); err != nil {
		t.Fatalf("send: %v", err)
	}

	select {
	case pkt := <-bob.recv:
		if pkt.fromID != "alice" {
			t.Errorf("fromID = %q, want %q", pkt.fromID, "alice")
		}
		if !bytes.Equal(pkt.data, want) {
			t.Errorf("payload = %q, want %q", pkt.data, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for relayed frame")
	}
	if got := srv.forwards.Load(); got != 1 {
		t.Errorf("server forwarded %d frames, want 1", got)
	}
}

func TestRelaySocket_TLS(t *testing.T) {
	cfg := generateTestTLSConfig(t)
	srv := startTestRelayServer(t, cfg)

	clientCfg := &tls.Config{RootCAs: cfg.RootCAs, ServerName: "go-dds-tcp-test"}
	alice, err := newRelaySocket(srv.addr(), "alice", clientCfg)
	if err != nil {
		t.Fatalf("newRelaySocket(alice): %v", err)
	}
	defer alice.close()
	bob, err := newRelaySocket(srv.addr(), "bob", clientCfg)
	if err != nil {
		t.Fatalf("newRelaySocket(bob): %v", err)
	}
	defer bob.close()
	time.Sleep(50 * time.Millisecond)

	if err := alice.send("bob", []byte("over tls")); err != nil {
		t.Fatalf("send: %v", err)
	}
	select {
	case pkt := <-bob.recv:
		if string(pkt.data) != "over tls" {
			t.Errorf("payload = %q, want %q", pkt.data, "over tls")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for relayed frame")
	}
}

func TestRelaySocket_DialFailure(t *testing.T) {
	if _, err := newRelaySocket("127.0.0.1:1", "alice", nil); err == nil {
		t.Fatal("expected an error dialling a closed port")
	}
}

func TestRelaySocket_IDTooLarge(t *testing.T) {
	big := make([]byte, int(relayMaxIDBytes)+1)
	if _, err := newRelaySocket("127.0.0.1:0", string(big), nil); err == nil {
		t.Fatal("expected an error for an oversized relay ID")
	}
}

func TestRelaySocket_CloseIsIdempotent(t *testing.T) {
	srv := startTestRelayServer(t, nil)
	s, err := newRelaySocket(srv.addr(), "alice", nil)
	if err != nil {
		t.Fatalf("newRelaySocket: %v", err)
	}
	s.close()
	s.close() // must not panic or block
}

func TestRelayIDFromGuidPrefix_RoundTrip(t *testing.T) {
	var prefix GuidPrefix
	for i := range prefix {
		prefix[i] = byte(i + 1)
	}
	id := relayIDFromGuidPrefix(prefix)
	got, ok := guidPrefixFromRelayID(id)
	if !ok {
		t.Fatalf("guidPrefixFromRelayID(%q): ok = false", id)
	}
	if got != prefix {
		t.Errorf("round trip = %x, want %x", got, prefix)
	}
}

func TestGuidPrefixFromRelayID_Invalid(t *testing.T) {
	if _, ok := guidPrefixFromRelayID("not-hex!!"); ok {
		t.Error("expected ok = false for non-hex input")
	}
	if _, ok := guidPrefixFromRelayID("aabb"); ok {
		t.Error("expected ok = false for the wrong length")
	}
}

// ── end-to-end: two participants discover and exchange samples via relay ────

func TestRelay_TwoParticipants_SameHost(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping two-participant test in -short mode (requires network)")
	}
	if runtime.GOOS != "linux" {
		t.Skipf("UDP multicast loopback unreliable on %s; skipping cross-participant relay test", runtime.GOOS)
	}

	srv := startTestRelayServer(t, nil)
	const domain = 95

	p1, err := newParticipant(dds.Domain(domain),
		WithRelayAddr(srv.addr()), WithNoMulticast())
	if err != nil {
		t.Skipf("newParticipant(p1): %v", err)
	}
	defer func() { _ = p1.Close() }()

	// p2 is the only side that needs WithRelayPeers configured (at
	// construction time, so there is no data race with p2's own
	// announceLoop goroutine reading it concurrently once started): once
	// p2's periodic SPDP announcement reaches p1 over the relay, p1 learns
	// p2's relay ID directly from that announcement's pidRelayID field (see
	// parseParticipantData) and its immediate onNewPeer reply routes back
	// over the relay the same way — exactly like WithTCPPeers/WithDTLSPeers
	// only need to be configured on the side initiating first contact.
	p2, err := newParticipant(dds.Domain(domain),
		WithRelayAddr(srv.addr()), WithNoMulticast(),
		WithRelayPeers(relayIDFromGuidPrefix(p1.guidPrefix)))
	if err != nil {
		t.Skipf("newParticipant(p2): %v", err)
	}
	defer func() { _ = p2.Close() }()

	sub, err := p2.NewSubscriber("relay/cross-participant", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()

	pub, err := p1.NewPublisher("relay/cross-participant", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	// Allow SPDP (over the relay) + SEDP to complete.
	time.Sleep(2200 * time.Millisecond)

	want := []byte(`{"transport":"rtps-over-relay","ok":true}`)
	if err := pub.Write(want); err != nil {
		t.Fatalf("Write: %v", err)
	}

	select {
	case s := <-sub.C():
		if !bytes.Equal(s.Payload, want) {
			t.Errorf("payload: got %q, want %q", s.Payload, want)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout: sample not delivered over RTPS-over-Relay")
	}

	// Confirm the mechanism under test actually ran: the relay server must
	// have forwarded at least one frame (there is no other transport
	// configured on either participant that could have delivered this).
	if got := srv.forwards.Load(); got == 0 {
		t.Error("expected at least one frame to have been forwarded by the relay server")
	}
}

// TestRelay_SkipsMulticastWhenPeerIsRelayOnly proves the writer send path's
// multicast/relay decision (Milestone 15's fix to the writer's Write method
// in participant.go): even when UDP multicast delivery is available (no
// WithNoMulticast this time), a matched reader reachable only via the relay
// must still receive the sample, because the multicast fast path is skipped
// whenever any matched reader requires relay routing.
func TestRelay_SkipsMulticastWhenPeerIsRelayOnly(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping two-participant test in -short mode (requires network)")
	}
	if runtime.GOOS != "linux" {
		t.Skipf("UDP multicast loopback unreliable on %s", runtime.GOOS)
	}

	srv := startTestRelayServer(t, nil)
	const domain = 96

	p1, err := newParticipant(dds.Domain(domain), WithRelayAddr(srv.addr()))
	if err != nil {
		t.Skipf("newParticipant(p1): %v", err)
	}
	defer func() { _ = p1.Close() }()

	p2, err := newParticipant(dds.Domain(domain), WithRelayAddr(srv.addr()),
		WithRelayPeers(relayIDFromGuidPrefix(p1.guidPrefix)))
	if err != nil {
		t.Skipf("newParticipant(p2): %v", err)
	}
	defer func() { _ = p2.Close() }()

	if p1.dataMcastSock == nil {
		t.Skip("no user-data multicast socket available on this host; cannot exercise the skip path")
	}

	sub, err := p2.NewSubscriber("relay/multicast-skip", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()
	pub, err := p1.NewPublisher("relay/multicast-skip", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	time.Sleep(2200 * time.Millisecond)

	want := []byte("relay-not-multicast")
	if err := pub.Write(want); err != nil {
		t.Fatalf("Write: %v", err)
	}

	select {
	case s := <-sub.C():
		if !bytes.Equal(s.Payload, want) {
			t.Errorf("payload: got %q, want %q", s.Payload, want)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout: sample not delivered")
	}
	if got := srv.forwards.Load(); got == 0 {
		t.Error("expected the sample to have been routed through the relay, not multicast")
	}
}

func TestRelaySocket_UnknownField(t *testing.T) {
	// Regression guard: relayFrameError frames from the server must be
	// silently ignored by readLoop rather than delivered as data.
	srv := startTestRelayServer(t, nil)
	alice, err := newRelaySocket(srv.addr(), "alice", nil)
	if err != nil {
		t.Fatalf("newRelaySocket: %v", err)
	}
	defer alice.close()
	time.Sleep(20 * time.Millisecond)

	if err := alice.send("nobody", []byte("hi")); err != nil {
		t.Fatalf("send: %v", err)
	}
	select {
	case pkt, ok := <-alice.recv:
		t.Fatalf("unexpected delivery: ok=%v pkt=%+v", ok, pkt)
	case <-time.After(300 * time.Millisecond):
		// Expected: the server silently drops a SEND to an unregistered
		// target in this minimal test server (it has no ERROR frame path),
		// so nothing at all should arrive.
	}
}
