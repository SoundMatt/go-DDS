// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// RELAY spec v1.10 conformance tests: Sample<->Message round-trip against the
// published golden vector (spec/vectors/dds-sample.json), domain validation
// (spec/vectors/errors/dds-domain-out-of-range.json), and the Adapt() Node
// adapter's WithTopic subscription routing (§14.1).

package dds_test

//fusa:test REQ-PART-001
//fusa:test REQ-SUB-001
//fusa:test REQ-LLR-006

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	relay "github.com/SoundMatt/RELAY"
	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/mock"
)

func TestSpecVersion_TracksRelay(t *testing.T) {
	if dds.SpecVersion != relay.SpecVersion {
		t.Errorf("SpecVersion = %q, want it to track relay.SpecVersion %q", dds.SpecVersion, relay.SpecVersion)
	}
	if dds.SpecVersion != "1.14" {
		t.Errorf("SpecVersion = %q, want %q (RELAY v1.14 spec)", dds.SpecVersion, "1.14")
	}
}

// TestSample_ToMessage_GoldenVector reproduces spec/vectors/dds-sample.json and
// asserts ToMessage() yields exactly the canonical envelope in that vector.
func TestSample_ToMessage_GoldenVector(t *testing.T) {
	var guid dds.GUID
	for i := range guid {
		guid[i] = byte(i + 1) // [1,2,...,16]
	}
	s := dds.Sample{
		Topic:          "rt/chatter",
		Payload:        []byte("hello dds"), // base64 "aGVsbG8gZGRz"
		Timestamp:      time.Time{},         // 0001-01-01T00:00:00Z
		SequenceNumber: 7,
		WriterGUID:     guid,
	}

	got := s.ToMessage()
	want := relay.Message{
		Protocol:  relay.DDS,
		ID:        "rt/chatter",
		Payload:   []byte("hello dds"),
		Timestamp: time.Time{},
		Seq:       7,
		Meta:      map[string]string{"dds.writer_guid": "0102030405060708090a0b0c0d0e0f10"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ToMessage() mismatch:\n got  %+v\n want %+v", got, want)
	}
}

// TestSample_RoundTrip verifies FromMessage(ToMessage(s)) == s for the vector.
func TestSample_RoundTrip(t *testing.T) {
	var guid dds.GUID
	for i := range guid {
		guid[i] = byte(i + 1)
	}
	orig := dds.Sample{
		Topic:          "rt/chatter",
		Payload:        []byte("hello dds"),
		Timestamp:      time.Time{},
		SequenceNumber: 7,
		WriterGUID:     guid,
	}

	back, err := dds.FromMessage(orig.ToMessage())
	if err != nil {
		t.Fatalf("FromMessage: %v", err)
	}
	if !reflect.DeepEqual(back, orig) {
		t.Fatalf("round-trip mismatch:\n got  %+v\n want %+v", back, orig)
	}
}

func TestValidateDomain(t *testing.T) {
	cases := []struct {
		d  dds.Domain
		ok bool
	}{
		{0, true}, {1, true}, {232, true},
		{-1, false}, {233, false}, {1000, false},
	}
	for _, c := range cases {
		err := dds.ValidateDomain(c.d)
		if c.ok && err != nil {
			t.Errorf("ValidateDomain(%d) = %v, want nil", c.d, err)
		}
		if !c.ok {
			if err == nil {
				t.Errorf("ValidateDomain(%d) = nil, want ErrDomainOutOfRange", c.d)
				continue
			}
			if !errors.Is(err, dds.ErrDomainOutOfRange) {
				t.Errorf("ValidateDomain(%d) error does not wrap ErrDomainOutOfRange: %v", c.d, err)
			}
			// Domain-out-of-range remains a connection-setup failure (wraps relay.ErrNotConnected).
			if !errors.Is(err, relay.ErrNotConnected) {
				t.Errorf("ValidateDomain(%d) error does not wrap relay.ErrNotConnected: %v", c.d, err)
			}
		}
	}
}

// TestAdapt_Subscribe_WithTopic verifies the Node adapter creates a real DDS
// subscription from relay.WithTopic and forwards published samples (§14.1).
func TestAdapt_Subscribe_WithTopic(t *testing.T) {
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		t.Fatalf("mock.New: %v", err)
	}
	node := dds.Adapt(p)
	defer func() { _ = node.Close() }()

	ch, err := node.Subscribe(relay.WithTopic("conformance/topic"))
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	ctx := context.Background()
	if err := node.Send(ctx, relay.Message{Protocol: relay.DDS, ID: "conformance/topic", Payload: []byte("ping")}); err != nil {
		t.Fatalf("Send: %v", err)
	}

	select {
	case msg, ok := <-ch:
		if !ok {
			t.Fatal("channel closed before a message arrived")
		}
		if string(msg.Payload) != "ping" {
			t.Errorf("payload = %q, want %q", msg.Payload, "ping")
		}
		if msg.ID != "conformance/topic" {
			t.Errorf("ID = %q, want %q", msg.ID, "conformance/topic")
		}
		if msg.Protocol != relay.DDS {
			t.Errorf("Protocol = %v, want relay.DDS", msg.Protocol)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for forwarded sample")
	}
}

// TestAdapt_Subscribe_NoTopic asserts the adapter rejects a topic-less
// subscription with ErrNotConnected, per spec §14.1.
func TestAdapt_Subscribe_NoTopic(t *testing.T) {
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		t.Fatalf("mock.New: %v", err)
	}
	node := dds.Adapt(p)
	defer func() { _ = node.Close() }()

	ch, err := node.Subscribe()
	if err == nil {
		t.Fatal("expected error for topic-less Subscribe")
	}
	if !errors.Is(err, relay.ErrNotConnected) {
		t.Errorf("error does not wrap relay.ErrNotConnected: %v", err)
	}
	if ch != nil {
		t.Error("expected nil channel on error")
	}
}

func TestAdapt_Subscribe_AfterClose(t *testing.T) {
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		t.Fatalf("mock.New: %v", err)
	}
	node := dds.Adapt(p)
	_ = node.Close()

	ch, err := node.Subscribe(relay.WithTopic("x"))
	if !errors.Is(err, relay.ErrClosed) {
		t.Errorf("Subscribe after Close: error does not wrap relay.ErrClosed: %v", err)
	}
	// A closed node returns a closed channel, not nil, for the lifecycle path.
	if ch != nil {
		if _, ok := <-ch; ok {
			t.Error("expected closed channel after Close")
		}
	}
}
