// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package dds

// Internal tests for the relay.Node adapter's unexported deliver back-pressure
// logic, which is otherwise only reachable through racy timing in Subscribe.

import (
	"testing"

	relay "github.com/SoundMatt/RELAY/v2"
)

func newTestNode() *ddsNode { return &ddsNode{done: make(chan struct{})} }

func TestDeliver_DropNewest(t *testing.T) {
	n := newTestNode()
	ch := make(chan relay.Message, 1)
	ch <- relay.Message{ID: "first"} // fill the channel

	// DropNewest: arriving message is silently dropped, returns true (keep going).
	if !n.deliver(ch, relay.Message{ID: "second"}, relay.DropNewest) {
		t.Fatal("DropNewest should return true")
	}
	if got := <-ch; got.ID != "first" {
		t.Errorf("DropNewest evicted the wrong message: got %q, want %q", got.ID, "first")
	}
}

func TestDeliver_DropOldest(t *testing.T) {
	n := newTestNode()
	ch := make(chan relay.Message, 1)
	ch <- relay.Message{ID: "old"}

	if !n.deliver(ch, relay.Message{ID: "new"}, relay.DropOldest) {
		t.Fatal("DropOldest should return true")
	}
	if got := <-ch; got.ID != "new" {
		t.Errorf("DropOldest kept the wrong message: got %q, want %q", got.ID, "new")
	}
}

func TestDeliver_EmptyChannelTakesFastPath(t *testing.T) {
	n := newTestNode()
	ch := make(chan relay.Message, 1)
	if !n.deliver(ch, relay.Message{ID: "x"}, relay.DropNewest) {
		t.Fatal("deliver to empty channel should return true")
	}
	if got := <-ch; got.ID != "x" {
		t.Errorf("got %q, want x", got.ID)
	}
}

func TestDeliver_Block(t *testing.T) {
	n := newTestNode()
	ch := make(chan relay.Message, 1)
	if !n.deliver(ch, relay.Message{ID: "a"}, relay.Block) {
		t.Fatal("Block delivery to free channel should return true")
	}
	<-ch // drain
}

func TestDeliver_BlockReturnsFalseWhenClosing(t *testing.T) {
	n := newTestNode()
	ch := make(chan relay.Message) // unbuffered, no reader
	close(n.done)
	if n.deliver(ch, relay.Message{ID: "a"}, relay.Block) {
		t.Fatal("Block delivery should return false once the node is closing")
	}
}

func TestDeliver_NonBlockReturnsFalseWhenClosing(t *testing.T) {
	n := newTestNode()
	ch := make(chan relay.Message) // unbuffered, no reader
	close(n.done)
	if n.deliver(ch, relay.Message{ID: "a"}, relay.DropNewest) {
		t.Fatal("non-blocking delivery should return false once the node is closing")
	}
}
