// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Tests for Milestone 14 "QoS Enforcement — Active Policy": Liveliness,
// Ownership, Partition, and the Time-Based Filter.

package rtps

import (
	"sync"
	"testing"
	"time"

	dds "github.com/SoundMatt/go-DDS"
)

func drainOne(t *testing.T, ch <-chan dds.Sample, timeout time.Duration) (dds.Sample, bool) {
	t.Helper()
	select {
	case s, ok := <-ch:
		return s, ok
	case <-time.After(timeout):
		return dds.Sample{}, false
	}
}

func expectNone(t *testing.T, ch <-chan dds.Sample, wait time.Duration) {
	t.Helper()
	select {
	case s, ok := <-ch:
		if ok {
			t.Fatalf("expected no delivery, got sample %q", s.Payload)
		}
	case <-time.After(wait):
	}
}

// ── Partition ─────────────────────────────────────────────────────────────────

func TestPartition_MatchIntersects(t *testing.T) {
	if got := partitionsMatch([]string{"A", "B"}, []string{"B", "C"}); !got {
		t.Fatal("expected intersecting partitions to match")
	}
}

func TestPartition_DefaultMatchesDefault(t *testing.T) {
	if !partitionsMatch(nil, nil) {
		t.Fatal("expected two default (empty) partitions to match")
	}
}

func TestPartition_DefaultDoesNotMatchNamed(t *testing.T) {
	if partitionsMatch(nil, []string{"A"}) {
		t.Fatal("expected default partition not to match a named partition")
	}
}

