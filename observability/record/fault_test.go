// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package record_test

//fusa:test REQ-REC-004
//fusa:test REQ-REC-005
//fusa:test REQ-REC-006
//fusa:test REQ-REC-007
//fusa:test REQ-REC-008
//fusa:test REQ-REC-009

import (
	"bytes"
	"context"
	"testing"
	"time"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/observability/record"
)

// ── FaultPublisher ────────────────────────────────────────────────────────────

func TestFaultPublisher_NoFaults_Delivers(t *testing.T) {
	p := newPart(t)
	topic := uniqueTopic("fault/none")
	sub, err := p.NewSubscriber(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()
	pub, err := p.NewPublisher(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
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
	sub, err := p.NewSubscriber(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()
	pub, err := p.NewPublisher(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
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
	sub, err := p.NewSubscriber(topic, dds.DefaultQoS, dds.WithChannelDepth(10))
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()
	pub, err := p.NewPublisher(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
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
	sub, err := p.NewSubscriber(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()
	pub, err := p.NewPublisher(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
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
	sub, err := p.NewSubscriber(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()
	pub, err := p.NewPublisher(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
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
	sub, err := p.NewSubscriber(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()
	pub, err := p.NewPublisher(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
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
	pub, err := p.NewPublisher(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
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
	pub, err := p.NewPublisher(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	fp := record.NewFaultPublisher(pub, record.FaultOptions{}, 1)
	if closeErr := fp.Close(); closeErr != nil {
		t.Fatalf("Close: %v", closeErr)
	}
	err = fp.Write([]byte("x"))
	if err == nil {
		t.Fatal("expected error writing to closed FaultPublisher")
	}
}

func TestFaultPublisher_CloseIdempotent(t *testing.T) {
	p := newPart(t)
	topic := uniqueTopic("fault/close-idem")
	pub, err := p.NewPublisher(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	fp := record.NewFaultPublisher(pub, record.FaultOptions{}, 1)
	_ = fp.Close()
	// second close: underlying pub is already closed, but must not panic
	_ = fp.Close()
}

// TestFaultPublisher_UnderlyingWriteError covers the `return err` branch inside
// Write when the underlying publisher fails but the FaultPublisher itself is
// still open (f.closed == false).
func TestFaultPublisher_UnderlyingWriteError(t *testing.T) {
	p := newPart(t)
	topic := uniqueTopic("fault/pub-err")
	pub, err := p.NewPublisher(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}

	fp := record.NewFaultPublisher(pub, record.FaultOptions{}, 1)
	// Close the underlying publisher directly — FaultPublisher stays "open".
	_ = pub.Close()

	err = fp.Write([]byte("x"))
	if err == nil {
		t.Fatal("expected error from underlying closed publisher")
	}
	_ = fp.Close()
}

func TestFaultPublisher_ZeroSeedIsNonDeterministic(t *testing.T) {
	// Seed 0 triggers time.Now() seeding; just verify it constructs without panic.
	p := newPart(t)
	topic := uniqueTopic("fault/seed0")
	pub, err := p.NewPublisher(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	fp := record.NewFaultPublisher(pub, record.FaultOptions{}, 0)
	if err := fp.Write([]byte("ok")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	_ = fp.Close()
}

func TestFaultPublisher_ReorderWindow_FlushesOnFull(t *testing.T) {
	p := newPart(t)
	topic := uniqueTopic("fault/reorder-full")
	const window = 4
	sub, err := p.NewSubscriber(topic, dds.DefaultQoS, dds.WithChannelDepth(window*2))
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()
	pub, err := p.NewPublisher(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	fp := record.NewFaultPublisher(pub, record.FaultOptions{ReorderWindow: window}, 42)
	defer fp.Close()

	// Write window-1 samples: none should arrive yet (buffered).
	for i := 0; i < window-1; i++ {
		if err := fp.Write([]byte{byte(i)}); err != nil {
			t.Fatalf("Write(%d): %v", i, err)
		}
	}
	select {
	case <-sub.C():
		t.Fatal("sample arrived before window was full")
	case <-time.After(20 * time.Millisecond):
	}

	// Write the last sample: all window samples should now be emitted.
	if err := fp.Write([]byte{byte(window - 1)}); err != nil {
		t.Fatalf("Write(last): %v", err)
	}
	deadline := time.After(time.Second)
	received := make(map[byte]bool)
	for i := 0; i < window; i++ {
		select {
		case s := <-sub.C():
			if len(s.Payload) != 1 {
				t.Fatalf("unexpected payload %v", s.Payload)
			}
			received[s.Payload[0]] = true
		case <-deadline:
			t.Fatalf("timeout: only received %d/%d samples", i, window)
		}
	}
	for i := 0; i < window; i++ {
		if !received[byte(i)] {
			t.Errorf("sample %d not delivered", i)
		}
	}
}

func TestFaultPublisher_ReorderWindow_FlushOnClose(t *testing.T) {
	p := newPart(t)
	topic := uniqueTopic("fault/reorder-close")
	const window = 4
	sub, err := p.NewSubscriber(topic, dds.DefaultQoS, dds.WithChannelDepth(window))
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()
	pub, err := p.NewPublisher(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}

	fp := record.NewFaultPublisher(pub, record.FaultOptions{ReorderWindow: window}, 7)

	// Write fewer than window samples then close.
	for i := 0; i < window-1; i++ {
		if err := fp.Write([]byte{byte(i)}); err != nil {
			t.Fatalf("Write(%d): %v", i, err)
		}
	}
	if err := fp.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// All buffered samples must arrive after Close.
	deadline := time.After(time.Second)
	received := make(map[byte]bool)
	for i := 0; i < window-1; i++ {
		select {
		case s := <-sub.C():
			if len(s.Payload) == 1 {
				received[s.Payload[0]] = true
			}
		case <-deadline:
			t.Fatalf("timeout: only %d/%d samples after Close", i, window-1)
		}
	}
	for i := 0; i < window-1; i++ {
		if !received[byte(i)] {
			t.Errorf("sample %d not flushed on Close", i)
		}
	}
}

func TestFaultPublisher_WriteCtx_Delivers(t *testing.T) {
	p := newPart(t)
	topic := uniqueTopic("fault/writectx-ok")
	sub, err := p.NewSubscriber(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()
	pub, err := p.NewPublisher(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	fp := record.NewFaultPublisher(pub, record.FaultOptions{}, 1)
	defer fp.Close()

	ctx := context.Background()
	if err := fp.WriteCtx(ctx, []byte("via-ctx")); err != nil {
		t.Fatalf("WriteCtx: %v", err)
	}
	select {
	case s := <-sub.C():
		if string(s.Payload) != "via-ctx" {
			t.Errorf("payload: got %q", s.Payload)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

func TestFaultPublisher_WriteCtx_CancelledDuringDelay(t *testing.T) {
	p := newPart(t)
	topic := uniqueTopic("fault/writectx-cancel")
	pub, err := p.NewPublisher(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	const delay = 200 * time.Millisecond
	fp := record.NewFaultPublisher(pub, record.FaultOptions{
		DelayMin: delay,
		DelayMax: delay,
	}, 1)
	defer fp.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	start := time.Now()
	err = fp.WriteCtx(ctx, []byte("delayed"))
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected context error, got nil")
	}
	if elapsed >= delay {
		t.Errorf("WriteCtx did not honour context cancellation: elapsed %v >= delay %v", elapsed, delay)
	}
}

// TestFaultPublisher_EmitWindow_WriteError covers the return-err branch inside
// emitWindow: fill the reorder window with a closed underlying publisher so
// that emitWindow's first pub.Write call fails and the error propagates.
func TestFaultPublisher_EmitWindow_WriteError(t *testing.T) {
	p := newPart(t)
	topic := uniqueTopic("fault/emitwindow-err")
	pub, err := p.NewPublisher(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}

	const window = 2
	fp := record.NewFaultPublisher(pub, record.FaultOptions{ReorderWindow: window}, 1)
	// Close the underlying publisher so emitWindow's Write returns an error.
	_ = pub.Close()

	// First write: buffered (window not full yet).
	_ = fp.Write([]byte("a"))
	// Second write: window full → emitWindow called → pub.Write fails.
	writeErr := fp.Write([]byte("b"))
	if writeErr == nil {
		t.Fatal("expected error from emitWindow when underlying publisher is closed")
	}
	_ = fp.Close()
}
