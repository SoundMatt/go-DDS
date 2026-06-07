// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package record_test

import (
	"bytes"
	"testing"
	"time"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/record"
)

// ── FaultPublisher ────────────────────────────────────────────────────────────

func TestFaultPublisher_NoFaults_Delivers(t *testing.T) {
	p := newPart(t)
	topic := uniqueTopic("fault/none")
	sub, _ := p.NewSubscriber(topic, dds.DefaultQoS)
	defer sub.Close()
	pub, _ := p.NewPublisher(topic, dds.DefaultQoS)
	defer pub.Close()

	fp := record.NewFaultPublisher(pub, record.FaultOptions{}, 1)
	defer fp.Close()

	if err := fp.Write([]byte("ok")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	select {
	case s := <-sub.C():
		if string(s.Payload) != "ok" {
			t.Errorf("payload: got %q, want ok", s.Payload)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout: sample not delivered with no faults")
	}
}

func TestFaultPublisher_LossRate100_DropsAll(t *testing.T) {
	p := newPart(t)
	topic := uniqueTopic("fault/loss100")
	sub, _ := p.NewSubscriber(topic, dds.DefaultQoS)
	defer sub.Close()
	pub, _ := p.NewPublisher(topic, dds.DefaultQoS)
	defer pub.Close()

	fp := record.NewFaultPublisher(pub, record.FaultOptions{LossRate: 1.0}, 1)
	defer fp.Close()

	for i := 0; i < 5; i++ {
		if err := fp.Write([]byte("drop")); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}
	select {
	case <-sub.C():
		t.Error("expected no samples with 100% loss")
	case <-time.After(60 * time.Millisecond):
		// correct
	}
}

func TestFaultPublisher_DuplicateRate100_Duplicates(t *testing.T) {
	p := newPart(t)
	topic := uniqueTopic("fault/dup100")
	sub, _ := p.NewSubscriber(topic, dds.DefaultQoS, dds.WithChannelDepth(10))
	defer sub.Close()
	pub, _ := p.NewPublisher(topic, dds.DefaultQoS)
	defer pub.Close()

	fp := record.NewFaultPublisher(pub, record.FaultOptions{DuplicateRate: 1.0}, 1)
	defer fp.Close()

	if err := fp.Write([]byte("dup")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	count := 0
	deadline := time.After(500 * time.Millisecond)
	for {
		select {
		case <-sub.C():
			count++
		case <-deadline:
			goto done
		}
	}
done:
	if count < 2 {
		t.Errorf("expected at least 2 deliveries with DuplicateRate=1.0, got %d", count)
	}
}

func TestFaultPublisher_CorruptRate100_FlipsByte(t *testing.T) {
	p := newPart(t)
	topic := uniqueTopic("fault/corrupt100")
	sub, _ := p.NewSubscriber(topic, dds.DefaultQoS)
	defer sub.Close()
	pub, _ := p.NewPublisher(topic, dds.DefaultQoS)
	defer pub.Close()

	original := []byte("hello")
	fp := record.NewFaultPublisher(pub, record.FaultOptions{CorruptRate: 1.0}, 42)
	defer fp.Close()

	if err := fp.Write(original); err != nil {
		t.Fatalf("Write: %v", err)
	}
	select {
	case s := <-sub.C():
		if bytes.Equal(s.Payload, original) {
			t.Error("expected payload to be corrupted, but it matched original")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout: sample not delivered")
	}
}

func TestFaultPublisher_CorruptRate_EmptyPayload_NoCorruption(t *testing.T) {
	p := newPart(t)
	topic := uniqueTopic("fault/corrupt-empty")
	sub, _ := p.NewSubscriber(topic, dds.DefaultQoS)
	defer sub.Close()
	pub, _ := p.NewPublisher(topic, dds.DefaultQoS)
	defer pub.Close()

	fp := record.NewFaultPublisher(pub, record.FaultOptions{CorruptRate: 1.0}, 1)
	defer fp.Close()

	// Empty payload: no bytes to corrupt, must not panic.
	if err := fp.Write([]byte{}); err != nil {
		t.Fatalf("Write: %v", err)
	}
}

func TestFaultPublisher_FixedDelay(t *testing.T) {
	p := newPart(t)
	topic := uniqueTopic("fault/delay")
	sub, _ := p.NewSubscriber(topic, dds.DefaultQoS)
	defer sub.Close()
	pub, _ := p.NewPublisher(topic, dds.DefaultQoS)
	defer pub.Close()

	const delay = 40 * time.Millisecond
	fp := record.NewFaultPublisher(pub, record.FaultOptions{
		DelayMin: delay,
		DelayMax: delay,
	}, 1)
	defer fp.Close()

	start := time.Now()
	if err := fp.Write([]byte("delayed")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed < delay {
		t.Errorf("expected delay >= %v, got %v", delay, elapsed)
	}

	select {
	case <-sub.C():
	case <-time.After(time.Second):
		t.Fatal("timeout: delayed sample not delivered")
	}
}

func TestFaultPublisher_RandomDelay_WithinRange(t *testing.T) {
	p := newPart(t)
	topic := uniqueTopic("fault/delay-range")
	pub, _ := p.NewPublisher(topic, dds.DefaultQoS)
	defer pub.Close()

	const lo, hi = 10 * time.Millisecond, 30 * time.Millisecond
	fp := record.NewFaultPublisher(pub, record.FaultOptions{
		DelayMin: lo,
		DelayMax: hi,
	}, 99)

	start := time.Now()
	_ = fp.Write([]byte("x"))
	elapsed := time.Since(start)
	_ = fp.Close()
	if elapsed < lo {
		t.Errorf("delay %v shorter than DelayMin %v", elapsed, lo)
	}
	if elapsed > hi+20*time.Millisecond { // generous margin for scheduler jitter
		t.Errorf("delay %v much longer than DelayMax %v", elapsed, hi)
	}
}

func TestFaultPublisher_WriteAfterClose_ReturnsError(t *testing.T) {
	p := newPart(t)
	topic := uniqueTopic("fault/closed")
	pub, _ := p.NewPublisher(topic, dds.DefaultQoS)
	fp := record.NewFaultPublisher(pub, record.FaultOptions{}, 1)
	if err := fp.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	err := fp.Write([]byte("x"))
	if err == nil {
		t.Fatal("expected error writing to closed FaultPublisher")
	}
}

func TestFaultPublisher_CloseIdempotent(t *testing.T) {
	p := newPart(t)
	topic := uniqueTopic("fault/close-idem")
	pub, _ := p.NewPublisher(topic, dds.DefaultQoS)
	fp := record.NewFaultPublisher(pub, record.FaultOptions{}, 1)
	_ = fp.Close()
	// second close: underlying pub is already closed, but must not panic
	_ = fp.Close()
}

func TestFaultPublisher_ZeroSeedIsNonDeterministic(t *testing.T) {
	// Seed 0 triggers time.Now() seeding; just verify it constructs without panic.
	p := newPart(t)
	topic := uniqueTopic("fault/seed0")
	pub, _ := p.NewPublisher(topic, dds.DefaultQoS)
	fp := record.NewFaultPublisher(pub, record.FaultOptions{}, 0)
	if err := fp.Write([]byte("ok")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	_ = fp.Close()
}
