// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package mock_test

import (
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

	pub.Write([]byte("should not arrive"))

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

	pub.Write([]byte("broadcast"))

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
	pub.Write(orig)
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

	pub.Write([]byte("cross"))
	select {
	case <-sub.C():
	case <-time.After(time.Second):
		t.Fatal("cross-domain delivery failed in mock")
	}
}
