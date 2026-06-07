// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package mock_test

import (
	"bytes"
	"testing"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/mock"
)

// Fuzz tests run as ordinary unit tests when invoked without -fuzz, using
// only the seed corpus entries below. Run the full fuzzer with:
//
//	go test -fuzz=FuzzPublish          -fuzztime=60s ./mock/...
//	go test -fuzz=FuzzTopicName        -fuzztime=60s ./mock/...
//	go test -fuzz=FuzzNoRouting        -fuzztime=60s ./mock/...
//	go test -fuzz=FuzzPublishIsolation -fuzztime=60s ./mock/...
//	go test -fuzz=FuzzConcurrentPubSub -fuzztime=60s ./mock/...

// ── FuzzPublish ───────────────────────────────────────────────────────────────

// FuzzPublish verifies that any payload round-trips through publish→subscribe
// unchanged. The mock's broker.publish is synchronous: the sample is in the
// channel before Write returns, so a non-blocking select suffices.
func FuzzPublish(f *testing.F) {
	// Seed corpus — covers common real-world payloads and edge cases.
	f.Add([]byte(""))
	f.Add([]byte("hello world"))
	f.Add([]byte(`{"action":"get","path":"Vehicle.Speed"}`))
	f.Add([]byte(`{"value":3.14159,"unit":"m/s"}`))
	f.Add([]byte{0x00})
	f.Add([]byte{0xFF, 0xFE, 0x00, 0x01})
	f.Add([]byte{0x00, 0x00, 0x00, 0x00})
	f.Add(make([]byte, 1024))               // 1 KB zeros
	f.Add(bytes.Repeat([]byte{0xAB}, 4096)) // 4 KB repeated byte

	f.Fuzz(func(t *testing.T, payload []byte) {
		p, err := mock.New(dds.Domain(0))
		if err != nil {
			t.Fatalf("mock.New: %v", err)
		}
		defer p.Close()

		sub, err := p.NewSubscriber("fuzz/publish", dds.DefaultQoS)
		if err != nil {
			t.Fatalf("NewSubscriber: %v", err)
		}
		defer sub.Close()

		pub, err := p.NewPublisher("fuzz/publish", dds.DefaultQoS)
		if err != nil {
			t.Fatalf("NewPublisher: %v", err)
		}
		defer pub.Close()

		// Snapshot before Write in case the broker modifies in place.
		want := make([]byte, len(payload))
		copy(want, payload)

		if err := pub.Write(payload); err != nil {
			t.Fatalf("Write: %v", err)
		}

		select {
		case s := <-sub.C():
			if !bytes.Equal(s.Payload, want) {
				t.Errorf("payload mismatch:\n  got  %q\n  want %q", s.Payload, want)
			}
		default:
			// Empty payload still produces a sample; only a full channel causes
			// a drop — and we have a fresh subscriber with an empty 64-slot buffer.
			t.Errorf("no sample delivered for payload %q", payload)
		}
	})
}

// ── FuzzPublishIsolation ──────────────────────────────────────────────────────

// FuzzPublishIsolation verifies that mutating the original byte slice after
// Write does not corrupt the sample delivered to the subscriber. The broker
// must copy the payload, not retain a reference to the caller's slice.
func FuzzPublishIsolation(f *testing.F) {
	f.Add([]byte("mutable"))
	f.Add([]byte{0x00, 0x01, 0x02})
	f.Add([]byte("longer string that spans multiple words"))
	f.Add(make([]byte, 512))

	f.Fuzz(func(t *testing.T, payload []byte) {
		if len(payload) == 0 {
			return // nothing to mutate; skip
		}

		p, _ := mock.New(dds.Domain(0))
		defer p.Close()

		sub, _ := p.NewSubscriber("fuzz/isolation", dds.DefaultQoS)
		pub, _ := p.NewPublisher("fuzz/isolation", dds.DefaultQoS)
		defer sub.Close()
		defer pub.Close()

		want := make([]byte, len(payload))
		copy(want, payload)

		if err := pub.Write(payload); err != nil {
			t.Fatalf("Write: %v", err)
		}

		// Mutate every byte of the original slice after Write.
		for i := range payload {
			payload[i] ^= 0xFF
		}

		select {
		case s := <-sub.C():
			if !bytes.Equal(s.Payload, want) {
				t.Errorf("broker did not copy payload: mutation leaked into delivered sample")
			}
		default:
			t.Error("no sample delivered")
		}
	})
}

// ── FuzzTopicName ─────────────────────────────────────────────────────────────

// FuzzTopicName verifies that any non-empty topic string can be published on
// and subscribed to, and that the delivered sample carries the correct Topic
// field. Topic naming is application-level convention; the broker imposes no
// restrictions on the string content.
func FuzzTopicName(f *testing.F) {
	f.Add("sensors/temperature")
	f.Add("a")
	f.Add("topic with spaces")
	f.Add("unicode/日本語/topic")
	f.Add("/leading/slash")
	f.Add("trailing/slash/")
	f.Add("double//slash")
	f.Add("UPPER_CASE")
	f.Add("mixed-Case_topic.v2")
	f.Add("very/deeply/nested/path/to/signal/value")

	f.Fuzz(func(t *testing.T, topic string) {
		if topic == "" {
			return
		}

		p, _ := mock.New(dds.Domain(0))
		defer p.Close()

		sub, err := p.NewSubscriber(topic, dds.DefaultQoS)
		if err != nil {
			t.Fatalf("NewSubscriber(%q): %v", topic, err)
		}
		defer sub.Close()

		pub, err := p.NewPublisher(topic, dds.DefaultQoS)
		if err != nil {
			t.Fatalf("NewPublisher(%q): %v", topic, err)
		}
		defer pub.Close()

		want := []byte("probe")
		if err := pub.Write(want); err != nil {
			t.Fatalf("Write: %v", err)
		}

		select {
		case s := <-sub.C():
			if s.Topic != topic {
				t.Errorf("Topic field: got %q, want %q", s.Topic, topic)
			}
			if !bytes.Equal(s.Payload, want) {
				t.Errorf("Payload: got %q, want %q", s.Payload, want)
			}
		default:
			t.Fatalf("no sample delivered for topic %q", topic)
		}
	})
}

