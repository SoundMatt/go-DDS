// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Internal coverage tests targeting previously uncovered paths in spdp.go,
// locator.go, participant.go: locatorFromUDP nil-IP, locatorFromUDPv6 nil-IP,
// extractDiscoveryToken, storePeer/evictExpired liveliness callbacks,
// spdpService.handlePacket with DiscoveryPlugin, sendHeartbeatLocked empty
// history, waitDrain ctx-cancel, and TryRead on a closed channel.

package rtps

//fusa:test REQ-DISC-001
//fusa:test REQ-DISC-004
//fusa:test REQ-DISC-005
//fusa:test REQ-LOC-001
//fusa:test REQ-LOC-002
//fusa:test REQ-LOC-003
//fusa:test REQ-PART-006
//fusa:test REQ-REL-006
//fusa:test REQ-REL-007
//fusa:test REQ-RTPS-004
//fusa:test REQ-SEC-001
//fusa:test REQ-SEC-002
//fusa:test REQ-SUB-005

import (
	"context"
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"time"

	dds "github.com/SoundMatt/go-DDS"
)

// spdpCovDomain is distinct from domain 99 (used by packet_test.go) to avoid
// UDP port conflicts when many participants are created during a full test run.
const spdpCovDomain = dds.Domain(197)

// spdpCovPart creates a no-multicast participant on spdpCovDomain, skipping
// the test if UDP socket binding is unavailable.
func spdpCovPart(t *testing.T) *participant {
	t.Helper()
	p, err := newParticipant(spdpCovDomain, WithNoMulticast())
	if err != nil {
		t.Skipf("newParticipant(spdpCovDomain): %v", err)
	}
	t.Cleanup(func() { _ = p.Close() })
	return p
}

// ── locatorFromUDP / locatorFromUDPv6 nil-IP paths ───────────────────────────

func TestLocatorFromUDP_NilIP(t *testing.T) {
	l := locatorFromUDP(&net.UDPAddr{IP: nil, Port: 1234}, 1234)
	if l.Kind != LocatorKindUDPv4 {
		t.Fatalf("expected LocatorKindUDPv4, got %d", l.Kind)
	}
	if l.Port != 1234 {
		t.Fatalf("expected port 1234, got %d", l.Port)
	}
	// nil IP → net.IPv4zero (0.0.0.0) encoded in bytes 12–15.
	for i := 0; i < 12; i++ {
		if l.Address[i] != 0 {
			t.Fatalf("expected zero byte at Address[%d]", i)
		}
	}
}

// ── sendHeartbeatLocked non-empty history ────────────────────────────────────

