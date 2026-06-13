// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Internal tests for unexported paths in the domain bridge package.
// Kept in package domain so that b.subs and b.pubs are accessible.

package domain

//fusa:test REQ-BRIDGE-001
//fusa:test REQ-BRIDGE-002

import (
	"fmt"
	"testing"
	"time"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/mock"
)

func newTestPart(t *testing.T) dds.Participant {
	t.Helper()
	p, err := mock.New(0)
	if err != nil {
		t.Fatalf("mock.New: %v", err)
	}
	t.Cleanup(func() { _ = p.Close() })
	return p
}

// TestForward_SubscriberChannelClosed covers the !ok branch in forward (line 127-129)
// where the subscriber's channel is closed while the goroutine is running.
// We access b.subs directly (internal test) and close the subscriber before
// Close() signals b.done, ensuring the goroutine exits via the !ok path.
func TestForward_SubscriberChannelClosed(t *testing.T) {
	src := newTestPart(t)
	dst := newTestPart(t)
	topic := fmt.Sprintf("internal/forward-sub-closed/%d", time.Now().UnixNano())

	b, err := New(src, dst, Options{Topics: []string{topic}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	b.Start()

	// Close the bridge's own subscriber directly so that sub.C() closes.
	// This fires the !ok branch before b.done is signalled.
	b.subs[0].Close()

	// Give the goroutine time to observe the closed channel and decrement wg.
	time.Sleep(30 * time.Millisecond)

	// b.Close() will close b.done (already fired) and wait on wg (already at 0).
	if err := b.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

// TestForward_PublisherWriteError covers the pub.Write error return in forward
// (line 130-132). We close the bridge's dst publisher directly, then publish a
// sample so the forward goroutine tries to write to the closed publisher and
// returns on error.
func TestForward_PublisherWriteError(t *testing.T) {
	src := newTestPart(t)
	dst := newTestPart(t)
	topic := fmt.Sprintf("internal/forward-pub-err/%d", time.Now().UnixNano())

	srcPub, err := src.NewPublisher(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher on src: %v", err)
	}
	defer srcPub.Close()

	b, err := New(src, dst, Options{Topics: []string{topic}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	b.Start()

	// Close the bridge's dst publisher so Write returns an error (ErrClosed).
	b.pubs[0].Close()

	// Publish a sample — the forward goroutine receives it, tries pub.Write,
	// gets ErrClosed, and exits via the error-return path.
	_ = srcPub.Write([]byte("trigger"))
	time.Sleep(30 * time.Millisecond)

	if err := b.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}