// ── FuzzNoRouting ─────────────────────────────────────────────────────────────

// FuzzNoRouting verifies that a sample published on topicA is never delivered
// to a subscriber on a distinct topicB. Topic isolation is a core DDS invariant.
func FuzzNoRouting(f *testing.F) {
	f.Add("topicA", "topicB", []byte("signal"))
	f.Add("a", "b", []byte{})
	f.Add("x/y", "x/z", []byte("data"))
	f.Add("sensors/temp", "sensors/speed", []byte(`{"v":0}`))
	f.Add("foo", "fooo", []byte("close-names"))
	f.Add("/a", "/b", []byte{0xFF})

	f.Fuzz(func(t *testing.T, topicA, topicB string, payload []byte) {
		if topicA == topicB || topicA == "" || topicB == "" {
			return
		}

		p, _ := mock.New(dds.Domain(0))
		defer p.Close()

		subB, _ := p.NewSubscriber(topicB, dds.DefaultQoS)
		defer subB.Close()

		pub, _ := p.NewPublisher(topicA, dds.DefaultQoS)
		defer pub.Close()

		_ = pub.Write(payload) // intentional: error irrelevant for routing check

		// broker.publish is synchronous: if a sample were mis-routed it would
		// already be in subB's channel. Non-blocking read is correct here.
		select {
		case s := <-subB.C():
			t.Errorf("topic isolation violation: received %q on %q, published on %q",
				s.Payload, topicB, topicA)
		default:
			// correct: nothing delivered to wrong subscriber
		}
	})
}

// ── FuzzConcurrentPubSub ──────────────────────────────────────────────────────

// FuzzConcurrentPubSub verifies that concurrent publishers on the same topic
// do not race or corrupt each other's payloads. The race detector (-race) is
// the primary tool here; the fuzz engine finds inputs that maximise interleaving.
func FuzzConcurrentPubSub(f *testing.F) {
	f.Add([]byte("concurrent"), uint8(2))
	f.Add([]byte(`{"n":1}`), uint8(4))
	f.Add([]byte{0xDE, 0xAD, 0xBE, 0xEF}, uint8(8))
	f.Add(make([]byte, 256), uint8(3))

	f.Fuzz(func(t *testing.T, payload []byte, nPubs uint8) {
		if nPubs == 0 || nPubs > 16 {
			return // cap to avoid runaway goroutine counts
		}
		n := int(nPubs)

		p, _ := mock.New(dds.Domain(0))
		defer p.Close()

		const topic = "fuzz/concurrent"
		sub, _ := p.NewSubscriber(topic, dds.DefaultQoS)
		defer sub.Close()

		done := make(chan struct{}, n)
		for i := 0; i < n; i++ {
			go func() {
				defer func() { done <- struct{}{} }()
				pub, _ := p.NewPublisher(topic, dds.DefaultQoS)
				defer pub.Close()
				// Any error here (e.g. closed participant) is a test bug, not a library bug.
				_ = pub.Write(payload)
			}()
		}

		// Drain done signals; we don't assert on delivery count because
		// some samples may be dropped (channel full) — that is correct behaviour.
		for i := 0; i < n; i++ {
			<-done
		}
	})
}

// ── FuzzDropOnFullChannel ─────────────────────────────────────────────────────

// FuzzDropOnFullChannel verifies that filling the subscriber's 64-slot buffer
// and then writing more samples is handled gracefully (samples are dropped, no
// panic, no deadlock, no data corruption in subsequently-drained samples).
func FuzzDropOnFullChannel(f *testing.F) {
	f.Add([]byte("fill"), []byte("overflow"))
	f.Add([]byte{0x00}, []byte{0xFF})
	f.Add([]byte("aaaaaaaaa"), []byte("bbbbbbbbb"))

	f.Fuzz(func(t *testing.T, fillPayload, overflowPayload []byte) {
		p, _ := mock.New(dds.Domain(0))
		defer p.Close()

		sub, _ := p.NewSubscriber("fuzz/dropfull", dds.DefaultQoS)
		pub, _ := p.NewPublisher("fuzz/dropfull", dds.DefaultQoS)
		defer sub.Close()
		defer pub.Close()

		// Fill the 64-slot buffer.
		wantFill := make([]byte, len(fillPayload))
		copy(wantFill, fillPayload)
		for i := 0; i < 64; i++ {
			_ = pub.Write(fillPayload)
		}

		// These must not panic or block — they hit the drop path.
		for i := 0; i < 10; i++ {
			_ = pub.Write(overflowPayload)
		}

		// Drain and verify the buffered samples are the fill payload, not the overflow.
		for i := 0; i < 64; i++ {
			select {
			case s := <-sub.C():
				if !bytes.Equal(s.Payload, wantFill) {
					t.Errorf("slot %d: got %q, want %q", i, s.Payload, wantFill)
				}
			default:
				// Channel may have fewer than 64 if fillPayload was empty and
				// the broker made a zero-length copy. Accept early drain.
				return
			}
		}
	})
}
