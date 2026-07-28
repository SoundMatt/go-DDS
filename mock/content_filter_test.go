// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Tests for Milestone 15 "Content-Filtered Topics" in the mock backend. The
// same scenarios are exercised for the rtps and shmem backends in
// content_filter_test.go — see cfilter's own tests for predicate-language
// coverage; these tests only cover the NewFilteredSubscriber wiring.

package mock_test

//fusa:test REQ-CFILT-006
//fusa:test REQ-CFILT-007
//fusa:test REQ-CFILT-008

import (
	"testing"
	"time"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/mock"
)

func TestMock_NewFilteredSubscriber_MatchAndReject(t *testing.T) {
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer p.Close()

	sub, err := dds.NewFilteredSubscriber(p, "signals", "x > 42 AND status = 'active'", nil, dds.QoS{})
	if err != nil {
		t.Fatalf("NewFilteredSubscriber: %v", err)
	}
	defer sub.Close()

	pub, err := p.NewPublisher("signals", dds.QoS{})
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	// Matches: delivered.
	if err := pub.Write([]byte(`{"x": 43, "status": "active"}`)); err != nil {
		t.Fatalf("Write: %v", err)
	}
	s, ok := drainOneQoS(t, sub.C(), time.Second)
	if !ok {
		t.Fatal("expected matching sample to be delivered")
	}
	if string(s.Payload) != `{"x": 43, "status": "active"}` {
		t.Errorf("unexpected payload: %s", s.Payload)
	}

	// Does not match: silently dropped, never delivered.
	if err := pub.Write([]byte(`{"x": 1, "status": "active"}`)); err != nil {
		t.Fatalf("Write: %v", err)
	}
	expectNoneQoS(t, sub.C(), 100*time.Millisecond)
}

func TestMock_NewFilteredSubscriber_InvalidExpr(t *testing.T) {
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer p.Close()

	if _, err := dds.NewFilteredSubscriber(p, "signals", "x >", nil, dds.QoS{}); err == nil {
		t.Error("expected error for invalid predicate expression")
	}
}

func TestMock_NewFilteredSubscriber_EmptyTopic(t *testing.T) {
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer p.Close()

	if _, err := dds.NewFilteredSubscriber(p, "", "x > 1", nil, dds.QoS{}); err == nil {
		t.Error("expected error for empty topic")
	}
}

func TestMock_NewFilteredSubscriber_ClosedParticipant(t *testing.T) {
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	p.Close()

	if _, err := dds.NewFilteredSubscriber(p, "signals", "x > 1", nil, dds.QoS{}); err == nil {
		t.Error("expected ErrClosed after Close")
	}
}

func TestMock_NewFilteredSubscriber_WithParams(t *testing.T) {
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer p.Close()

	sub, err := dds.NewFilteredSubscriber(p, "signals", "status = %0", []string{"active"}, dds.QoS{})
	if err != nil {
		t.Fatalf("NewFilteredSubscriber: %v", err)
	}
	defer sub.Close()

	pub, err := p.NewPublisher("signals", dds.QoS{})
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	if err := pub.Write([]byte(`{"status": "active"}`)); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, ok := drainOneQoS(t, sub.C(), time.Second); !ok {
		t.Fatal("expected param-bound predicate to match")
	}
}

// TestMock_NewFilteredSubscriber_WildcardTopicCompatible proves content
// filters compose with MQTT-style wildcard topic subscriptions (Milestone
// 11), as required by ROADMAP.md's Milestone 15 "Content-Filtered Topics".
func TestMock_NewFilteredSubscriber_WildcardTopicCompatible(t *testing.T) {
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer p.Close()

	sub, err := dds.NewFilteredSubscriber(p, "sensors/+", "x > 10", nil, dds.QoS{})
	if err != nil {
		t.Fatalf("NewFilteredSubscriber: %v", err)
	}
	defer sub.Close()

	pub, err := p.NewPublisher("sensors/a", dds.QoS{})
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	if err := pub.Write([]byte(`{"x": 11}`)); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, ok := drainOneQoS(t, sub.C(), time.Second); !ok {
		t.Fatal("expected wildcard-matched topic + passing predicate to be delivered")
	}

	if err := pub.Write([]byte(`{"x": 1}`)); err != nil {
		t.Fatalf("Write: %v", err)
	}
	expectNoneQoS(t, sub.C(), 100*time.Millisecond)
}

// TestMock_NewFilteredSubscriber_TypeAssertion proves mock's Participant
// implements dds.ContentFilteredSubscriberFactory directly, not just via
// the dds.NewFilteredSubscriber convenience wrapper.
func TestMock_NewFilteredSubscriber_TypeAssertion(t *testing.T) {
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer p.Close()

	if _, ok := p.(dds.ContentFilteredSubscriberFactory); !ok {
		t.Fatal("mock participant does not implement dds.ContentFilteredSubscriberFactory")
	}
}
