// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Tests for Milestone 15 "Content-Filtered Topics" in the rtps backend. The
// same in-process scenarios are exercised for the mock and shmem backends —
// see mock/content_filter_test.go and shmem/content_filter_test.go — proving
// identical cross-backend semantics. The cross-process tests here additionally
// prove the rtps-specific "evaluated at the publisher, before transmission"
// contract: a matched writer never sends DATA over UDP for a sample its
// remote reader's predicate rejects (see rtpsWriter.Write /
// participant.matchedReaderLocators), which the mock and shmem in-process
// tests cannot exercise since there is no real network hop to withhold.

package rtps_test

//fusa:test REQ-CFILT-006
//fusa:test REQ-CFILT-007
//fusa:test REQ-CFILT-008

import (
	"runtime"
	"testing"
	"time"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/rtps"
)

func drainOneCFT(t *testing.T, ch <-chan dds.Sample, timeout time.Duration) (dds.Sample, bool) {
	t.Helper()
	select {
	case s, ok := <-ch:
		return s, ok
	case <-time.After(timeout):
		return dds.Sample{}, false
	}
}

func expectNoneCFT(t *testing.T, ch <-chan dds.Sample, wait time.Duration) {
	t.Helper()
	select {
	case s, ok := <-ch:
		if ok {
			t.Fatalf("expected no delivery, got sample: %s", s.Payload)
		}
	case <-time.After(wait):
	}
}

// ── Intra-process ────────────────────────────────────────────────────────────

