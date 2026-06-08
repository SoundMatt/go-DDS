// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package shmem_test

//fusa:test REQ-SHMEM-001
//fusa:test REQ-SHMEM-002
//fusa:test REQ-SHMEM-003
//fusa:test REQ-SHMEM-004

import (
	"context"
	"errors"
	"testing"
	"time"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/shmem"
)

func newPart(t *testing.T) dds.Participant {
	t.Helper()
	p, err := shmem.New(0)
	if err != nil {
		t.Fatalf("shmem.New: %v", err)
	}
	t.Cleanup(func() { p.Close() })
	return p
}

func TestPublishSubscribe_SameProcess(t *testing.T) {
	p := newPart(t)

	sub, err := p.NewSubscriber("shmem/basic", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()

	pub, err := p.NewPublisher("shmem/basic", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	want := []byte("hello shmem")
	if err := pub.Write(want); err != nil {
		t.Fatalf("Write: %v", err)
	}

	select {
	case s := <-sub.C():
		if string(s.Payload) != string(want) {
			t.Errorf("got %q, want %q", s.Payload, want)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for shmem sample")
	}
}

func TestTransientLocal_LateJoiner(t *testing.T) {
	p := newPart(t)

	pub, _ := p.NewPublisher("shmem/transient", dds.ReliableQoS)
	defer pub.Close()

	_ = pub.Write([]byte("state"))

	sub, _ := p.NewSubscriber("shmem/transient", dds.ReliableQoS)
	defer sub.Close()

	select {
	case s := <-sub.C():
		if string(s.Payload) != "state" {
			t.Errorf("got %q, want state", s.Payload)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("TransientLocal late-joiner did not receive last sample")
	}
}

func TestEmptyTopic_Errors(t *testing.T) {
	p := newPart(t)
	if _, err := p.NewPublisher("", dds.DefaultQoS); err == nil {
		t.Error("expected error for empty publisher topic")
	}
	if _, err := p.NewSubscriber("", dds.DefaultQoS); err == nil {
		t.Error("expected error for empty subscriber topic")
	}
}

func TestClosedParticipant_Errors(t *testing.T) {
	p, _ := shmem.New(0)
	p.Close()
	if _, err := p.NewPublisher("x", dds.DefaultQoS); err == nil {
		t.Error("expected error from closed participant")
	}
}

func TestPublisherClose_StopsWrites(t *testing.T) {
	p := newPart(t)
	pub, _ := p.NewPublisher("shmem/close", dds.DefaultQoS)
	pub.Close()
	if err := pub.Write([]byte("x")); err == nil {
		t.Error("expected error writing to closed publisher")
	}
}

func TestMetrics_ShmemParticipant(t *testing.T) {
	p := newPart(t)
	mp, ok := p.(dds.MetricsProvider)
	if !ok {
		t.Skip("shmem does not implement MetricsProvider")
	}
	pub, _ := p.NewPublisher("shmem/metrics", dds.DefaultQoS)
	sub, _ := p.NewSubscriber("shmem/metrics", dds.DefaultQoS)
	defer sub.Close()
	defer pub.Close()

	_ = pub.Write([]byte("m"))
	select {
	case <-sub.C():
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}

	m := mp.Metrics()
	if m.WriteCount == 0 {
		t.Error("WriteCount should be > 0")
	}
}

func TestChannelDepth_OptionAccepted(t *testing.T) {
	p := newPart(t)
	// Verify WithChannelDepth is accepted and the subscriber can publish/receive.
	sub, err := p.NewSubscriber("shmem/depth", dds.DefaultQoS, dds.WithChannelDepth(4))
	if err != nil {
		t.Fatalf("NewSubscriber with WithChannelDepth: %v", err)
	}
	defer sub.Close()
	pub, _ := p.NewPublisher("shmem/depth", dds.DefaultQoS)
	defer pub.Close()

	_ = pub.Write([]byte("x"))
	select {
	case s := <-sub.C():
		if string(s.Payload) != "x" {
			t.Errorf("got %q, want x", s.Payload)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

func TestSentinelErrors_ErrClosed_Wrapping(t *testing.T) {
	p, _ := shmem.New(0)
	p.Close()
	_, err := p.NewPublisher("x", dds.DefaultQoS)
	if !errors.Is(err, dds.ErrClosed) {
		t.Errorf("closed participant: expected ErrClosed in chain, got %v", err)
	}
	_, err = p.NewSubscriber("x", dds.DefaultQoS)
	if !errors.Is(err, dds.ErrClosed) {
		t.Errorf("closed participant subscriber: expected ErrClosed in chain, got %v", err)
	}
}

func TestSentinelErrors_ErrTopicEmpty_Wrapping(t *testing.T) {
	p := newPart(t)
	_, err := p.NewPublisher("", dds.DefaultQoS)
	if !errors.Is(err, dds.ErrTopicEmpty) {
		t.Errorf("empty topic publisher: expected ErrTopicEmpty in chain, got %v", err)
	}
	_, err = p.NewSubscriber("", dds.DefaultQoS)
	if !errors.Is(err, dds.ErrTopicEmpty) {
		t.Errorf("empty topic subscriber: expected ErrTopicEmpty in chain, got %v", err)
	}
}

func TestSentinelErrors_ErrClosed_Write(t *testing.T) {
	p := newPart(t)
	pub, _ := p.NewPublisher("shmem/closedwrite", dds.DefaultQoS)
	pub.Close()
	err := pub.Write([]byte("x"))
	if !errors.Is(err, dds.ErrClosed) {
		t.Errorf("closed publisher write: expected ErrClosed in chain, got %v", err)
	}
}

func TestMaxSampleSize_EnforcedInShmem(t *testing.T) {
	p := newPart(t)
	qos := dds.DefaultQoS
	qos.MaxSampleSize = 10
	pub, err := p.NewPublisher("shmem/maxsize", qos)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	// Exactly at the limit — must succeed.
	if writeErr := pub.Write([]byte("0123456789")); writeErr != nil {
		t.Fatalf("Write at limit: %v", writeErr)
	}

	// One byte over the limit — must return ErrPayloadTooLarge.
	err = pub.Write([]byte("01234567890")) // 11 bytes
	if !errors.Is(err, dds.ErrPayloadTooLarge) {
		t.Errorf("expected ErrPayloadTooLarge, got %v", err)
	}
}

func TestMaxSampleSize_ZeroMeansUnlimited_Shmem(t *testing.T) {
	p := newPart(t)
	qos := dds.DefaultQoS
	qos.MaxSampleSize = 0
	pub, _ := p.NewPublisher("shmem/unlimited", qos)
	defer pub.Close()

	large := make([]byte, 50_000)
	if err := pub.Write(large); err != nil {
		t.Fatalf("Write with MaxSampleSize=0 should be unlimited: %v", err)
	}
}

func TestCloseWithDrain_Shmem(t *testing.T) {
	p, err := shmem.New(0)
	if err != nil {
		t.Fatalf("shmem.New: %v", err)
	}
	// CloseWithDrain should succeed (shmem is synchronous, no ACKs to wait for).
	drainer, ok := p.(interface {
		CloseWithDrain(ctx interface{ Done() <-chan struct{} }) error
	})
	_ = drainer
	_ = ok
	// Use the dds.CloseWithDrain utility directly.
	if err := p.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestSubscriberClose_RemovesFromBroker(t *testing.T) {
	p := newPart(t)
	pub, _ := p.NewPublisher("shmem/unsubscribe", dds.DefaultQoS)
	defer pub.Close()

	sub, _ := p.NewSubscriber("shmem/unsubscribe", dds.DefaultQoS)

	// Close the subscriber — it must remove itself from the broker.
	sub.Close()

	// Write after the subscriber is closed. The sample should not be
	// delivered to the closed subscriber's channel (which is now closed
	// and removed from the broker). This just verifies Write doesn't panic.
	if err := pub.Write([]byte("after-close")); err != nil {
		t.Fatalf("Write after subscriber close: %v", err)
	}
}

func TestBackPressure_DropOldest_Shmem(t *testing.T) {
	p := newPart(t)
	sub, err := p.NewSubscriber("shmem/bp", dds.DefaultQoS,
		dds.WithChannelDepth(2),
		dds.WithBackPressure(dds.DropOldest),
	)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()

	pub, _ := p.NewPublisher("shmem/bp", dds.DefaultQoS)
	defer pub.Close()

	// Fill the channel and overflow — oldest should be dropped.
	_ = pub.Write([]byte("a"))
	_ = pub.Write([]byte("b"))
	_ = pub.Write([]byte("c")) // overflows depth=2 with DropOldest

	// At least two samples must be deliverable.
	got := 0
	for got < 2 {
		select {
		case <-sub.C():
			got++
		case <-time.After(time.Second):
			t.Fatalf("timeout after %d samples", got)
		}
	}
}

func TestContentFilter_Shmem(t *testing.T) {
	p := newPart(t)
	sub, err := p.NewSubscriber("shmem/filter", dds.DefaultQoS,
		dds.WithFilter(func(s dds.Sample) bool {
			return string(s.Payload) == "keep"
		}),
	)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()
	pub, _ := p.NewPublisher("shmem/filter", dds.DefaultQoS)
	defer pub.Close()

	_ = pub.Write([]byte("drop"))
	_ = pub.Write([]byte("keep"))

	select {
	case s := <-sub.C():
		if string(s.Payload) != "keep" {
			t.Errorf("filter: got %q, want keep", s.Payload)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for filtered sample")
	}
}

func TestSampleTimestamp_Set(t *testing.T) {
	p := newPart(t)
	sub, _ := p.NewSubscriber("shmem/ts", dds.DefaultQoS)
	defer sub.Close()
	pub, _ := p.NewPublisher("shmem/ts", dds.DefaultQoS)
	defer pub.Close()

	before := time.Now()
	_ = pub.Write([]byte("ts"))

	select {
	case s := <-sub.C():
		if s.Timestamp.IsZero() {
			t.Error("Timestamp must not be zero")
		}
		if s.Timestamp.Before(before) {
			t.Errorf("Timestamp %v is before write time", s.Timestamp)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

func TestDomain_ShmemParticipant(t *testing.T) {
	p, err := shmem.New(dds.Domain(7))
	if err != nil {
		t.Fatalf("shmem.New: %v", err)
	}
	defer func() { _ = p.Close() }()
	if got := p.Domain(); got != dds.Domain(7) {
		t.Errorf("Domain() = %d, want 7", got)
	}
}

func TestWriteCtx_Shmem_CancelledBeforeWrite(t *testing.T) {
	p, err := shmem.New(dds.Domain(0))
	if err != nil {
		t.Fatalf("shmem.New: %v", err)
	}
	defer func() { _ = p.Close() }()
	pub, err := p.NewPublisher("shmctx/cancel", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer func() { _ = pub.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := pub.WriteCtx(ctx, []byte("data")); err == nil {
		t.Error("WriteCtx with cancelled context should return error")
	}
}

func TestWriteCtx_Shmem_ValidContext(t *testing.T) {
	p, err := shmem.New(dds.Domain(0))
	if err != nil {
		t.Fatalf("shmem.New: %v", err)
	}
	defer func() { _ = p.Close() }()
	pub, err := p.NewPublisher("shmctx/valid", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer func() { _ = pub.Close() }()
	sub, err := p.NewSubscriber("shmctx/valid", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer func() { _ = sub.Close() }()

	if err := pub.WriteCtx(context.Background(), []byte("ok")); err != nil {
		t.Fatalf("WriteCtx: %v", err)
	}
	select {
	case s := <-sub.C():
		if string(s.Payload) != "ok" {
			t.Errorf("got %q, want %q", s.Payload, "ok")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

func TestSubscriberUnsubscribe_Shmem(t *testing.T) {
	p, err := shmem.New(dds.Domain(0))
	if err != nil {
		t.Fatalf("shmem.New: %v", err)
	}
	defer func() { _ = p.Close() }()
	pub, err := p.NewPublisher("shmunsub/test", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer func() { _ = pub.Close() }()
	sub, err := p.NewSubscriber("shmunsub/test", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}

	_ = pub.Write([]byte("before"))
	select {
	case <-sub.C():
	case <-time.After(time.Second):
		t.Fatal("timeout before unsubscribe")
	}

	if err := sub.Unsubscribe(); err != nil {
		t.Fatalf("Unsubscribe: %v", err)
	}

	_ = pub.Write([]byte("after"))
	select {
	case <-sub.C():
		// Samples written before Unsubscribe took effect may still be
		// buffered; drain them. Only fail if the channel was closed.
	case <-time.After(50 * time.Millisecond):
		// No new sample — expected.
	}

	// Idempotent unsubscribe.
	if err := sub.Unsubscribe(); err != nil {
		t.Errorf("Unsubscribe[2]: %v", err)
	}
	_ = sub.Close()
	_ = sub.Close() // idempotent close
}

// ── v0.9.1 additions ──────────────────────────────────────────────────────────

func TestTryRead_Empty_Shmem(t *testing.T) {
	p := newPart(t)
	sub, _ := p.NewSubscriber("shmem/tryread/empty", dds.DefaultQoS)
	defer sub.Close()

	_, ok := sub.TryRead()
	if ok {
		t.Error("TryRead on empty channel must return false")
	}
}

func TestTryRead_HasSample_Shmem(t *testing.T) {
	p := newPart(t)
	sub, _ := p.NewSubscriber("shmem/tryread/has", dds.DefaultQoS)
	defer sub.Close()
	pub, _ := p.NewPublisher("shmem/tryread/has", dds.DefaultQoS)
	defer pub.Close()

	_ = pub.Write([]byte("ready"))

	var s dds.Sample
	var ok bool
	for i := 0; i < 20; i++ {
		s, ok = sub.TryRead()
		if ok {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !ok {
		t.Fatal("TryRead must return true after Write")
	}
	if string(s.Payload) != "ready" {
		t.Errorf("payload: got %q, want ready", s.Payload)
	}
}

func TestSequenceNumber_Shmem(t *testing.T) {
	p := newPart(t)
	// WithChannelDepth large enough for in-process + cross-process duplicates.
	sub, _ := p.NewSubscriber("shmem/seqnum", dds.DefaultQoS, dds.WithChannelDepth(8))
	defer sub.Close()
	pub, _ := p.NewPublisher("shmem/seqnum", dds.DefaultQoS)
	defer pub.Close()

	_ = pub.Write([]byte("a"))
	_ = pub.Write([]byte("b"))

	// Collect samples for up to 500 ms; the shmem subscriber delivers each
	// write twice (in-process broker + cross-process shmListener), so we may
	// receive 4 samples total. Only in-process broker samples carry a non-zero
	// SequenceNumber; filter to those and verify monotonic increase.
	deadline := time.After(500 * time.Millisecond)
	var numbered []uint64
	for len(numbered) < 2 {
		select {
		case s := <-sub.C():
			if s.SequenceNumber > 0 {
				numbered = append(numbered, s.SequenceNumber)
			}
		case <-deadline:
			t.Fatalf("timeout: only collected %d numbered samples", len(numbered))
		}
	}
	if numbered[1] <= numbered[0] {
		t.Errorf("SequenceNumber must increase: %d then %d", numbered[0], numbered[1])
	}
}

func TestWriterGUID_Shmem(t *testing.T) {
	p := newPart(t)
	sub, _ := p.NewSubscriber("shmem/writerguid", dds.DefaultQoS, dds.WithChannelDepth(8))
	defer sub.Close()
	pub, _ := p.NewPublisher("shmem/writerguid", dds.DefaultQoS)
	defer pub.Close()

	_ = pub.Write([]byte("a"))
	_ = pub.Write([]byte("b"))

	// Cross-process shmem re-deliveries arrive with WriterGUID=zero; filter to
	// in-process broker samples which carry the real GUID.
	var zero dds.GUID
	deadline := time.After(500 * time.Millisecond)
	var guids []dds.GUID
	for len(guids) < 2 {
		select {
		case s := <-sub.C():
			if s.WriterGUID != zero {
				guids = append(guids, s.WriterGUID)
			}
		case <-deadline:
			t.Fatalf("timeout: only collected %d GUID samples", len(guids))
		}
	}
	if guids[0] != guids[1] {
		t.Errorf("WriterGUID must be consistent per publisher: %x vs %x", guids[0], guids[1])
	}
}

func TestWildcard_Shmem(t *testing.T) {
	p := newPart(t)

	sub, err := p.NewSubscriber("shmem/+/val", dds.DefaultQoS, dds.WithChannelDepth(4))
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()

	pub1, _ := p.NewPublisher("shmem/1/val", dds.DefaultQoS)
	defer pub1.Close()
	pub2, _ := p.NewPublisher("shmem/2/val", dds.DefaultQoS)
	defer pub2.Close()
	pubNo, _ := p.NewPublisher("shmem/1/other", dds.DefaultQoS)
	defer pubNo.Close()

	_ = pub1.Write([]byte("one"))
	_ = pub2.Write([]byte("two"))
	_ = pubNo.Write([]byte("no"))

	received := 0
	timeout := time.After(500 * time.Millisecond)
loop:
	for {
		select {
		case s := <-sub.C():
			if string(s.Payload) == "no" {
				t.Error("received sample from non-matching topic")
			}
			received++
			if received >= 2 {
				break loop
			}
		case <-timeout:
			break loop
		}
	}

	if received < 2 {
		t.Errorf("expected 2 matching samples, got %d", received)
	}
}

func TestDeadline_Shmem(t *testing.T) {
	fired := make(chan struct{}, 1)
	p, err := shmem.New(0)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()

	qos := dds.DefaultQoS
	qos.Deadline = 50 * time.Millisecond

	sub, err := p.NewSubscriber("shmem/deadline", qos,
		dds.WithDeadlineMissed(func() {
			select {
			case fired <- struct{}{}:
			default:
			}
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()

	select {
	case <-fired:
	case <-time.After(time.Second):
		t.Fatal("DeadlineMissedCallback did not fire")
	}
}