// TestPartition_SameProcessGating exercises the same-process bypass path in
// dispatchToReaders: two local subscribers on the same topic in different
// partitions must each see only the writer that shares their partition.
func TestPartition_SameProcessGating(t *testing.T) {
	p := testPart(t)

	subA, err := p.NewSubscriber("part/topic", dds.QoS{Partition: []string{"A"}})
	if err != nil {
		t.Fatalf("NewSubscriber A: %v", err)
	}
	defer subA.Close()
	subB, err := p.NewSubscriber("part/topic", dds.QoS{Partition: []string{"B"}})
	if err != nil {
		t.Fatalf("NewSubscriber B: %v", err)
	}
	defer subB.Close()

	pubA, err := p.NewPublisher("part/topic", dds.QoS{Partition: []string{"A"}})
	if err != nil {
		t.Fatalf("NewPublisher A: %v", err)
	}
	defer pubA.Close()

	if err := pubA.Write([]byte("hello")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	if s, ok := drainOne(t, subA.C(), time.Second); !ok || string(s.Payload) != "hello" {
		t.Fatalf("subA: expected delivery, got %v ok=%v", s, ok)
	}
	expectNone(t, subB.C(), 200*time.Millisecond)
}

// ── Ownership ─────────────────────────────────────────────────────────────────

func TestOwnership_HighestStrengthWins(t *testing.T) {
	if !guidLess(GUID{}, GUID{Entity: EntityId{0, 0, 0, 1}}) {
		t.Fatal("guidLess sanity check failed")
	}

	p := testPart(t)

	sub, err := p.NewSubscriber("own/topic", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()

	low, err := p.NewPublisher("own/topic", dds.QoS{Ownership: dds.ExclusiveOwnership, OwnershipStrength: 1})
	if err != nil {
		t.Fatalf("NewPublisher low: %v", err)
	}
	defer low.Close()
	high, err := p.NewPublisher("own/topic", dds.QoS{Ownership: dds.ExclusiveOwnership, OwnershipStrength: 10})
	if err != nil {
		t.Fatalf("NewPublisher high: %v", err)
	}

	if err := low.Write([]byte("from-low")); err != nil {
		t.Fatalf("Write low: %v", err)
	}
	expectNone(t, sub.C(), 200*time.Millisecond)

	if err := high.Write([]byte("from-high")); err != nil {
		t.Fatalf("Write high: %v", err)
	}
	if s, ok := drainOne(t, sub.C(), time.Second); !ok || string(s.Payload) != "from-high" {
		t.Fatalf("expected delivery from high-strength writer, got %v ok=%v", s, ok)
	}

	// Failover: closing the active (high-strength) writer should promote low.
	if err := high.Close(); err != nil {
		t.Fatalf("Close high: %v", err)
	}
	if err := low.Write([]byte("failover")); err != nil {
		t.Fatalf("Write low after failover: %v", err)
	}
	if s, ok := drainOne(t, sub.C(), time.Second); !ok || string(s.Payload) != "failover" {
		t.Fatalf("expected delivery from low-strength writer after failover, got %v ok=%v", s, ok)
	}
}

func TestOwnership_SharedIsUnaffected(t *testing.T) {
	p := testPart(t)
	sub, err := p.NewSubscriber("own/shared", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()

	pubA, err := p.NewPublisher("own/shared", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher A: %v", err)
	}
	defer pubA.Close()
	pubB, err := p.NewPublisher("own/shared", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher B: %v", err)
	}
	defer pubB.Close()

	if err := pubA.Write([]byte("a")); err != nil {
		t.Fatalf("Write A: %v", err)
	}
	if err := pubB.Write([]byte("b")); err != nil {
		t.Fatalf("Write B: %v", err)
	}
	got := map[string]bool{}
	for i := 0; i < 2; i++ {
		s, ok := drainOne(t, sub.C(), time.Second)
		if !ok {
			t.Fatalf("expected 2 deliveries under SharedOwnership, got %d", i)
		}
		got[string(s.Payload)] = true
	}
	if !got["a"] || !got["b"] {
		t.Fatalf("expected both writers' samples delivered, got %v", got)
	}
}

// ── Liveliness ────────────────────────────────────────────────────────────────

func TestLiveliness_AutomaticAssertionKeepsWriterAlive(t *testing.T) {
	p := testPart(t)

	var mu sync.Mutex
	var lost []dds.GUID
	sub, err := p.NewSubscriber("live/auto", dds.DefaultQoS, dds.WithLivelinessLost(func(g dds.GUID) {
		mu.Lock()
		lost = append(lost, g)
		mu.Unlock()
	}))
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()

	pub, err := p.NewPublisher("live/auto", dds.QoS{
		Liveliness:              dds.AutomaticLiveliness,
		LivelinessLeaseDuration: 40 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	// Give the writer's automatic assertion ticker several periods to prove
	// it — with no Write calls at all — the reader must never declare it lost.
	time.Sleep(250 * time.Millisecond)

	mu.Lock()
	n := len(lost)
	mu.Unlock()
	if n != 0 {
		t.Fatalf("expected 0 LivelinessLost callbacks for an actively-asserting writer, got %d", n)
	}
}

func TestLiveliness_ManualByTopicFiresWhenSilent(t *testing.T) {
	p := testPart(t)

	var mu sync.Mutex
	var lost []dds.GUID
	sub, err := p.NewSubscriber("live/manual", dds.DefaultQoS, dds.WithLivelinessLost(func(g dds.GUID) {
		mu.Lock()
		lost = append(lost, g)
		mu.Unlock()
	}))
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()

	pub, err := p.NewPublisher("live/manual", dds.QoS{
		Liveliness:              dds.ManualByTopicLiveliness,
		LivelinessLeaseDuration: 40 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	// ManualByTopicLiveliness sends no automatic assertions, so silence past
	// the lease must be declared lost.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(lost)
		mu.Unlock()
		if n > 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("expected LivelinessLost to fire for a silent ManualByTopicLiveliness writer")
}

// ── Time-Based Filter ─────────────────────────────────────────────────────────

func TestTimeBasedFilter_DropsFastSamples(t *testing.T) {
	p := testPart(t)

	sub, err := p.NewSubscriber("tbf/topic", dds.QoS{MinSeparation: 200 * time.Millisecond})
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()

	pub, err := p.NewPublisher("tbf/topic", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	if err := pub.Write([]byte("first")); err != nil {
		t.Fatalf("Write first: %v", err)
	}
	if s, ok := drainOne(t, sub.C(), time.Second); !ok || string(s.Payload) != "first" {
		t.Fatalf("expected first sample delivered, got %v ok=%v", s, ok)
	}

	// Second sample arrives well within MinSeparation: must be dropped.
	if err := pub.Write([]byte("second")); err != nil {
		t.Fatalf("Write second: %v", err)
	}
	expectNone(t, sub.C(), 100*time.Millisecond)

	// Third sample arrives after MinSeparation has elapsed: must be delivered.
	time.Sleep(200 * time.Millisecond)
	if err := pub.Write([]byte("third")); err != nil {
		t.Fatalf("Write third: %v", err)
	}
	if s, ok := drainOne(t, sub.C(), time.Second); !ok || string(s.Payload) != "third" {
		t.Fatalf("expected third sample delivered after MinSeparation elapsed, got %v ok=%v", s, ok)
	}
}

func TestTimeBasedFilter_Disabled(t *testing.T) {
	p := testPart(t)
	sub, err := p.NewSubscriber("tbf/off", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()
	pub, err := p.NewPublisher("tbf/off", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	for i := 0; i < 3; i++ {
		if err := pub.Write([]byte("x")); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}
	for i := 0; i < 3; i++ {
		if _, ok := drainOne(t, sub.C(), time.Second); !ok {
			t.Fatalf("expected all 3 samples delivered with MinSeparation disabled, got %d", i)
		}
	}
}