func TestRTPS_NewFilteredSubscriber_IntraProcess_MatchAndReject(t *testing.T) {
	p := newTestParticipant(t)

	sub, err := dds.NewFilteredSubscriber(p, "cfilter/intra1", "x > 42 AND status = 'active'", nil, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewFilteredSubscriber: %v", err)
	}
	defer sub.Close()

	pub, err := p.NewPublisher("cfilter/intra1", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	if err := pub.Write([]byte(`{"x": 1, "status": "active"}`)); err != nil {
		t.Fatalf("Write: %v", err)
	}
	expectNoneCFT(t, sub.C(), 200*time.Millisecond)

	if err := pub.Write([]byte(`{"x": 43, "status": "active"}`)); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, ok := drainOneCFT(t, sub.C(), time.Second); !ok {
		t.Fatal("expected matching sample to be delivered")
	}
}

func TestRTPS_NewFilteredSubscriber_InvalidExpr(t *testing.T) {
	p := newTestParticipant(t)

	if _, err := dds.NewFilteredSubscriber(p, "cfilter/invalid", "x >", nil, dds.DefaultQoS); err == nil {
		t.Error("expected error for invalid predicate expression")
	}
}

func TestRTPS_NewFilteredSubscriber_EmptyTopic(t *testing.T) {
	p := newTestParticipant(t)

	if _, err := dds.NewFilteredSubscriber(p, "", "x > 1", nil, dds.DefaultQoS); err == nil {
		t.Error("expected error for empty topic")
	}
}

func TestRTPS_NewFilteredSubscriber_TypeAssertion(t *testing.T) {
	p := newTestParticipant(t)

	if _, ok := p.(dds.ContentFilteredSubscriberFactory); !ok {
		t.Fatal("rtps participant does not implement dds.ContentFilteredSubscriberFactory")
	}
}

// ── Cross-process ────────────────────────────────────────────────────────────

// TestRTPS_NewFilteredSubscriber_CrossProcess proves the rtps backend
// propagates a NewFilteredSubscriber's predicate to the matched remote
// writer via SEDP (pidContentFilterExpr / pidContentFilterParam) and that
// the writer evaluates it before ever transmitting DATA for a non-matching
// sample — the same behavioral proof TestRTPS_TwoParticipants_SameHost uses
// for plain delivery, extended to show a non-matching write never arrives.
func TestRTPS_NewFilteredSubscriber_CrossProcess(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping two-participant test in -short mode (requires network)")
	}
	if runtime.GOOS != "linux" {
		t.Skipf("UDP multicast loopback unreliable on %s; skipping cross-participant test", runtime.GOOS)
	}

	p1, err := rtps.New(testDomain)
	if err != nil {
		t.Skipf("rtps.New(p1): %v", err)
	}
	defer p1.Close()

	p2, err := rtps.New(testDomain)
	if err != nil {
		t.Skipf("rtps.New(p2): %v", err)
	}
	defer p2.Close()

	sub, err := dds.NewFilteredSubscriber(p2, "test/cfilter/cross", "x > 42 AND status = 'active'", nil, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewFilteredSubscriber(p2): %v", err)
	}
	defer sub.Close()

	pub, err := p1.NewPublisher("test/cfilter/cross", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher(p1): %v", err)
	}
	defer pub.Close()

	// Allow SPDP + SEDP (including the content-filter announcement) to
	// complete (within the 2 s announce period).
	time.Sleep(2200 * time.Millisecond)

	// Non-matching sample: the writer must never transmit it.
	if err := pub.Write([]byte(`{"x": 1, "status": "active"}`)); err != nil {
		t.Fatalf("Write (non-matching): %v", err)
	}
	select {
	case s, ok := <-sub.C():
		if ok {
			t.Fatalf("expected non-matching sample to never be delivered, got %s", s.Payload)
		}
	case <-time.After(500 * time.Millisecond):
	}

	// Matching sample: delivered normally.
	want := []byte(`{"x": 43, "status": "active"}`)
	if err := pub.Write(want); err != nil {
		t.Fatalf("Write (matching): %v", err)
	}
	select {
	case s := <-sub.C():
		if string(s.Payload) != string(want) {
			t.Errorf("payload: got %q, want %q", s.Payload, want)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout: matching cross-participant sample not received")
	}
}

// TestRTPS_NewFilteredSubscriber_CrossProcess_UnfilteredReaderUnaffected
// proves a content filter registered by one reader never withholds samples
// from a different, plain (unfiltered) reader matched to the same writer —
// guarding against the OR-merge locator-sharing logic in
// participant.matchedReaderLocators over-restricting an unfiltered peer.
func TestRTPS_NewFilteredSubscriber_CrossProcess_UnfilteredReaderUnaffected(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping two-participant test in -short mode (requires network)")
	}
	if runtime.GOOS != "linux" {
		t.Skipf("UDP multicast loopback unreliable on %s; skipping cross-participant test", runtime.GOOS)
	}

	p1, err := rtps.New(testDomain)
	if err != nil {
		t.Skipf("rtps.New(p1): %v", err)
	}
	defer p1.Close()

	p2, err := rtps.New(testDomain)
	if err != nil {
		t.Skipf("rtps.New(p2): %v", err)
	}
	defer p2.Close()

	filtered, err := dds.NewFilteredSubscriber(p2, "test/cfilter/mixed", "x > 42", nil, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewFilteredSubscriber(p2): %v", err)
	}
	defer filtered.Close()

	plain, err := p2.NewSubscriber("test/cfilter/mixed", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber(p2): %v", err)
	}
	defer plain.Close()

	pub, err := p1.NewPublisher("test/cfilter/mixed", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher(p1): %v", err)
	}
	defer pub.Close()

	time.Sleep(2200 * time.Millisecond)

	nonMatching := []byte(`{"x": 1}`)
	if err := pub.Write(nonMatching); err != nil {
		t.Fatalf("Write: %v", err)
	}

	// The plain subscriber must still receive it even though the filtered
	// one won't.
	select {
	case s := <-plain.C():
		if string(s.Payload) != string(nonMatching) {
			t.Errorf("plain subscriber payload: got %q, want %q", s.Payload, nonMatching)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout: unfiltered subscriber never received sample rejected by the filtered one")
	}

	select {
	case s, ok := <-filtered.C():
		if ok {
			t.Fatalf("expected filtered subscriber to reject non-matching sample, got %s", s.Payload)
		}
	case <-time.After(300 * time.Millisecond):
	}
}
