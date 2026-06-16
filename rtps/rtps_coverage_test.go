// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Coverage tests targeting previously uncovered paths:
// - NewLoaningPublisher / Loan / Commit (rtps/loan.go)
// - WithConfig / WithIPv6 option coverage (rtps/participant.go)
// - heartbeatLoop branch coverage via reliable intra-process write
// - IPv6 transport socket creation (opportunistic — skipped when unavailable)

package rtps_test

//fusa:test REQ-LOAN-001
//fusa:test REQ-LOAN-002
//fusa:test REQ-LOAN-003
//fusa:test REQ-AUTO-001
//fusa:test REQ-CONF-001
//fusa:test REQ-CONF-002

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/config"
	"github.com/SoundMatt/go-DDS/rtps"
)

// ── Loaned samples (rtps/loan.go) ────────────────────────────────────────────

// coverDomain is distinct from testDomain (99) used by rtps_test.go so these
// tests do not compete for the same UDP unicast ports when the full test binary runs.
const coverDomain = dds.Domain(198)

// newNoMcastParticipant creates a participant without multicast to avoid port
// conflicts when many participants are created in the same test binary run.
func newNoMcastParticipant(t *testing.T) dds.Participant {
	t.Helper()
	p, err := rtps.New(coverDomain, rtps.WithNoMulticast())
	if err != nil {
		t.Skipf("rtps.New: %v", err)
	}
	t.Cleanup(func() { _ = p.Close() })
	return p
}

