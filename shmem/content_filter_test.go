// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Tests for Milestone 15 "Content-Filtered Topics" in the shmem backend. The
// same scenarios are exercised for the mock and rtps backends — see
// mock/content_filter_test.go and rtps/content_filter_test.go — proving
// identical cross-backend semantics.

package shmem_test

//fusa:test REQ-CFILT-006
//fusa:test REQ-CFILT-008

import (
	"testing"
	"time"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/shmem"
)

func drainOneCF(t *testing.T, ch <-chan dds.Sample, timeout time.Duration) (dds.Sample, bool) {
	t.Helper()
	select {
	case s, ok := <-ch:
		return s, ok
	case <-time.After(timeout):
		return dds.Sample{}, false
	}
}

func expectNoneCF(t *testing.T, ch <-chan dds.Sample, wait time.Duration) {
	t.Helper()
	select {
	case s, ok := <-ch:
		if ok {
			t.Fatalf("expected no delivery, got sample: %s", s.Payload)
		}
	case <-time.After(wait):
	}
}

func newTestParticipant(t *testing.T) dds.Participant {
	t.Helper()
	p, err := shmem.New(dds.Domain(0))
	if err != nil {
		t.Fatalf("shmem.New: %v", err)
	}
	t.Cleanup(func() { _ = p.Close() })
	return p
}

func TestShmem_NewFilteredSubscriber_MatchAndReject(t *testing.T) {
	p := newTestParticipant(t)

	sub, err := dds.NewFilteredSubscriber(p, "cfilter-signals-1", "x > 42 AND status = 'active'", nil, dds.QoS{})
	if err != nil {
		t.Fatalf("NewFilteredSubscriber: %v", err)
	}
	defer sub.Close()

	pub, err := p.NewPublisher("cfilter-signals-1", dds.QoS{})
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	// Write the non-matching sample first and confirm it is never delivered
	// (via either shmem's in-process broker path or its cross-process
	// self-loopback listener path) before writing a matching sample — this
	// ordering avoids racing the two delivery paths' independent timing
	// against each other (see shmSubscriber.pump).
	if err := pub.Write([]byte(`{"x": 1, "status": "active"}`)); err != nil {
		t.Fatalf("Write: %v", err)
	}
	expectNoneCF(t, sub.C(), 200*time.Millisecond)

	if err := pub.Write([]byte(`{"x": 43, "status": "active"}`)); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, ok := drainOneCF(t, sub.C(), time.Second); !ok {
		t.Fatal("expected matching sample to be delivered")
	}
}

func TestShmem_NewFilteredSubscriber_InvalidExpr(t *testing.T) {
	p := newTestParticipant(t)

	if _, err := dds.NewFilteredSubscriber(p, "cfilter-signals-3", "x >", nil, dds.QoS{}); err == nil {
		t.Error("expected error for invalid predicate expression")
	}
}

func TestShmem_NewFilteredSubscriber_EmptyTopic(t *testing.T) {
	p := newTestParticipant(t)

	if _, err := dds.NewFilteredSubscriber(p, "", "x > 1", nil, dds.QoS{}); err == nil {
		t.Error("expected error for empty topic")
	}
}

func TestShmem_NewFilteredSubscriber_WithParams(t *testing.T) {
	p := newTestParticipant(t)

	sub, err := dds.NewFilteredSubscriber(p, "cfilter-signals-2", "status = %0", []string{"active"}, dds.QoS{})
	if err != nil {
		t.Fatalf("NewFilteredSubscriber: %v", err)
	}
	defer sub.Close()

	pub, err := p.NewPublisher("cfilter-signals-2", dds.QoS{})
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	if err := pub.Write([]byte(`{"status": "active"}`)); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, ok := drainOneCF(t, sub.C(), time.Second); !ok {
		t.Fatal("expected param-bound predicate to match")
	}
}

// TestShmem_NewFilteredSubscriber_WildcardTopicCompatible proves content
// filters compose with MQTT-style wildcard topic subscriptions (Milestone
// 11), as required by ROADMAP.md's Milestone 15 "Content-Filtered Topics".
func TestShmem_NewFilteredSubscriber_WildcardTopicCompatible(t *testing.T) {
	p := newTestParticipant(t)

	sub, err := dds.NewFilteredSubscriber(p, "cfilter-sensors/+", "x > 10", nil, dds.QoS{})
	if err != nil {
		t.Fatalf("NewFilteredSubscriber: %v", err)
	}
	defer sub.Close()

	pub, err := p.NewPublisher("cfilter-sensors/a", dds.QoS{})
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	// Non-matching predicate first (see MatchAndReject for why this
	// ordering matters for shmem specifically).
	if err := pub.Write([]byte(`{"x": 1}`)); err != nil {
		t.Fatalf("Write: %v", err)
	}
	expectNoneCF(t, sub.C(), 200*time.Millisecond)

	if err := pub.Write([]byte(`{"x": 11}`)); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, ok := drainOneCF(t, sub.C(), time.Second); !ok {
		t.Fatal("expected wildcard-matched topic + passing predicate to be delivered")
	}
}

// TestShmem_NewFilteredSubscriber_TypeAssertion proves shmem's Participant
// implements dds.ContentFilteredSubscriberFactory directly.
func TestShmem_NewFilteredSubscriber_TypeAssertion(t *testing.T) {
	p := newTestParticipant(t)

	if _, ok := p.(dds.ContentFilteredSubscriberFactory); !ok {
		t.Fatal("shmem participant does not implement dds.ContentFilteredSubscriberFactory")
	}
}
