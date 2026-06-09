// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package mock_test

import (
	"fmt"
	"testing"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/mock"
)

// payloadSizes drives all size-parameterised benchmarks.
var payloadSizes = []struct {
	name string
	size int
}{
	{"1B", 1},
	{"64B", 64},
	{"1KB", 1024},
	{"16KB", 16 * 1024},
	{"64KB", 64 * 1024},
}

// ── Round-trip (publish + receive) ───────────────────────────────────────────

// BenchmarkPublish_RoundTrip measures end-to-end publish→receive latency
// at various payload sizes. One iteration = one Write + one channel receive.
func BenchmarkPublish_RoundTrip(b *testing.B) {
	for _, ps := range payloadSizes {
		b.Run(ps.name, func(b *testing.B) {
			p, err := mock.New(dds.Domain(0))
			if err != nil {
				b.Fatalf("New: %v", err)
			}
			defer p.Close()
			topic := "bench/roundtrip/" + ps.name
			sub, err := p.NewSubscriber(topic, dds.DefaultQoS)
			if err != nil {
				b.Fatalf("NewSubscriber: %v", err)
			}
			pub, err := p.NewPublisher(topic, dds.DefaultQoS)
			if err != nil {
				b.Fatalf("NewPublisher: %v", err)
			}
			defer sub.Close()
			defer pub.Close()

			payload := make([]byte, ps.size)
			b.SetBytes(int64(ps.size))
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				if err := pub.Write(payload); err != nil {
					b.Fatal(err)
				}
				<-sub.C()
			}
		})
	}
}

// ── Fire-and-forget (publish only, subscriber not reading) ───────────────────

// BenchmarkPublish_FireAndForget measures raw publish throughput when the
// subscriber channel is drained by nobody. Samples are dropped once the
// 64-slot buffer fills; this is deliberate — it isolates the publish-path
// cost from the receive-path cost.
func BenchmarkPublish_FireAndForget(b *testing.B) {
	for _, ps := range payloadSizes {
		b.Run(ps.name, func(b *testing.B) {
			p, err := mock.New(dds.Domain(0))
			if err != nil {
				b.Fatalf("New: %v", err)
			}
			defer p.Close()
			// Subscriber registered so broker iterates the subscription list
			// (same code path as production).
			topic := "bench/ff/" + ps.name
			sub, err := p.NewSubscriber(topic, dds.DefaultQoS)
			if err != nil {
				b.Fatalf("NewSubscriber: %v", err)
			}
			pub, err := p.NewPublisher(topic, dds.DefaultQoS)
			if err != nil {
				b.Fatalf("NewPublisher: %v", err)
			}
			defer sub.Close()
			defer pub.Close()

			payload := make([]byte, ps.size)
			b.SetBytes(int64(ps.size))
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				_ = pub.Write(payload)
			}
		})
	}
}

// ── Fan-out (one publisher, N subscribers) ───────────────────────────────────

// BenchmarkPublish_FanOut measures the cost of delivering one sample to N
// concurrent subscribers. b.SetBytes reflects total bytes delivered per op.
func BenchmarkPublish_FanOut(b *testing.B) {
	for _, n := range []int{1, 2, 4, 8, 16} {
		b.Run(fmt.Sprintf("%dsubs", n), func(b *testing.B) {
			p, err := mock.New(dds.Domain(0))
			if err != nil {
				b.Fatalf("New: %v", err)
			}
			defer p.Close()

			topic := fmt.Sprintf("bench/fanout/%d", n)
			subs := make([]dds.Subscriber, n)
			for i := range subs {
				var subErr error
				subs[i], subErr = p.NewSubscriber(topic, dds.DefaultQoS)
				if subErr != nil {
					b.Fatalf("NewSubscriber: %v", subErr)
				}
			}
			defer func() {
				for _, s := range subs {
					_ = s.Close()
				}
			}()
			pub, err := p.NewPublisher(topic, dds.DefaultQoS)
			if err != nil {
				b.Fatalf("NewPublisher: %v", err)
			}
			defer pub.Close()

			payload := []byte(`{"value":42}`)
			b.SetBytes(int64(len(payload) * n))
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				if err := pub.Write(payload); err != nil {
					b.Fatal(err)
				}
				for _, s := range subs {
					<-s.C()
				}
			}
		})
	}
}

// ── Parallel publishers ───────────────────────────────────────────────────────

// BenchmarkPublish_Parallel runs b.RunParallel with GOMAXPROCS goroutines,
// each with its own publisher, writing to a shared topic. Measures concurrent
// publish throughput and exercises the broker's RWMutex read path.
func BenchmarkPublish_Parallel(b *testing.B) {
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		b.Fatalf("New: %v", err)
	}
	defer p.Close()

	payload := []byte(`{"sensor":"parallel","value":1.0}`)
	b.SetBytes(int64(len(payload)))
	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		pub, err := p.NewPublisher("bench/parallel", dds.DefaultQoS)
		if err != nil {
			b.Fatalf("NewPublisher: %v", err)
		}
		defer pub.Close()
		for pb.Next() {
			_ = pub.Write(payload)
		}
	})
}