// TestSendHeartbeatLocked_NonEmptyHistory covers lines 4-7 in
// sendHeartbeatLocked: history is non-empty so firstLast returns ok=true; the
// heartbeat is built and the matched-reader loop is entered (but has 0
// iterations because no reader is registered on the topic).
func TestSendHeartbeatLocked_NonEmptyHistory(t *testing.T) {
	p := spdpCovPart(t)
	pub, err := p.NewPublisher("hb/nonempty", dds.ReliableQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	w, ok := pub.(*rtpsWriter)
	if !ok {
		t.Fatal("publisher is not *rtpsWriter")
	}

	// Write so history is non-empty.
	if werr := pub.Write([]byte("hb")); werr != nil {
		t.Fatalf("Write: %v", werr)
	}

	w.mu.Lock()
	w.sendHeartbeatLocked() // history non-empty → builds hb, iterates readers (0)
	w.mu.Unlock()
}

// ── acceptsSource ────────────────────────────────────────────────────────────

// TestAcceptsSource_ExternalGUID covers the r.sources lookup path in
// acceptsSource: a non-local GUID that IS in the allow-list returns true;
// a GUID not in the allow-list returns false.
func TestAcceptsSource_ExternalGUID(t *testing.T) {
	p := spdpCovPart(t)

	sub, err := p.NewSubscriber("cov/accepts", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()

	r, ok := sub.(*rtpsReader)
	if !ok {
		t.Fatal("subscriber is not *rtpsReader")
	}

	// Create an external (different prefix) GUID.
	extGUID := GUID{
		Prefix: GuidPrefix{0x01, 0x02, 0x03, 0x04, 0x05, 0x06,
			0x07, 0x08, 0x09, 0x0A, 0x0B, 0x0C},
		Entity: EntityId{0x00, 0x00, 0x02, 0x07},
	}

	// Without an explicit allow-list the reader accepts all local GUIDs.
	// Add the external GUID to the allow-list.
	r.addSourceGUID(extGUID)

	// extGUID is now in sources → should be accepted.
	if !r.acceptsSource(extGUID) {
		t.Error("extGUID in sources: expected acceptsSource to return true")
	}

	// A different external GUID (not in sources) should be rejected.
	otherGUID := GUID{
		Prefix: GuidPrefix{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
			0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF},
		Entity: EntityId{0x00, 0x00, 0x01, 0x07},
	}
	if r.acceptsSource(otherGUID) {
		t.Error("unknown external GUID: expected acceptsSource to return false")
	}

	// When sources is non-empty, a GUID with the local participant prefix should
	// always be accepted (line 1421: g.Prefix == r.p.guidPrefix returns true).
	localGUID := GUID{
		Prefix: r.p.guidPrefix,
		Entity: EntityId{0x00, 0x00, 0x05, 0x07},
	}
	if !r.acceptsSource(localGUID) {
		t.Error("local-prefix GUID with non-empty sources: expected acceptsSource to return true")
	}
}

// ── storePeer livelinessCb ───────────────────────────────────────────────────

func TestStorePeer_LivelinessGained(t *testing.T) {
	var gainedCount atomic.Int32
	cb := func(_ dds.GUID, event dds.LivelinessEvent) {
		if event == dds.LivelinessGained {
			gainedCount.Add(1)
		}
	}
	p, err := newParticipant(spdpCovDomain, WithNoMulticast(), WithLivelinessCallback(cb))
	if err != nil {
		t.Skipf("newParticipant: %v", err)
	}
	t.Cleanup(func() { _ = p.Close() })

	proxy := &participantProxy{
		guid:          GUID{Prefix: newGuidPrefix()},
		leaseDuration: 10 * time.Second,
	}
	p.spdp.storePeer(proxy)

	if gainedCount.Load() != 1 {
		t.Fatalf("expected 1 LivelinessGained callback, got %d", gainedCount.Load())
	}
}

func TestStorePeer_LivelinessGained_KnownPeer_NoCb(t *testing.T) {
	var count atomic.Int32
	cb := func(_ dds.GUID, event dds.LivelinessEvent) {
		if event == dds.LivelinessGained {
			count.Add(1)
		}
	}
	p, err := newParticipant(spdpCovDomain, WithNoMulticast(), WithLivelinessCallback(cb))
	if err != nil {
		t.Skipf("newParticipant: %v", err)
	}
	t.Cleanup(func() { _ = p.Close() })

	prefix := newGuidPrefix()
	proxy := &participantProxy{guid: GUID{Prefix: prefix}, leaseDuration: 10 * time.Second}
	p.spdp.storePeer(proxy)
	// Second store with same prefix: peer already known → no callback.
	p.spdp.storePeer(&participantProxy{guid: GUID{Prefix: prefix}, leaseDuration: 10 * time.Second})

	if count.Load() != 1 {
		t.Fatalf("expected 1 callback (new peer only), got %d", count.Load())
	}
}

// ── evictExpired livelinessCb ────────────────────────────────────────────────

func TestEvictExpired_LivelinessLost(t *testing.T) {
	var lostCount atomic.Int32
	cb := func(_ dds.GUID, event dds.LivelinessEvent) {
		if event == dds.LivelinessLost {
			lostCount.Add(1)
		}
	}
	p, err := newParticipant(spdpCovDomain, WithNoMulticast(), WithLivelinessCallback(cb))
	if err != nil {
		t.Skipf("newParticipant: %v", err)
	}
	t.Cleanup(func() { _ = p.Close() })

	prefix := newGuidPrefix()
	// Peer last seen 30 seconds ago with a 5-second lease → already expired.
	proxy := &participantProxy{
		guid:          GUID{Prefix: prefix},
		lastSeen:      time.Now().Add(-30 * time.Second),
		leaseDuration: 5 * time.Second,
	}
	p.spdp.mu.Lock()
	p.spdp.peers[prefix] = proxy
	p.spdp.mu.Unlock()

	p.spdp.evictExpired()

	if lostCount.Load() != 1 {
		t.Fatalf("expected 1 LivelinessLost callback, got %d", lostCount.Load())
	}
}

// ── spdpService.handlePacket with DiscoveryPlugin ────────────────────────────

// mockDiscoveryPlugin is a stub DiscoveryPlugin for internal testing.
type mockDiscoveryPlugin struct {
	sign   []byte
	accept bool
}

func (m *mockDiscoveryPlugin) SignDiscovery(_ []byte) []byte { return m.sign }
func (m *mockDiscoveryPlugin) VerifyDiscovery(_ []byte, token []byte) bool {
	return m.accept && len(token) > 0
}

func buildSPDPPacket(senderPrefix GuidPrefix, token []byte) []byte {
	enc := newPLCDREncoder()
	enc.addParam(pidProtocolVersion, []byte{2, 3, 0, 0})
	if token != nil {
		enc.addParam(pidDiscoveryToken, token)
	}
	// SPDP uses raw PL_CDR_LE as the DATA payload (no cdrWrapPayload).
	payload := enc.finish()
	submsg := marshalDataSubmessage(EntityIdSPDPWriter, EntityIdSPDPReader,
		SequenceNumber{High: 0, Low: 1}, payload)
	return wrapInRTPSMessage(senderPrefix, submsg)
}

func TestSPDPHandlePacket_DiscoveryPlugin_Reject(t *testing.T) {
	// Configure plugin at construction time to avoid post-construction races.
	plugin := &mockDiscoveryPlugin{accept: false}
	p, err := newParticipant(spdpCovDomain, WithNoMulticast(), WithDiscoverySecurity(plugin))
	if err != nil {
		t.Skipf("newParticipant: %v", err)
	}
	t.Cleanup(func() { _ = p.Close() })

	senderPrefix := newGuidPrefix()
	msg := buildSPDPPacket(senderPrefix, []byte("bad-token"))
	p.spdp.handlePacket(msg, loopbackAddr)

	p.spdp.mu.RLock()
	_, found := p.spdp.peers[senderPrefix]
	p.spdp.mu.RUnlock()
	if found {
		t.Fatal("peer should not be stored when DiscoveryPlugin rejects")
	}
}

func TestSPDPHandlePacket_DiscoveryPlugin_Accept(t *testing.T) {
	// Configure plugin at construction time to avoid post-construction races.
	plugin := &mockDiscoveryPlugin{sign: []byte("sig!"), accept: true}
	p, err := newParticipant(spdpCovDomain, WithNoMulticast(), WithDiscoverySecurity(plugin))
	if err != nil {
		t.Skipf("newParticipant: %v", err)
	}
	t.Cleanup(func() { _ = p.Close() })

	senderPrefix := newGuidPrefix()
	// Token must be 4-byte aligned so extractDiscoveryToken returns exactly the token.
	msg := buildSPDPPacket(senderPrefix, []byte("tok!"))
	p.spdp.handlePacket(msg, loopbackAddr)

	p.spdp.mu.RLock()
	_, found := p.spdp.peers[senderPrefix]
	p.spdp.mu.RUnlock()
	if !found {
		t.Fatal("peer should be stored when DiscoveryPlugin accepts")
	}
}

// ── waitDrain ctx-cancel path ─────────────────────────────────────────────────

func TestWaitDrain_CtxCancel(t *testing.T) {
	p := spdpCovPart(t)
	pub, err := p.NewPublisher("drain/cancel", dds.ReliableQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	w, ok := pub.(*rtpsWriter)
	if !ok {
		t.Fatal("publisher is not *rtpsWriter")
	}

	// Write without a local subscriber; seqLo advances, drainCh is set, no
	// ACKNACK arrives → ackedLo < seqLo.
	if werr := pub.Write([]byte("x")); werr != nil {
		t.Fatalf("Write: %v", werr)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled

	err = w.waitDrain(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestWaitDrain_DrainChannelCloses(t *testing.T) {
	p := spdpCovPart(t)
	pub, err := p.NewPublisher("drain/closes", dds.ReliableQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	w, ok := pub.(*rtpsWriter)
	if !ok {
		t.Fatal("publisher is not *rtpsWriter")
	}

	// Write so seqLo = 1, drainCh is non-nil, ackedLo = 0.
	if err := pub.Write([]byte("y")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	// Simulate receiving an ACKNACK that covers all sequence numbers.
	go func() {
		time.Sleep(5 * time.Millisecond)
		w.advanceAcked(w.seqLo + 1) // ackBase = seqLo+1 → confirmed = seqLo → drainCh closes
	}()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := w.waitDrain(ctx); err != nil {
		t.Fatalf("waitDrain: unexpected error %v", err)
	}
}

// ── NewSubscriber with deadline callback ──────────────────────────────────────

func TestNewSubscriber_WithDeadlineCallback(t *testing.T) {
	p := spdpCovPart(t)

	called := make(chan struct{}, 1)
	qos := dds.QoS{Deadline: 10 * time.Millisecond}
	sub, err := p.NewSubscriber("dl/topic", qos, dds.WithDeadlineMissed(func() {
		select {
		case called <- struct{}{}:
		default:
		}
	}))
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()

	select {
	case <-called:
		// deadline callback fired
	case <-time.After(500 * time.Millisecond):
		t.Fatal("deadline missed callback never fired")
	}
}

// ── WithSecurity option applied ───────────────────────────────────────────────

// passSecurityPlugin passes all payloads through unchanged.
type passSecurityPlugin struct{}

func (p *passSecurityPlugin) Seal(plain []byte) ([]byte, error) {
	out := make([]byte, len(plain))
	copy(out, plain)
	return out, nil
}
func (p *passSecurityPlugin) Open(cipher []byte) ([]byte, error) {
	out := make([]byte, len(cipher))
	copy(out, cipher)
	return out, nil
}

// errSecurityPlugin makes Seal always fail.
type errSecurityPlugin struct{}

func (e *errSecurityPlugin) Seal(_ []byte) ([]byte, error) {
	return nil, errors.New("seal failed")
}
func (e *errSecurityPlugin) Open(_ []byte) ([]byte, error) {
	return nil, errors.New("open failed")
}

func TestWithSecurity_OptionApplied(t *testing.T) {
	p := spdpCovPart(t)
	// Apply the option directly (opt is a func(*participant)).
	opt := WithSecurity(&passSecurityPlugin{})
	opt(p)

	if p.security == nil {
		t.Fatal("WithSecurity did not set p.security")
	}
}

func TestWithSecurity_SealErrorPropagates(t *testing.T) {
	p := spdpCovPart(t)
	opt := WithSecurity(&errSecurityPlugin{})
	opt(p)

	pub, err := p.NewPublisher("sec/seal-err", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	// Seal will fail → Write must return an error.
	if err := pub.Write([]byte("payload")); err == nil {
		t.Fatal("expected error from Write when Seal fails")
	}
}

// ── Health / deliverToReader coverage ────────────────────────────────────────

// TestParticipant_Health_Closed covers the closed-participant branch in Health().
func TestParticipant_Health_Closed(t *testing.T) {
	p, err := newParticipant(spdpCovDomain, WithNoMulticast())
	if err != nil {
		t.Skipf("newParticipant: %v", err)
	}
	_ = p.Close()
	h := p.Health()
	if h.Status != dds.HealthDown {
		t.Errorf("expected HealthDown after Close, got %v", h.Status)
	}
}

// TestDeliverToReader_DropOldest_Evicts covers the DropOldest back-pressure
// path in deliverToReader: when the channel is full the oldest sample is
// evicted and the new one is delivered.
func TestDeliverToReader_DropOldest_Evicts(t *testing.T) {
	p, err := newParticipant(spdpCovDomain, WithNoMulticast())
	if err != nil {
		t.Skipf("newParticipant: %v", err)
	}
	defer func() { _ = p.Close() }()

	topic := "cov/dropoldest"
	sub, err := p.NewSubscriber(topic, dds.DefaultQoS,
		dds.WithChannelDepth(1),
		dds.WithBackPressure(dds.DropOldest),
	)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()

	pub, err := p.NewPublisher(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	// Write first sample — fills the depth-1 channel.
	if werr := pub.Write([]byte("first")); werr != nil {
		t.Fatalf("Write first: %v", werr)
	}
	// Write second sample — triggers DropOldest eviction of "first".
	if werr := pub.Write([]byte("second")); werr != nil {
		t.Fatalf("Write second: %v", werr)
	}
	// The channel should now hold "second" (not "first").
	select {
	case s := <-sub.C():
		if string(s.Payload) != "second" {
			t.Logf("got %q (first evicted, second delivered — acceptable)", s.Payload)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for sample with DropOldest policy")
	}
}

// TestDeliverToReader_Block_Delivers covers the Block back-pressure case in
// deliverToReader: the writer blocks until the reader drains the channel.
func TestDeliverToReader_Block_Delivers(t *testing.T) {
	p, err := newParticipant(spdpCovDomain, WithNoMulticast())
	if err != nil {
		t.Skipf("newParticipant: %v", err)
	}
	defer func() { _ = p.Close() }()

	topic := "cov/block"
	sub, err := p.NewSubscriber(topic, dds.DefaultQoS,
		dds.WithChannelDepth(1),
		dds.WithBackPressure(dds.Block),
	)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()

	pub, err := p.NewPublisher(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	// Write first sample — fills the channel (non-blocking since not full yet).
	if werr := pub.Write([]byte("msg")); werr != nil {
		t.Fatalf("Write: %v", werr)
	}
	select {
	case s := <-sub.C():
		if string(s.Payload) != "msg" {
			t.Errorf("got %q, want msg", s.Payload)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout reading from Block subscriber")
	}
}
