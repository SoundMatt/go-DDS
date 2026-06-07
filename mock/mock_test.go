// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package mock_test

import (
	"context"
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