// BenchmarkSubscribe_Parallel exercises concurrent subscriber creation and
// receive under parallel writes — stresses the broker's RWMutex write path.
func BenchmarkSubscribe_Parallel(b *testing.B) {
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		b.Fatalf("New: %v", err)
	}
	defer p.Close()

	payload := []byte("ping")
	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			sub, err := p.NewSubscriber("bench/sub-parallel", dds.DefaultQoS)
			if err != nil {
				b.Fatalf("NewSubscriber: %v", err)
			}
			pub, err := p.NewPublisher("bench/sub-parallel", dds.DefaultQoS)
			if err != nil {
				b.Fatalf("NewPublisher: %v", err)
			}
			if err := pub.Write(payload); err != nil {
				b.Fatal(err)
			}
			<-sub.C()
			pub.Close()
			sub.Close()
		}
	})
}

// ── Object creation overhead ──────────────────────────────────────────────────

// BenchmarkNewParticipant measures participant creation + close.
func BenchmarkNewParticipant(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		p, err := mock.New(dds.Domain(0))
		if err != nil {
			b.Fatalf("New: %v", err)
		}
		p.Close()
	}
}

// BenchmarkNewPublisher measures publisher creation + close on a live participant.
func BenchmarkNewPublisher(b *testing.B) {
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		b.Fatalf("New: %v", err)
	}
	defer p.Close()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pub, err := p.NewPublisher(fmt.Sprintf("bench/pubcreate/%d", i), dds.DefaultQoS)
		if err != nil {
			b.Fatalf("NewPublisher: %v", err)
		}
		pub.Close()
	}
}

// BenchmarkNewSubscriber measures subscriber creation + close on a live participant.
func BenchmarkNewSubscriber(b *testing.B) {
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		b.Fatalf("New: %v", err)
	}
	defer p.Close()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sub, err := p.NewSubscriber(fmt.Sprintf("bench/subcreate/%d", i), dds.DefaultQoS)
		if err != nil {
			b.Fatalf("NewSubscriber: %v", err)
		}
		sub.Close()
	}
}

// ── Broker internals ──────────────────────────────────────────────────────────

// BenchmarkBroker_DroppedSamples measures the publish fast-path when the
// subscriber channel is full (64 slots). This covers the default: drop branch
// in broker.publish and is the common case for a slow consumer.
func BenchmarkBroker_DroppedSamples(b *testing.B) {
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		b.Fatalf("New: %v", err)
	}
	defer p.Close()

	sub, err := p.NewSubscriber("bench/drop", dds.DefaultQoS)
	if err != nil {
		b.Fatalf("NewSubscriber: %v", err)
	}
	pub, err := p.NewPublisher("bench/drop", dds.DefaultQoS)
	if err != nil {
		b.Fatalf("NewPublisher: %v", err)
	}
	defer sub.Close()
	defer pub.Close()

	payload := []byte("x")
	// Pre-fill the 64-slot buffer so every subsequent Write hits the drop path.
	for i := 0; i < 64; i++ {
		_ = pub.Write(payload)
	}

	b.SetBytes(int64(len(payload)))
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = pub.Write(payload) // buffer full → drop
	}
}

// BenchmarkBroker_SubscribeUnsubscribe measures the cost of subscribe +
// unsubscribe (broker mutex write path) under no concurrent traffic.
func BenchmarkBroker_SubscribeUnsubscribe(b *testing.B) {
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		b.Fatalf("New: %v", err)
	}
	defer p.Close()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sub, err := p.NewSubscriber("bench/subunsub", dds.DefaultQoS)
		if err != nil {
			b.Fatalf("NewSubscriber: %v", err)
		}
		sub.Close()
	}
}

// BenchmarkPublish_ManyTopics measures broker publish when many distinct topics
// exist — exercises the map lookup cost as the topic namespace grows.
func BenchmarkPublish_ManyTopics(b *testing.B) {
	const numTopics = 1000
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		b.Fatalf("New: %v", err)
	}
	defer p.Close()

	subs := make([]dds.Subscriber, numTopics)
	pubs := make([]dds.Publisher, numTopics)
	for i := 0; i < numTopics; i++ {
		t := fmt.Sprintf("bench/manytopics/%d", i)
		var subErr, pubErr error
		subs[i], subErr = p.NewSubscriber(t, dds.DefaultQoS)
		if subErr != nil {
			b.Fatalf("NewSubscriber: %v", subErr)
		}
		pubs[i], pubErr = p.NewPublisher(t, dds.DefaultQoS)
		if pubErr != nil {
			b.Fatalf("NewPublisher: %v", pubErr)
		}
	}
	defer func() {
		for i := range subs {
			_ = subs[i].Close()
			_ = pubs[i].Close()
		}
	}()

	payload := []byte("data")
	b.SetBytes(int64(len(payload)))
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		idx := i % numTopics
		if err := pubs[idx].Write(payload); err != nil {
			b.Fatal(err)
		}
		<-subs[idx].C()
	}
}
