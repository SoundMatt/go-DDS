// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package domain_test

import (
	"fmt"
	"testing"
	"time"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/bridge/domain"
	"github.com/SoundMatt/go-DDS/mock"
)

func newPart(t *testing.T) dds.Participant {
	t.Helper()
	p, err := mock.New(0)
	if err != nil {
		t.Fatalf("mock.New: %v", err)
	}
	t.Cleanup(func() { _ = p.Close() })
	return p
}

func uniqueTopic(prefix string) string {
	return fmt.Sprintf("%s/%d", prefix, time.Now().UnixNano())
}

func TestBridge_ForwardsSample(t *testing.T) {
	src := newPart(t)
	dst := newPart(t)
	topic := uniqueTopic("bridge/fwd")

	// Subscribe and publish are registered before the bridge so their defers
	// run after b.Close() stops the forward goroutine (Go defers are LIFO).
	sub, err := dst.NewSubscriber(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber on dst: %v", err)
	}
	defer sub.Close()

	pub, err := src.NewPublisher(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher on src: %v", err)
	}
	defer pub.Close()

	b, err := domain.New(src, dst, domain.Options{Topics: []string{topic}})
	if err != nil {
		t.Fatalf("domain.New: %v", err)
	}
	b.Start()
	defer b.Close() // registered last → runs first; goroutines exit before sub/pub defers

	if err := pub.Write([]byte("bridged")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	select {
	case s := <-sub.C():
		if string(s.Payload) != "bridged" {
			t.Errorf("payload: got %q, want bridged", s.Payload)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout: sample not forwarded across domain bridge")
	}
}

func TestBridge_MultipleTopics(t *testing.T) {
	src := newPart(t)
	dst := newPart(t)
	topicA := uniqueTopic("bridge/multi/a")
	topicB := uniqueTopic("bridge/multi/b")

	subA, _ := dst.NewSubscriber(topicA, dds.DefaultQoS)
	subB, _ := dst.NewSubscriber(topicB, dds.DefaultQoS)
	defer subA.Close()
	defer subB.Close()

	pubA, _ := src.NewPublisher(topicA, dds.DefaultQoS)
	pubB, _ := src.NewPublisher(topicB, dds.DefaultQoS)
	defer pubA.Close()
	defer pubB.Close()

	b, err := domain.New(src, dst, domain.Options{Topics: []string{topicA, topicB}})
	if err != nil {
		t.Fatalf("domain.New: %v", err)
	}
	b.Start()
	defer b.Close()

	_ = pubA.Write([]byte("from-a"))
	_ = pubB.Write([]byte("from-b"))

	for _, tc := range []struct {
		sub  dds.Subscriber
		want string
	}{
		{subA, "from-a"},
		{subB, "from-b"},
	} {
		select {
		case s := <-tc.sub.C():
			if string(s.Payload) != tc.want {
				t.Errorf("payload: got %q, want %q", s.Payload, tc.want)
			}
		case <-time.After(time.Second):
			t.Fatalf("timeout waiting for %q", tc.want)
		}
	}
}

func TestBridge_EmptyTopics(t *testing.T) {
	src := newPart(t)
	dst := newPart(t)
	b, err := domain.New(src, dst, domain.Options{})
	if err != nil {
		t.Fatalf("domain.New with no topics: %v", err)
	}
	b.Start()
	if err := b.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

func TestBridge_Close_Idempotent(t *testing.T) {
	src := newPart(t)
	dst := newPart(t)
	topic := uniqueTopic("bridge/idempotent")
	b, err := domain.New(src, dst, domain.Options{Topics: []string{topic}})
	if err != nil {
		t.Fatalf("domain.New: %v", err)
	}
	b.Start()
	_ = b.Close()
	if err := b.Close(); err != nil {
		t.Errorf("second Close should not error: %v", err)
	}
}

func TestBridge_New_ClosedSrc_Error(t *testing.T) {
	src := newPart(t)
	dst := newPart(t)
	src.Close()

	topic := uniqueTopic("bridge/closedsrc")
	_, err := domain.New(src, dst, domain.Options{Topics: []string{topic}})
	if err == nil {
		t.Error("expected error when src is closed")
	}
}

func TestBridge_New_ClosedDst_Error(t *testing.T) {
	src := newPart(t)
	dst := newPart(t)
	dst.Close()

	topic := uniqueTopic("bridge/closeddst")
	_, err := domain.New(src, dst, domain.Options{Topics: []string{topic}})
	if err == nil {
		t.Error("expected error when dst is closed")
	}
}

// TestBridge_DstClosed_ForwardExits exercises the forward goroutine's
// pub.Write error (or recover'd panic) path by closing dst after the bridge
// starts. The forward goroutine must exit cleanly whether mock returns
// ErrClosed or panics on a closed channel.
func TestBridge_DstClosed_ForwardExits(t *testing.T) {
	src := newPart(t)
	dst := newPart(t)
	topic := uniqueTopic("bridge/dstclosed")

	pub, err := src.NewPublisher(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher on src: %v", err)
	}
	defer pub.Close()

	b, err := domain.New(src, dst, domain.Options{Topics: []string{topic}})
	if err != nil {
		t.Fatalf("domain.New: %v", err)
	}
	b.Start()
	defer b.Close() // registered last → runs first; forward goroutine should already be done

	// Close dst so the bridge's dst publisher Write will fail or panic.
	dst.Close()
	_ = pub.Write([]byte("trigger"))
	// Give the forward goroutine time to read the sample and attempt the
	// Write on the closed dst publisher before the test ends.
	time.Sleep(30 * time.Millisecond)
}

// TestBridge_SrcSubscriberClosed exercises the forward goroutine's !ok branch.
func TestBridge_SrcSubscriberClosed(t *testing.T) {
	src := newPart(t)
	dst := newPart(t)
	topic := uniqueTopic("bridge/srcclosed")

	b, err := domain.New(src, dst, domain.Options{Topics: []string{topic}})
	if err != nil {
		t.Fatalf("domain.New: %v", err)
	}
	b.Start()

	// Closing src closes all subscribers on src — the forward goroutine exits.
	src.Close()
	time.Sleep(30 * time.Millisecond)

	if err := b.Close(); err != nil {
		t.Errorf("Close after src closed: %v", err)
	}
}
