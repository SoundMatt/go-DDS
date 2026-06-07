// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package shmem_test

import (
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
