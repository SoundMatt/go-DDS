//go:build cyclone

// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package cyclone_test

import (
	"fmt"
	"testing"
	"time"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/cyclone"
)

// newBenchParticipant creates a participant for benchmarking and skips if
// CycloneDDS is not available on this host.
func newBenchParticipant(b *testing.B) dds.Participant {
	b.Helper()
	p, err := cyclone.New(testDomain)
	if err != nil {
		b.Skipf("CycloneDDS unavailable (%v) — skipping", err)
	}
	b.Cleanup(func() { p.Close() })
	return p
}

// waitDiscovery sleeps long enough for CycloneDDS endpoint discovery (SPDP/SEDP)
// to complete before the benchmark loop starts. Without this, early writes may
// be delivered before the remote reader is matched and samples are silently lost.
const discoveryDelay = 150 * time.Millisecond

// BenchmarkCyclone_RoundTrip measures end-to-end publish→receive over real DDS.
// One iteration = one Write (via CycloneDDS dds_write) + one channel receive
// (polled at 5 ms intervals by the subscriber goroutine).
func BenchmarkCyclone_RoundTrip(b *testing.B) {
	for _, ps := range cycloneBenchSizes {
		ps := ps
		b.Run(ps.name, func(b *testing.B) {
			p := newBenchParticipant(b)
			topic := "bench/cyclone/roundtrip/" + ps.name

			sub, err := p.NewSubscriber(topic, dds.DefaultQoS)
			if err != nil {
				b.Fatalf("NewSubscriber: %v", err)
			}
			defer sub.Close()

			pub, err := p.NewPublisher(topic, dds.DefaultQoS)
			if err != nil {
				b.Fatalf("NewPublisher: %v", err)
			}
			defer pub.Close()

			time.Sleep(discoveryDelay)

			payload := make([]byte, ps.size)
			b.SetBytes(int64(ps.size))
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				if err := pub.Write(payload); err != nil {
					b.Fatalf("Write: %v", err)
				}
				<-sub.C()
			}
		})
	}
}

// BenchmarkCyclone_PublishOnly measures the cost of dds_write alone (no subscriber
// reading). This isolates the CycloneDDS serialisation + transport layer overhead.
func BenchmarkCyclone_PublishOnly(b *testing.B) {
	for _, ps := range cycloneBenchSizes {
		ps := ps
		b.Run(ps.name, func(b *testing.B) {
			p := newBenchParticipant(b)

			pub, err := p.NewPublisher("bench/cyclone/pubonly/"+ps.name, dds.DefaultQoS)
			if err != nil {
				b.Fatalf("NewPublisher: %v", err)
			}
			defer pub.Close()

			time.Sleep(discoveryDelay)

			payload := make([]byte, ps.size)
			b.SetBytes(int64(ps.size))
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				pub.Write(payload)
			}
		})
	}
}

// BenchmarkCyclone_NewWithOptions measures participant creation with
// a custom PollInterval — exercises the Options code path.
func BenchmarkCyclone_NewWithOptions(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		p, err := cyclone.NewWithOptions(testDomain, cyclone.Options{
			PollInterval: 1 * time.Millisecond,
		})
		if err != nil {
			b.Skipf("CycloneDDS unavailable: %v", err)
		}
		p.Close()
	}
}

// BenchmarkCyclone_FanOut measures delivery to N CycloneDDS subscribers.
func BenchmarkCyclone_FanOut(b *testing.B) {
	for _, n := range []int{1, 2, 4} {
		n := n
		b.Run(fmt.Sprintf("%dsubs", n), func(b *testing.B) {
			p := newBenchParticipant(b)
			topic := fmt.Sprintf("bench/cyclone/fanout/%d", n)

			subs := make([]dds.Subscriber, n)
			for i := range subs {
				var err error
				subs[i], err = p.NewSubscriber(topic, dds.DefaultQoS)
				if err != nil {
					b.Fatalf("NewSubscriber: %v", err)
				}
			}
			defer func() {
				for _, s := range subs {
					s.Close()
				}
			}()

			pub, err := p.NewPublisher(topic, dds.DefaultQoS)
			if err != nil {
				b.Fatalf("NewPublisher: %v", err)
			}
			defer pub.Close()

			time.Sleep(discoveryDelay)

			payload := []byte(`{"v":1}`)
			b.SetBytes(int64(len(payload) * n))
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				pub.Write(payload)
				for _, s := range subs {
					<-s.C()
				}
			}
		})
	}
}

// cycloneBenchSizes is kept smaller than the mock equivalent because CycloneDDS
// round-trip latency (5 ms poll + discovery) makes large-N runs expensive.
var cycloneBenchSizes = []struct {
	name string
	size int
}{
	{"64B", 64},
	{"1KB", 1024},
	{"16KB", 16 * 1024},
}
