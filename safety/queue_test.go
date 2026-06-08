// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package safety_test

//fusa:test REQ-SAFETY-002
//fusa:test REQ-SAFETY-010
//fusa:test REQ-SAFETY-011
//fusa:test REQ-SAFETY-012
//fusa:test REQ-SAFETY-014
//fusa:test REQ-SEOOC-007

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/safety"
)

// ── DeterministicQueue ────────────────────────────────────────────────────────

func TestDeterministicQueue_DeliversSamples(t *testing.T) {
	p := newPart(t)
	topic := uniqueTopic("queue/deliver")
	sub, _ := p.NewSubscriber(topic, dds.DefaultQoS, dds.WithChannelDepth(10))
	defer sub.Close()
	pub, _ := p.NewPublisher(topic, dds.DefaultQoS)

	q := safety.NewDeterministicQueue(pub, 16).Start()
	defer q.Stop()

	if err := q.Enqueue([]byte("msg1")); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	select {
	case s := <-sub.C():
		if string(s.Payload) != "msg1" {
			t.Errorf("payload: got %q, want msg1", s.Payload)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout: enqueued sample not delivered")
	}
}

func TestDeterministicQueue_Full_ReturnsErrQueueFull(t *testing.T) {
	p := newPart(t)
	topic := uniqueTopic("queue/full")
	pub, _ := p.NewPublisher(topic, dds.DefaultQoS)
	defer pub.Close()

	// Depth 1, no Start — drain goroutine not running so channel fills immediately.
	q := safety.NewDeterministicQueue(pub, 1)

	if err := q.Enqueue([]byte("a")); err != nil {
		t.Fatalf("first Enqueue: unexpected error: %v", err)
	}
	err := q.Enqueue([]byte("b"))
	if !errors.Is(err, safety.ErrQueueFull) {
		t.Errorf("second Enqueue: expected ErrQueueFull, got %v", err)
	}
}

func TestDeterministicQueue_DefaultDepth(t *testing.T) {
	p := newPart(t)
	topic := uniqueTopic("queue/default-depth")
	pub, _ := p.NewPublisher(topic, dds.DefaultQoS)
	defer pub.Close()

	// depth <= 0 should default to 64
	q := safety.NewDeterministicQueue(pub, 0)
	// Fill 64 slots without Start (drain not running).
	for i := 0; i < 64; i++ {
		if err := q.Enqueue([]byte("x")); err != nil {
			t.Fatalf("Enqueue[%d]: unexpected error: %v", i, err)
		}
	}
	if err := q.Enqueue([]byte("overflow")); !errors.Is(err, safety.ErrQueueFull) {
		t.Errorf("slot 65: expected ErrQueueFull, got %v", err)
	}
}

func TestDeterministicQueue_Stop(t *testing.T) {
	p := newPart(t)
	topic := uniqueTopic("queue/stop")
	pub, _ := p.NewPublisher(topic, dds.DefaultQoS)
	defer pub.Close()

	q := safety.NewDeterministicQueue(pub, 8).Start()
	_ = q.Enqueue([]byte("x"))
	q.Stop()
	// After Stop, the drain goroutine has exited; further Enqueues are still
	// non-blocking (they write to the buffered channel) but will never be drained.
}

func TestDeterministicQueue_StopIdempotent(t *testing.T) {
	p := newPart(t)
	topic := uniqueTopic("queue/stop-idem")
	pub, _ := p.NewPublisher(topic, dds.DefaultQoS)
	defer pub.Close()

	q := safety.NewDeterministicQueue(pub, 8).Start()
	q.Stop()
	q.Stop() // must not panic
}

func TestDeterministicQueue_PanicContainment(t *testing.T) {
	// panicPublisher panics on every Write.
	pp := &panicPublisher{}
	q := safety.NewDeterministicQueue(pp, 8).Start()
	defer q.Stop()

	if err := q.Enqueue([]byte("boom")); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	select {
	case err := <-q.Errors():
		if err == nil {
			t.Fatal("expected non-nil error from panic")
		}
		if !strings.Contains(err.Error(), "publisher panic") {
			t.Errorf("unexpected error text: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout: expected panic error on Errors channel")
	}
}

func TestDeterministicQueue_ErrorsChannelReceivesWriteError(t *testing.T) {
	// errorPublisher returns an error on every Write.
	ep := &errorPublisher{err: errors.New("write failed")}
	q := safety.NewDeterministicQueue(ep, 8).Start()
	defer q.Stop()

	if err := q.Enqueue([]byte("x")); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	select {
	case err := <-q.Errors():
		if !strings.Contains(err.Error(), "write failed") {
			t.Errorf("unexpected error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout: expected write error on Errors channel")
	}
}

func TestDeterministicQueue_MultipleSamples(t *testing.T) {
	p := newPart(t)
	topic := uniqueTopic("queue/multi")
	sub, _ := p.NewSubscriber(topic, dds.DefaultQoS, dds.WithChannelDepth(20))
	defer sub.Close()
	pub, _ := p.NewPublisher(topic, dds.DefaultQoS)

	q := safety.NewDeterministicQueue(pub, 16).Start()
	defer q.Stop()

	const n = 5
	for i := 0; i < n; i++ {
		if err := q.Enqueue([]byte("x")); err != nil {
			t.Fatalf("Enqueue[%d]: %v", i, err)
		}
	}
	for i := 0; i < n; i++ {
		select {
		case <-sub.C():
		case <-time.After(time.Second):
			t.Fatalf("timeout waiting for sample %d/%d", i+1, n)
		}
	}
}

// ── stubs ─────────────────────────────────────────────────────────────────────

type panicPublisher struct{}

func (p *panicPublisher) Write(_ []byte) error                       { panic("test panic from publisher") }
func (p *panicPublisher) WriteCtx(_ context.Context, b []byte) error { return p.Write(b) }
func (p *panicPublisher) Close() error                               { return nil }

type errorPublisher struct{ err error }

func (p *errorPublisher) Write(_ []byte) error                       { return p.err }
func (p *errorPublisher) WriteCtx(_ context.Context, b []byte) error { return p.Write(b) }
func (p *errorPublisher) Close() error                               { return nil }