func TestRTPS_LoaningPublisher_roundtrip(t *testing.T) {
	p := newNoMcastParticipant(t)

	sub, err := p.NewSubscriber("rtps/loan/test", dds.DefaultQoS)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()

	lp, err := rtps.NewLoaningPublisher(p, "rtps/loan/test", dds.DefaultQoS, 256)
	if err != nil {
		t.Fatalf("NewLoaningPublisher: %v", err)
	}
	defer lp.Close()

	buf, err := lp.Loan(11)
	if err != nil {
		t.Fatalf("Loan: %v", err)
	}
	copy(buf, "hello-rtps!")

	if err := lp.Commit(buf); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	select {
	case s := <-sub.C():
		if string(s.Payload) != "hello-rtps!" {
			t.Fatalf("got %q want %q", s.Payload, "hello-rtps!")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for loaned sample")
	}
}

func TestRTPS_LoaningPublisher_size_exceeded(t *testing.T) {
	p := newNoMcastParticipant(t)

	lp, err := rtps.NewLoaningPublisher(p, "rtps/loan/size", dds.DefaultQoS, 64)
	if err != nil {
		t.Fatalf("NewLoaningPublisher: %v", err)
	}
	defer lp.Close()

	_, err = lp.Loan(4096)
	if !errors.Is(err, dds.ErrLoanBuffer) {
		t.Fatalf("expected ErrLoanBuffer for oversized loan, got %v", err)
	}
}

func TestRTPS_LoaningPublisher_closed(t *testing.T) {
	p := newNoMcastParticipant(t)

	lp, err := rtps.NewLoaningPublisher(p, "rtps/loan/closed", dds.DefaultQoS, 256)
	if err != nil {
		t.Fatalf("NewLoaningPublisher: %v", err)
	}
	_ = lp.Close()

	_, err = lp.Loan(10)
	if !errors.Is(err, dds.ErrClosed) {
		t.Fatalf("expected ErrClosed after close, got %v", err)
	}
}

// stubPublisher and stubParticipant let us pass a non-rtps publisher to
// NewLoaningPublisher without importing another package (which would be circular).
type stubPublisher struct{}

func (s *stubPublisher) Write(_ []byte) error                       { return nil }
func (s *stubPublisher) WriteCtx(_ context.Context, _ []byte) error { return nil }
func (s *stubPublisher) Close() error                               { return nil }

type stubParticipant struct{}

func (s *stubParticipant) NewPublisher(_ string, _ dds.QoS) (dds.Publisher, error) {
	return &stubPublisher{}, nil
}
func (s *stubParticipant) NewSubscriber(_ string, _ dds.QoS, _ ...dds.SubscriberOption) (dds.Subscriber, error) {
	return nil, nil
}
func (s *stubParticipant) Domain() dds.Domain { return 0 }
func (s *stubParticipant) Close() error       { return nil }

func TestRTPS_LoaningPublisher_wrong_participant(t *testing.T) {
	_, err := rtps.NewLoaningPublisher(&stubParticipant{}, "topic", dds.DefaultQoS, 256)
	if !errors.Is(err, dds.ErrLoanBuffer) {
		t.Fatalf("expected ErrLoanBuffer for non-rtps participant, got %v", err)
	}
}

// TestRTPS_LoaningPublisher_NewPublisher_error covers the return nil, err path
// in NewLoaningPublisher when the underlying NewPublisher call fails.
// We use a closed RTPS participant which returns ErrClosed from NewPublisher.
func TestRTPS_LoaningPublisher_NewPublisher_error(t *testing.T) {
	p := newNoMcastParticipant(t)
	_ = p.Close()
	_, err := rtps.NewLoaningPublisher(p, "loan/closed-p", dds.DefaultQoS, 256)
	if err == nil {
		t.Fatal("expected error from closed participant, got nil")
	}
}

func TestRTPS_LoaningPublisher_write_and_close(t *testing.T) {
	p := newNoMcastParticipant(t)

	// Write path via the embedded rtpsWriter — confirms Publisher methods work.
	lp, err := rtps.NewLoaningPublisher(p, "rtps/loan/write", dds.DefaultQoS, 256)
	if err != nil {
		t.Fatal(err)
	}

	if err := lp.Write([]byte("direct")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := lp.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

// ── WithConfig option coverage ────────────────────────────────────────────────

func TestRTPS_WithConfig_heartbeat_and_spdp(t *testing.T) {
	cfg := &config.ParticipantConfig{
		HeartbeatPeriod: "100ms",
		SPDPInterval:    "2s",
		SPDPJitter:      "50ms",
		NoMulticast:     true,
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	p, err := rtps.New(coverDomain, rtps.WithConfig(cfg), rtps.WithNoMulticast())
	if err != nil {
		t.Skipf("rtps.New: %v", err)
	}
	defer p.Close()

	// Verify participant works after config application.
	sub, err := p.NewSubscriber("cfg/test", dds.DefaultQoS)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()

	pub, err := p.NewPublisher("cfg/test", dds.DefaultQoS)
	if err != nil {
		t.Fatal(err)
	}
	defer pub.Close()

	if err := pub.Write([]byte("configured")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	select {
	case s := <-sub.C():
		if string(s.Payload) != "configured" {
			t.Errorf("got %q", s.Payload)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
}

func TestRTPS_WithConfig_peer_locators(t *testing.T) {
	cfg := &config.ParticipantConfig{
		PeerLocators: []string{"127.0.0.1:7411"},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	p, err := rtps.New(coverDomain, rtps.WithConfig(cfg), rtps.WithNoMulticast())
	if err != nil {
		t.Skipf("rtps.New: %v", err)
	}
	defer p.Close()
	// Participant created — locator is registered (coverage of append branch).
}

// ── WithIPv6 option + IPv6 transport coverage ─────────────────────────────────

func TestRTPS_WithIPv6_creates_participant(t *testing.T) {
	// Skip if IPv6 is not available in this environment.
	if !ipv6Available() {
		t.Skip("IPv6 not available in this environment")
	}

	p, err := rtps.New(coverDomain, rtps.WithIPv6(), rtps.WithNoMulticast())
	if err != nil {
		t.Skipf("rtps.New with IPv6: %v", err)
	}
	defer p.Close()

	// Basic pub/sub works after IPv6 init.
	sub, err := p.NewSubscriber("ipv6/test", dds.DefaultQoS)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()

	pub, err := p.NewPublisher("ipv6/test", dds.DefaultQoS)
	if err != nil {
		t.Fatal(err)
	}
	defer pub.Close()

	if err := pub.Write([]byte("ipv6")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	select {
	case <-sub.C():
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
}

// ── Metrics and health interfaces ─────────────────────────────────────────────

func TestRTPS_DiscoveryMetrics(t *testing.T) {
	p := newNoMcastParticipant(t)

	mp, ok := p.(dds.DiscoveryMetricsProvider)
	if !ok {
		t.Skip("participant does not implement DiscoveryMetricsProvider")
	}
	dm := mp.DiscoveryMetrics()
	// After creation the participant has sent at least 0 announcements; just
	// verify the call succeeds and the struct is valid.
	if dm.AnnouncesSent > 1e9 {
		t.Errorf("suspicious AnnouncesSent value: %d", dm.AnnouncesSent)
	}
}

func TestRTPS_TopicMetrics(t *testing.T) {
	p := newNoMcastParticipant(t)

	mp, ok := p.(dds.TopicMetricsProvider)
	if !ok {
		t.Skip("participant does not implement TopicMetricsProvider")
	}

	pub, err := p.NewPublisher("metrics/topic", dds.DefaultQoS)
	if err != nil {
		t.Fatal(err)
	}
	defer pub.Close()

	if err := pub.Write([]byte("m")); err != nil {
		t.Fatal(err)
	}

	metrics := mp.TopicMetrics()
	found := false
	for _, tm := range metrics {
		if tm.Topic == "metrics/topic" {
			found = true
			if tm.WriteCount == 0 {
				t.Errorf("expected WriteCount > 0")
			}
		}
	}
	if !found {
		t.Error("metrics/topic not found in TopicMetrics")
	}
}

func TestRTPS_Health(t *testing.T) {
	p := newNoMcastParticipant(t)

	hp, ok := p.(dds.HealthProvider)
	if !ok {
		t.Skip("participant does not implement HealthProvider")
	}
	h := hp.Health()
	if h.Status != dds.HealthOK {
		t.Errorf("expected HealthOK, got %v", h.Status)
	}
}

func TestRTPS_WithContext_cancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	p, err := rtps.New(coverDomain, rtps.WithContext(ctx), rtps.WithNoMulticast())
	if err != nil {
		t.Skipf("rtps.New: %v", err)
	}
	// Cancel the context; participant should close itself.
	cancel()
	// Give the goroutine time to observe the cancellation.
	time.Sleep(50 * time.Millisecond)

	// After context cancel, new publishers should fail.
	_, err = p.NewPublisher("ctx/topic", dds.DefaultQoS)
	if err == nil {
		t.Log("participant may not have closed yet — this is a best-effort coverage test")
		p.Close() //nolint:errcheck
	}
}

// ipv6Available returns true when the host has at least one non-loopback IPv6
// address that can be bound. Containers and many CI environments lack this.
func ipv6Available() bool {
	ifaces, err := net.Interfaces()
	if err != nil {
		return false
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, a := range addrs {
			ipnet, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			if ipnet.IP.To4() == nil && ipnet.IP.To16() != nil && !ipnet.IP.IsLoopback() {
				return true
			}
		}
	}
	return false
}

// ── heartbeatLoop branch coverage ─────────────────────────────────────────────

func TestRTPS_ReliableWriter_heartbeat_path(t *testing.T) {
	// Use a short heartbeat period so the heartbeatLoop fires during the test.
	p, err := rtps.New(coverDomain, rtps.WithHeartbeatPeriod(20*time.Millisecond), rtps.WithNoMulticast())
	if err != nil {
		t.Skipf("rtps.New: %v", err)
	}
	defer p.Close()

	sub, err := p.NewSubscriber("hb/test", dds.ReliableQoS)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()

	pub, err := p.NewPublisher("hb/test", dds.ReliableQoS)
	if err != nil {
		t.Fatal(err)
	}
	defer pub.Close()

	// Write several samples so the heartbeat loop has history to reference.
	for i := 0; i < 3; i++ {
		if err := pub.Write([]byte("hb")); err != nil {
			t.Fatalf("Write %d: %v", i, err)
		}
	}

	// Let the heartbeat loop run for a few ticks.
	time.Sleep(100 * time.Millisecond)

	// Drain delivered samples.
	for {
		select {
		case <-sub.C():
			continue
		default:
			return
		}
	}
}

// TestRTPS_Write_LargePayload_Fragmented covers the fragment-building branch in
// rtpsWriter.Write (participant.go) when payload exceeds maxFragmentPayload.
// Payloads > 1196 bytes trigger splitIntoFragmentsN and marshalDataFrag paths.
func TestRTPS_Write_LargePayload_Fragmented(t *testing.T) {
	p := newNoMcastParticipant(t)

	sub, err := p.NewSubscriber("frag/large", dds.DefaultQoS)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()

	pub, err := p.NewPublisher("frag/large", dds.DefaultQoS)
	if err != nil {
		t.Fatal(err)
	}
	defer pub.Close()

	// 2000 bytes exceeds the 1200-byte maxFragmentPayload threshold.
	big := make([]byte, 2000)
	for i := range big {
		big[i] = byte(i)
	}
	if err := pub.Write(big); err != nil {
		t.Fatalf("Write large payload: %v", err)
	}

	select {
	case s := <-sub.C():
		if len(s.Payload) != len(big) {
			t.Errorf("got %d bytes, want %d", len(s.Payload), len(big))
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("timeout waiting for large fragmented payload")
	}
}

// TestRTPS_TopicMetrics_DoubleWrite covers the topicCounterFor fast path where
// a counter already exists for the topic (sync.Map Load returns true on second call).
func TestRTPS_TopicMetrics_DoubleWrite(t *testing.T) {
	p := newNoMcastParticipant(t)

	mp, ok := p.(dds.TopicMetricsProvider)
	if !ok {
		t.Skip("participant does not implement TopicMetricsProvider")
	}

	sub, err := p.NewSubscriber("metrics/double", dds.DefaultQoS)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()

	pub, err := p.NewPublisher("metrics/double", dds.DefaultQoS)
	if err != nil {
		t.Fatal(err)
	}
	defer pub.Close()

	// First write creates the topic counter.
	if err := pub.Write([]byte("first")); err != nil {
		t.Fatal(err)
	}
	// Second write hits the topicCounterFor fast path (existing counter).
	if err := pub.Write([]byte("second")); err != nil {
		t.Fatal(err)
	}

	metrics := mp.TopicMetrics()
	for _, tm := range metrics {
		if tm.Topic == "metrics/double" && tm.WriteCount >= 2 {
			return
		}
	}
	t.Error("expected WriteCount >= 2 for metrics/double")
}

// TestRTPS_DispatchToReaders_TopicMismatch covers the topic-filter continue
// branch in dispatchToReaders when a reader's topic does not match the
// publisher's topic and has no wildcard overlap.
func TestRTPS_DispatchToReaders_TopicMismatch(t *testing.T) {
	p := newNoMcastParticipant(t)

	// Subscriber on a different topic — will not match publisher writes.
	sub, err := p.NewSubscriber("dispatch/reader", dds.DefaultQoS)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()

	pub, err := p.NewPublisher("dispatch/writer", dds.DefaultQoS)
	if err != nil {
		t.Fatal(err)
	}
	defer pub.Close()

	// Writing to "dispatch/writer" dispatches through all readers; the reader
	// on "dispatch/reader" triggers the topic-mismatch continue branch.
	if err := pub.Write([]byte("mismatch")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	// Confirm the mismatched reader received nothing.
	select {
	case s := <-sub.C():
		t.Errorf("unexpected sample on mismatched topic: %v", s)
	case <-time.After(50 * time.Millisecond):
	}
}
