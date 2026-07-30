// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package dds_test

// Tests for the relay.Node adapter's Send/Protocol/Close paths and the no-op
// tracer.

import (
	"context"
	"errors"
	"testing"
	"time"

	relay "github.com/SoundMatt/RELAY/v2"
	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/mock"
)

func newNode(t *testing.T) (relay.Node, dds.Participant) {
	t.Helper()
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		t.Fatalf("mock.New: %v", err)
	}
	return dds.Adapt(p), p
}

func TestAdapt_Protocol(t *testing.T) {
	n, p := newNode(t)
	defer func() { _ = p.Close() }()
	if n.Protocol() != relay.DDS {
		t.Errorf("Protocol() = %v, want %v", n.Protocol(), relay.DDS)
	}
}

func TestAdapt_Send(t *testing.T) {
	n, p := newNode(t)
	defer func() { _ = p.Close() }()
	ctx := context.Background()

	// First send to a topic creates the publisher.
	if err := n.Send(ctx, relay.Message{ID: "rt/topic", Payload: []byte("a")}); err != nil {
		t.Fatalf("first Send: %v", err)
	}
	// Second send to the same topic uses the cached publisher.
	if err := n.Send(ctx, relay.Message{ID: "rt/topic", Payload: []byte("b")}); err != nil {
		t.Fatalf("cached Send: %v", err)
	}
}

func TestAdapt_Send_EmptyID(t *testing.T) {
	n, p := newNode(t)
	defer func() { _ = p.Close() }()
	if err := n.Send(context.Background(), relay.Message{Payload: []byte("x")}); !errors.Is(err, dds.ErrTopicEmpty) {
		t.Errorf("Send with empty ID = %v, want ErrTopicEmpty", err)
	}
}

func TestAdapt_Send_AfterClose(t *testing.T) {
	n, p := newNode(t)
	defer func() { _ = p.Close() }()
	if err := n.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := n.Send(context.Background(), relay.Message{ID: "t", Payload: []byte("x")}); !errors.Is(err, dds.ErrClosed) {
		t.Errorf("Send after close = %v, want ErrClosed", err)
	}
}

func TestAdapt_Close_Idempotent(t *testing.T) {
	n, p := newNode(t)
	defer func() { _ = p.Close() }()
	if err := n.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := n.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

// TestAdapt_Send_RoundTrip publishes via the Node and receives via a Subscribe
// channel, exercising Send → ToMessage delivery end to end.
func TestAdapt_Send_RoundTrip(t *testing.T) {
	n, p := newNode(t)
	defer func() { _ = p.Close() }()

	ch, err := n.Subscribe(relay.WithTopic("rt/rtt"))
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	if err := n.Send(context.Background(), relay.Message{ID: "rt/rtt", Payload: []byte("ping")}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	select {
	case msg := <-ch:
		if string(msg.Payload) != "ping" {
			t.Errorf("payload = %q, want ping", msg.Payload)
		}
		if msg.ID != "rt/rtt" {
			t.Errorf("id = %q, want rt/rtt", msg.ID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no message received")
	}
}

func TestNoopTracer(t *testing.T) {
	ctx, span := dds.NoopTracer.Start(context.Background(), "op",
		dds.SpanAttribute{Key: "k", Value: "v"})
	if ctx == nil {
		t.Fatal("NoopTracer returned nil context")
	}
	// These must be no-ops and must not panic.
	span.SetAttribute("a", "b")
	span.End()
}
