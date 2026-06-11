// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package auto_test

//fusa:test REQ-AUTO-001
//fusa:test REQ-AUTO-002
//fusa:test REQ-AUTO-003

import (
	"testing"
	"time"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/auto"
	"github.com/SoundMatt/go-DDS/rtps"
)

func TestNewParticipant_auto_roundtrip(t *testing.T) {
	p, err := auto.NewParticipant(dds.Domain(0))
	if err != nil {
		t.Fatalf("NewParticipant: %v", err)
	}
	defer p.Close()

	sub, err := p.NewSubscriber("auto/test", dds.DefaultQoS)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()

	pub, err := p.NewPublisher("auto/test", dds.DefaultQoS)
	if err != nil {
		t.Fatal(err)
	}
	defer pub.Close()

	if err := pub.Write([]byte("hello")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	select {
	case s := <-sub.C():
		if string(s.Payload) != "hello" {
			t.Fatalf("got %q want %q", s.Payload, "hello")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for sample")
	}
}

func TestNewParticipant_rtps(t *testing.T) {
	p, err := auto.NewParticipant(dds.Domain(1), auto.WithTransport(auto.TransportRTPS))
	if err != nil {
		t.Fatalf("NewParticipant(RTPS): %v", err)
	}
	defer p.Close()

	if p.Domain() != dds.Domain(1) {
		t.Fatalf("domain: got %v want 1", p.Domain())
	}
}

func TestNewParticipant_shmem(t *testing.T) {
	p, err := auto.NewParticipant(dds.Domain(2), auto.WithTransport(auto.TransportShmem))
	if err != nil {
		t.Fatalf("NewParticipant(Shmem): %v", err)
	}
	defer p.Close()

	if p.Domain() != dds.Domain(2) {
		t.Fatalf("domain: got %v want 2", p.Domain())
	}
}

func TestTransport_String(t *testing.T) {
	cases := []struct {
		t    auto.Transport
		want string
	}{
		{auto.TransportAuto, "auto"},
		{auto.TransportShmem, "shmem"},
		{auto.TransportRTPS, "rtps"},
	}
	for _, c := range cases {
		if got := c.t.String(); got != c.want {
			t.Errorf("Transport(%d).String() = %q, want %q", c.t, got, c.want)
		}
	}
}

func TestNewParticipant_domain_preserved(t *testing.T) {
	p, err := auto.NewParticipant(dds.Domain(42))
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()

	if p.Domain() != dds.Domain(42) {
		t.Fatalf("domain: got %v want 42", p.Domain())
	}
}

func TestNewParticipant_with_rtps_opts(t *testing.T) {
	// WithRTPSOpts passes options through to the RTPS backend.
	p, err := auto.NewParticipant(dds.Domain(5),
		auto.WithTransport(auto.TransportRTPS),
		auto.WithRTPSOpts(rtps.WithNoMulticast()),
	)
	if err != nil {
		t.Fatalf("NewParticipant with rtps opts: %v", err)
	}
	defer p.Close()

	if p.Domain() != dds.Domain(5) {
		t.Fatalf("domain: got %v want 5", p.Domain())
	}

	// Verify pub/sub still works with the option applied.
	sub, err := p.NewSubscriber("auto/opts/test", dds.DefaultQoS)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()

	pub, err := p.NewPublisher("auto/opts/test", dds.DefaultQoS)
	if err != nil {
		t.Fatal(err)
	}
	defer pub.Close()

	if writeErr := pub.Write([]byte("opts")); writeErr != nil {
		t.Fatalf("Write: %v", writeErr)
	}
	select {
	case s := <-sub.C():
		if string(s.Payload) != "opts" {
			t.Fatalf("got %q want opts", s.Payload)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
}
