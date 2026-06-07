// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package mock_test

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/mock"
)

func newParticipant(t *testing.T) dds.Participant {
	t.Helper()
	p, err := mock.New(0)
	if err != nil {
		t.Fatalf("mock.New: %v", err)
	}
	t.Cleanup(func() { p.Close() })
	return p
}

func TestPublishSubscribe_SameTopic(t *testing.T) {
	p := newParticipant(t)

	sub, err := p.NewSubscriber("test/topic", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()

	pub, err := p.NewPublisher("test/topic", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	want := []byte(`{"msg":"hello"}`)
	if err := pub.Write(want); err != nil {
		t.Fatalf("Write: %v", err)
	}

	select {
	case got := <-sub.C():
		if string(got.Payload) != string(want) {
			t.Errorf("payload: got %q, want %q", got.Payload, want)
		}
		if got.Topic != "test/topic" {
			t.Errorf("topic: got %q, want %q", got.Topic, "test/topic")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for sample")
	}
}

func TestPublishSubscribe_DifferentTopic(t *testing.T) {
	p := newParticipant(t)

	sub, _ := p.NewSubscriber("topic/A", dds.DefaultQoS)
	defer sub.Close()

	pub, _ := p.NewPublisher("topic/B", dds.DefaultQoS)
	defer pub.Close()

	_ = pub.Write([]byte("should not arrive"))

	select {
	case s := <-sub.C():
		t.Errorf("unexpected sample on wrong topic: %v", s)
	case <-time.After(50 * time.Millisecond):
		// correct: nothing delivered
	}
}

func TestPublishSubscribe_MultipleSubscribers(t *testing.T) {
	p := newParticipant(t)

	const topic = "fan/out"
	sub1, _ := p.NewSubscriber(topic, dds.DefaultQoS)
	sub2, _ := p.NewSubscriber(topic, dds.DefaultQoS)
	defer sub1.Close()
	defer sub2.Close()

	pub, _ := p.NewPublisher(topic, dds.DefaultQoS)
	defer pub.Close()

	_ = pub.Write([]byte("broadcast"))

	recv := func(sub dds.Subscriber, name string) {
		select {
		case s := <-sub.C():
			if string(s.Payload) != "broadcast" {
				t.Errorf("%s: got %q", name, s.Payload)
			}
		case <-time.After(time.Second):
			t.Errorf("%s: timed out", name)
		}
	}
	recv(sub1, "sub1")
	recv(sub2, "sub2")
}

func TestPayloadIsolation(t *testing.T) {
	p := newParticipant(t)
	sub, _ := p.NewSubscriber("iso", dds.DefaultQoS)
	defer sub.Close()
	pub, _ := p.NewPublisher("iso", dds.DefaultQoS)
	defer pub.Close()

	orig := []byte("mutable")
	_ = pub.Write(orig)
	orig[0] = 'X' // mutate after Write

	select {
	case s := <-sub.C():
		if s.Payload[0] == 'X' {
			t.Error("publisher mutation leaked into delivered sample")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

func TestSubscriberClose_ClosesChannel(t *testing.T) {
	p := newParticipant(t)
	sub, _ := p.NewSubscriber("close/me", dds.DefaultQoS)
	sub.Close()
	select {
	case _, ok := <-sub.C():
		if ok {
			t.Error("channel should be closed after subscriber.Close()")
		}
	case <-time.After(time.Second):
		t.Fatal("channel was not closed")
	}
}

func TestSubscriberClose_Idempotent(t *testing.T) {
	p := newParticipant(t)
	sub, _ := p.NewSubscriber("close/twice", dds.DefaultQoS)
	sub.Close()
	sub.Close() // must not panic
}

func TestPublisherWrite_AfterClose(t *testing.T) {
	p := newParticipant(t)
	pub, _ := p.NewPublisher("closed/pub", dds.DefaultQoS)
	pub.Close()
	if err := pub.Write([]byte("x")); err == nil {
		t.Error("expected error writing to closed publisher")
	}
}

func TestParticipantClose_BlocksNewEndpoints(t *testing.T) {
	p, _ := mock.New(0)
	p.Close()
	if _, err := p.NewPublisher("t", dds.DefaultQoS); err == nil {
		t.Error("expected error from closed participant NewPublisher")
	}
	if _, err := p.NewSubscriber("t", dds.DefaultQoS); err == nil {
		t.Error("expected error from closed participant NewSubscriber")
	}
}

// ── WaitSet ───────────────────────────────────────────────────────────────────

func TestWaitSet_DeliversSample(t *testing.T) {
	p := newParticipant(t)

	subA, _ := p.NewSubscriber("waitset/a", dds.DefaultQoS)
	subB, _ := p.NewSubscriber("waitset/b", dds.DefaultQoS)
	defer subA.Close()
	defer subB.Close()

	pubB, _ := p.NewPublisher("waitset/b", dds.DefaultQoS)
	defer pubB.Close()

	ws := dds.NewWaitSet(subA, subB)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := pubB.Write([]byte("ws-hello")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	sample, got, err := ws.Wait(ctx)
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if got != subB {
		t.Error("expected sample from subB")
	}
	if string(sample.Payload) != "ws-hello" {
		t.Errorf("payload: got %q", sample.Payload)
	}
}

func TestWaitSet_Timeout(t *testing.T) {
	p := newParticipant(t)
	sub, _ := p.NewSubscriber("waitset/timeout", dds.DefaultQoS)
	defer sub.Close()

	ws := dds.NewWaitSet(sub)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	_, _, err := ws.Wait(ctx)
	if err == nil {
		t.Error("expected context error")
	}
}

func TestWaitSet_MultipleWaits(t *testing.T) {
	p := newParticipant(t)
	sub, _ := p.NewSubscriber("waitset/multi", dds.DefaultQoS)
	pub, _ := p.NewPublisher("waitset/multi", dds.DefaultQoS)
	defer sub.Close()
	defer pub.Close()

	ws := dds.NewWaitSet(sub)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		_ = pub.Write([]byte{byte(i)})
		s, _, err := ws.Wait(ctx)
		if err != nil {
			t.Fatalf("Wait %d: %v", i, err)
		}
		if s.Payload[0] != byte(i) {
			t.Errorf("Wait %d: got %d", i, s.Payload[0])
		}
	}
}

// ── TransientLocal durability ─────────────────────────────────────────────────

func TestTransientLocal_LateJoiner(t *testing.T) {
	p := newParticipant(t)

	pub, err := p.NewPublisher("transient/state", dds.ReliableQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	want := []byte(`{"value":42}`)
	if werr := pub.Write(want); werr != nil {
		t.Fatalf("Write: %v", werr)
	}

	// Subscribe after publish; should receive the last sample immediately.
	sub, err := p.NewSubscriber("transient/state", dds.ReliableQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()

	select {
	case s := <-sub.C():
		if string(s.Payload) != string(want) {
			t.Errorf("TransientLocal: got %q, want %q", s.Payload, want)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout: TransientLocal late-joiner did not receive last sample")
	}
}

func TestTransientLocal_NoSample(t *testing.T) {
	// Subscribe before any publish; no phantom sample should be delivered.
	p := newParticipant(t)
	sub, err := p.NewSubscriber("transient/empty", dds.ReliableQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()

	select {
	case s := <-sub.C():
		t.Errorf("unexpected phantom sample: %q", s.Payload)
	case <-time.After(30 * time.Millisecond):
		// correct
	}
}

func TestTransientLocal_VolatileNotDelivered(t *testing.T) {
	// Volatile QoS must not deliver last sample to late joiners.
	p := newParticipant(t)

	pub, _ := p.NewPublisher("transient/volatile", dds.DefaultQoS)
	defer pub.Close()
	_ = pub.Write([]byte("volatile-value"))

	sub, _ := p.NewSubscriber("transient/volatile", dds.DefaultQoS)
	defer sub.Close()

	select {
	case s := <-sub.C():
		t.Errorf("Volatile subscriber should not receive historical sample: %q", s.Payload)
	case <-time.After(30 * time.Millisecond):
		// correct
	}
}

// ── WaitSet — all-closed path ─────────────────────────────────────────────────

func TestWaitSet_AllChannelsClosed(t *testing.T) {
	p := newParticipant(t)
	sub, _ := p.NewSubscriber("ws/all-closed", dds.DefaultQoS)
	sub.Close() // channel is now closed before we start waiting

	ws := dds.NewWaitSet(sub)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _, _ = ws.Wait(ctx)
	}()

	select {
	case <-done:
		// correct: returned promptly, did not block until context deadline
	case <-time.After(500 * time.Millisecond):
		t.Fatal("WaitSet.Wait should return promptly when all channels are closed")
	}
}

func TestMultipleDomains_ShareBroker(t *testing.T) {
	// Mock ignores domain; all participants share the global broker.
	p1, _ := mock.New(0)
	p2, _ := mock.New(1)
	defer p1.Close()
	defer p2.Close()

	sub, _ := p1.NewSubscriber("cross/domain", dds.DefaultQoS)
	defer sub.Close()
	pub, _ := p2.NewPublisher("cross/domain", dds.DefaultQoS)
	defer pub.Close()

	_ = pub.Write([]byte("cross"))
	select {
	case <-sub.C():
	case <-time.After(time.Second):
		t.Fatal("cross-domain delivery failed in mock")
	}
}

func TestContentFilter_DeliverMatchingOnly(t *testing.T) {
	p := newParticipant(t)

	sub, err := p.NewSubscriber("filter/mock", dds.DefaultQoS,
		dds.WithFilter(func(s dds.Sample) bool { return string(s.Payload) == "pass" }),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	pub, _ := p.NewPublisher("filter/mock", dds.DefaultQoS)
	defer pub.Close()

	_ = pub.Write([]byte("drop"))
	_ = pub.Write([]byte("pass"))

	select {
	case s := <-sub.C():
		if string(s.Payload) != "pass" {
			t.Errorf("got %q, want pass", s.Payload)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout: filtered sample not delivered")
	}
	select {
	case s := <-sub.C():
		t.Errorf("unexpected sample: %q", s.Payload)
	default:
	}
}

func TestWildcard_SingleLevel(t *testing.T) {
	p := newParticipant(t)

	sub, err := p.NewSubscriber("wild/+/data", dds.DefaultQoS)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	pub, _ := p.NewPublisher("wild/sensor1/data", dds.DefaultQoS)
	defer pub.Close()

	_ = pub.Write([]byte("42"))
	select {
	case s := <-sub.C():
		if string(s.Payload) != "42" {
			t.Errorf("got %q, want 42", s.Payload)
		}
	case <-time.After(time.Second):
		t.Fatal("wildcard sample not delivered")
	}
}

func TestWildcard_MultiLevel(t *testing.T) {
	p := newParticipant(t)

	sub, err := p.NewSubscriber("sensors/#", dds.DefaultQoS)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	pub, _ := p.NewPublisher("sensors/temp/room1", dds.DefaultQoS)
	defer pub.Close()

	_ = pub.Write([]byte("23"))
	select {
	case <-sub.C():
	case <-time.After(time.Second):
		t.Fatal("# wildcard sample not delivered")
	}
}

// TestWildcard_PatternLongerThanTopic covers the `len(tSegs) == 0` branch in
// matchSlices, which fires when the pattern has more segments than the topic.
func TestWildcard_PatternLongerThanTopic(t *testing.T) {
	p := newParticipant(t)
	// Subscriber pattern has 3 levels; publisher has 2.
	// matchSlices(["a","b","c"],["a","b"]) → recurses to (["c"],[]) → len(tSegs)==0 → false
	sub, err := p.NewSubscriber("a/b/c", dds.DefaultQoS)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()

	pub, err := p.NewPublisher("a/b", dds.DefaultQoS)
	if err != nil {
		t.Fatal(err)
	}
	defer pub.Close()

	_ = pub.Write([]byte("data"))
	// Shorter topic must NOT match longer pattern.
	select {
	case <-sub.C():
		t.Error("shorter topic 'a/b' must not match longer pattern 'a/b/c'")
	case <-time.After(50 * time.Millisecond):
		// correct: no delivery
	}
}

func TestSentinelErrors_EmptyTopic(t *testing.T) {
	p := newParticipant(t)
	if _, err := p.NewPublisher("", dds.DefaultQoS); err == nil {
		t.Error("expected error for empty publisher topic")
	}
	if _, err := p.NewSubscriber("", dds.DefaultQoS); err == nil {
		t.Error("expected error for empty subscriber topic")
	}
}

func TestSentinelErrors_ClosedParticipant(t *testing.T) {
	p, _ := mock.New(0)
	p.Close()
	if _, err := p.NewPublisher("x", dds.DefaultQoS); err == nil {
		t.Error("expected error from closed participant publisher")
	}
	if _, err := p.NewSubscriber("x", dds.DefaultQoS); err == nil {
		t.Error("expected error from closed participant subscriber")
	}
}

func TestDeadlineCallback_Fires(t *testing.T) {
	fired := make(chan string, 1)
	p, err := mock.New(0, mock.WithDeadlineCallback(func(topic string) {
		select {
		case fired <- topic:
		default:
		}
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()

	qos := dds.DefaultQoS
	qos.Deadline = 50 * time.Millisecond
	pub, err := p.NewPublisher("mock/deadline", qos)
	if err != nil {
		t.Fatal(err)
	}
	defer pub.Close()

	select {
	case topic := <-fired:
		if topic != "mock/deadline" {
			t.Errorf("expected mock/deadline, got %q", topic)
		}
	case <-time.After(time.Second):
		t.Fatal("deadline callback did not fire")
	}
}

func TestDeadlineCallback_ResetOnWrite(t *testing.T) {
	fired := make(chan struct{}, 1)
	p, err := mock.New(0, mock.WithDeadlineCallback(func(_ string) {
		select {
		case fired <- struct{}{}:
		default:
		}
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()

	qos := dds.DefaultQoS
	qos.Deadline = 100 * time.Millisecond
	pub, err := p.NewPublisher("mock/nodeadline", qos)
	if err != nil {
		t.Fatal(err)
	}
	defer pub.Close()

	// Keep writing to prevent deadline.
	_ = pub.Write([]byte("a"))
	time.Sleep(60 * time.Millisecond)
	_ = pub.Write([]byte("b"))
	time.Sleep(60 * time.Millisecond)
	_ = pub.Write([]byte("c"))

	select {
	case <-fired:
		t.Error("deadline callback should not fire when writes are timely")
	case <-time.After(50 * time.Millisecond):
		// Good.
	}
}

// ── v0.4 features ─────────────────────────────────────────────────────────────

func TestChannelDepth_ConfigurableSize(t *testing.T) {
	p := newParticipant(t)
	// Depth of 2: after 3 writes the 3rd should be dropped (DropNewest is default).
	sub, err := p.NewSubscriber("depth/test", dds.DefaultQoS, dds.WithChannelDepth(2))
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	pub, _ := p.NewPublisher("depth/test", dds.DefaultQoS)
	defer pub.Close()

	_ = pub.Write([]byte("a"))
	_ = pub.Write([]byte("b"))
	_ = pub.Write([]byte("c")) // should be dropped

	got := []string{}
	timeout := time.After(50 * time.Millisecond)
loop:
	for {
		select {
		case s := <-sub.C():
			got = append(got, string(s.Payload))
		case <-timeout:
			break loop
		}
	}
	if len(got) > 2 {
		t.Errorf("channel depth=2: got %d samples, want ≤2", len(got))
	}
}

func TestBackPressure_DropOldest(t *testing.T) {
	p := newParticipant(t)
	sub, err := p.NewSubscriber("bp/oldest", dds.DefaultQoS,
		dds.WithChannelDepth(1),
		dds.WithBackPressure(dds.DropOldest),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	pub, _ := p.NewPublisher("bp/oldest", dds.DefaultQoS)
	defer pub.Close()

	_ = pub.Write([]byte("first"))
	_ = pub.Write([]byte("second")) // evicts first, delivers second

	select {
	case s := <-sub.C():
		if string(s.Payload) != "second" {
			t.Errorf("DropOldest: got %q, want second", s.Payload)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

func TestBackPressure_Block(t *testing.T) {
	p := newParticipant(t)
	sub, err := p.NewSubscriber("bp/block", dds.DefaultQoS,
		dds.WithChannelDepth(2),
		dds.WithBackPressure(dds.Block),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	pub, _ := p.NewPublisher("bp/block", dds.DefaultQoS)
	defer pub.Close()

	// Both writes must succeed with Block policy.
	_ = pub.Write([]byte("one"))
	_ = pub.Write([]byte("two"))

	count := 0
	for count < 2 {
		select {
		case <-sub.C():
			count++
		case <-time.After(time.Second):
			t.Fatalf("timeout: only received %d/2 samples", count)
		}
	}
}

func TestLogger_MockLogsToHandler(t *testing.T) {
	// Verify WithLogger doesn't panic and allows logging.
	l := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	p, err := mock.New(0, mock.WithLogger(l))
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	pub, _ := p.NewPublisher("log/test", dds.DefaultQoS)
	defer pub.Close()
	_ = pub.Write([]byte("hello logger"))
}

func TestLiveliness_GainedOnNew(t *testing.T) {
	events := make(chan dds.LivelinessEvent, 4)
	cb := func(_ dds.GUID, ev dds.LivelinessEvent) {
		select {
		case events <- ev:
		default:
		}
	}

	p, err := mock.New(0, mock.WithLivelinessCallback(cb))
	if err != nil {
		t.Fatal(err)
	}

	select {
	case ev := <-events:
		if ev != dds.LivelinessGained {
			t.Errorf("expected LivelinessGained, got %d", ev)
		}
	case <-time.After(time.Second):
		t.Fatal("LivelinessGained not fired on New")
	}

	p.Close()
	select {
	case ev := <-events:
		if ev != dds.LivelinessLost {
			t.Errorf("expected LivelinessLost, got %d", ev)
		}
	case <-time.After(time.Second):
		t.Fatal("LivelinessLost not fired on Close")
	}
}

func TestCloseWithDrain_Mock(t *testing.T) {
	p, err := mock.New(0)
	if err != nil {
		t.Fatal(err)
	}
	// CloseWithDrain on mock should succeed immediately (all writes are sync).
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := dds.CloseWithDrain(ctx, p); err != nil {
		t.Errorf("CloseWithDrain: %v", err)
	}
}

func TestSampleTimestamp_SetOnPublish(t *testing.T) {
	p := newParticipant(t)
	sub, _ := p.NewSubscriber("ts/test", dds.DefaultQoS)
	defer sub.Close()
	pub, _ := p.NewPublisher("ts/test", dds.DefaultQoS)
	defer pub.Close()

	before := time.Now()
	_ = pub.Write([]byte("ts"))

	select {
	case s := <-sub.C():
		if s.Timestamp.IsZero() {
			t.Error("Timestamp must not be zero for mock delivery")
		}
		if s.Timestamp.Before(before) {
			t.Errorf("Timestamp %v is before write time %v", s.Timestamp, before)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

func TestTypedPublisherSubscriber_JSONCodec(t *testing.T) {
	type Msg struct {
		Value int `json:"value"`
	}

	p := newParticipant(t)
	rawPub, _ := p.NewPublisher("typed/json", dds.DefaultQoS)
	rawSub, _ := p.NewSubscriber("typed/json", dds.DefaultQoS)
	defer rawPub.Close()
	defer rawSub.Close()

	tp := dds.NewTypedPublisher[Msg](rawPub, dds.JSONCodec[Msg]{})
	ts := dds.NewTypedSubscriber[Msg](rawSub, dds.JSONCodec[Msg]{})
	defer ts.Close()

	if err := tp.Write(Msg{Value: 42}); err != nil {
		t.Fatalf("TypedPublisher.Write: %v", err)
	}

	select {
	case got := <-ts.C():
		if got.Value.Value != 42 {
			t.Errorf("TypedSubscriber: got value %d, want 42", got.Value.Value)
		}
		if got.Topic != "typed/json" {
			t.Errorf("Topic: got %q, want typed/json", got.Topic)
		}
	case <-time.After(time.Second):
		t.Fatal("typed sample not delivered")
	}
}

func TestTypedSubscriber_DecodeErrorDropped(t *testing.T) {
	p := newParticipant(t)
	rawPub, _ := p.NewPublisher("typed/err", dds.DefaultQoS)
	rawSub, _ := p.NewSubscriber("typed/err", dds.DefaultQoS)
	defer rawPub.Close()
	defer rawSub.Close()

	type Msg struct{ V int }
	ts := dds.NewTypedSubscriber[Msg](rawSub, dds.JSONCodec[Msg]{})
	defer ts.Close()

	// Write invalid JSON — should be silently dropped.
	_ = rawPub.Write([]byte("not-json"))

	select {
	case s := <-ts.C():
		t.Errorf("unexpected typed sample for bad payload: %v", s)
	case <-time.After(50 * time.Millisecond):
		// correct: bad payload was dropped
	}
}

func TestCloseWithDrain_NonDrainer(t *testing.T) {
	// Verify package-level CloseWithDrain falls back to Close on non-Drainer.
	// mock.participant is a Drainer; just verify the function doesn't error.
	p, _ := mock.New(0)
	ctx := context.Background()
	if err := dds.CloseWithDrain(ctx, p); err != nil {
		t.Errorf("CloseWithDrain: %v", err)
	}
	// Calling it again (on already-closed) should also be fine.
	if err := dds.CloseWithDrain(ctx, p); err != nil {
		t.Errorf("CloseWithDrain (idempotent): %v", err)
	}
}

func TestSentinelErrors_Wrapping(t *testing.T) {
	// Closed participant with non-empty topic → ErrClosed.
	p, _ := mock.New(0)
	p.Close()
	_, err := p.NewPublisher("x", dds.DefaultQoS)
	if !errors.Is(err, dds.ErrClosed) {
		t.Errorf("expected ErrClosed in chain, got %v", err)
	}

	// Open participant with empty topic → ErrTopicEmpty.
	p2, _ := mock.New(0)
	defer p2.Close()
	_, err = p2.NewPublisher("", dds.DefaultQoS)
	if !errors.Is(err, dds.ErrTopicEmpty) {
		t.Errorf("expected ErrTopicEmpty in chain, got %v", err)
	}
}

func TestMaxSampleSize_EnforcedInMock(t *testing.T) {
	p := newParticipant(t)
	qos := dds.DefaultQoS
	qos.MaxSampleSize = 10
	pub, err := p.NewPublisher("mock/maxsize", qos)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	// Payload within limit must succeed.
	if writeErr := pub.Write([]byte("0123456789")); writeErr != nil {
		t.Fatalf("Write at limit: %v", writeErr)
	}

	// Payload over limit must return ErrPayloadTooLarge.
	err = pub.Write([]byte("01234567890")) // 11 bytes
	if !errors.Is(err, dds.ErrPayloadTooLarge) {
		t.Errorf("expected ErrPayloadTooLarge, got %v", err)
	}
}

func TestMaxSampleSize_ZeroMeansUnlimited_Mock(t *testing.T) {
	p := newParticipant(t)
	qos := dds.DefaultQoS
	qos.MaxSampleSize = 0 // unlimited
	pub, _ := p.NewPublisher("mock/unlimited", qos)
	defer pub.Close()

	large := make([]byte, 100_000)
	if err := pub.Write(large); err != nil {
		t.Fatalf("Write with MaxSampleSize=0 should be unlimited: %v", err)
	}
}

func TestMetrics_MockParticipant(t *testing.T) {
	p := newParticipant(t)
	mp, ok := p.(dds.MetricsProvider)
	if !ok {
		t.Skip("mock does not implement MetricsProvider")
	}

	pub, _ := p.NewPublisher("metrics/mock", dds.DefaultQoS)
	sub, _ := p.NewSubscriber("metrics/mock", dds.DefaultQoS)
	defer sub.Close()

	_ = pub.Write([]byte("x"))
	select {
	case <-sub.C():
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for sample")
	}

	m := mp.Metrics()
	if m.WriteCount == 0 {
		t.Error("WriteCount should be > 0")
	}
	if m.DeliverCount == 0 {
		t.Error("DeliverCount should be > 0")
	}
}
