// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package testutil provides test-harness helpers for DDS participants.
// It is intended for use in _test.go files only; production code must not
// import this package.
package testutil

import (
	"fmt"
	"sync"
	"testing"
	"time"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/mock"
)

// NewParticipant creates a mock DDS participant on domain and registers
// t.Cleanup to close it when the test ends.
func NewParticipant(t testing.TB, domain dds.Domain) dds.Participant {
	t.Helper()
	p, err := mock.New(domain)
	if err != nil {
		t.Fatalf("testutil.NewParticipant: %v", err)
	}
	t.Cleanup(func() { _ = p.Close() })
	return p
}

// AssertSample blocks until a sample arrives on sub and asserts that its
// payload equals want. The test is fatal if no sample arrives within timeout
// or if the payload does not match.
func AssertSample(t testing.TB, sub dds.Subscriber, want []byte, timeout time.Duration) {
	t.Helper()
	select {
	case got := <-sub.C():
		if string(got.Payload) != string(want) {
			t.Fatalf("AssertSample: payload mismatch\n  got  %q\n  want %q", got.Payload, want)
		}
	case <-time.After(timeout):
		t.Fatalf("AssertSample: timeout after %v waiting for %q", timeout, want)
	}
}

// AssertNoSample asserts that no sample arrives on sub within timeout.
func AssertNoSample(t testing.TB, sub dds.Subscriber, timeout time.Duration) {
	t.Helper()
	select {
	case got := <-sub.C():
		t.Fatalf("AssertNoSample: unexpected sample: topic=%q payload=%q", got.Topic, got.Payload)
	case <-time.After(timeout):
		// expected: silence
	}
}

// BurstPublish writes n copies of payload to pub. It returns the first error
// encountered, or nil if all writes succeed.
func BurstPublish(pub dds.Publisher, n int, payload []byte) error {
	for i := 0; i < n; i++ {
		if err := pub.Write(payload); err != nil {
			return fmt.Errorf("BurstPublish: write %d/%d: %w", i+1, n, err)
		}
	}
	return nil
}

// PeriodicPublish publishes payload at the given interval until stop is
// closed. It returns the first write error, or nil.
func PeriodicPublish(pub dds.Publisher, payload []byte, interval time.Duration, stop <-chan struct{}) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return nil
		case <-ticker.C:
			if err := pub.Write(payload); err != nil {
				return fmt.Errorf("PeriodicPublish: %w", err)
			}
		}
	}
}

// TopicRecorder records every sample delivered to a subscriber. Call Start to
// begin, Stop to end, and Samples to retrieve what was captured.
type TopicRecorder struct {
	sub     dds.Subscriber
	mu      sync.Mutex
	samples []dds.Sample
	done    chan struct{}
	once    sync.Once
}

// NewTopicRecorder creates a recorder bound to sub. Call Start to begin.
func NewTopicRecorder(sub dds.Subscriber) *TopicRecorder {
	return &TopicRecorder{sub: sub, done: make(chan struct{})}
}

// Start launches the background recording goroutine. Returns r for chaining.
func (r *TopicRecorder) Start() *TopicRecorder {
	go r.loop()
	return r
}

func (r *TopicRecorder) loop() {
	for {
		select {
		case s, ok := <-r.sub.C():
			if !ok {
				return
			}
			r.mu.Lock()
			r.samples = append(r.samples, s)
			r.mu.Unlock()
		case <-r.done:
			return
		}
	}
}

// Stop ends recording. Safe to call multiple times.
func (r *TopicRecorder) Stop() {
	r.once.Do(func() { close(r.done) })
}

// Samples returns a copy of all samples recorded so far.
func (r *TopicRecorder) Samples() []dds.Sample {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := make([]dds.Sample, len(r.samples))
	copy(cp, r.samples)
	return cp
}

// Count returns the number of samples recorded so far.
func (r *TopicRecorder) Count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.samples)
}

// WaitFor blocks until at least n samples have been recorded or timeout elapses.
// Returns true if the target count was reached.
func (r *TopicRecorder) WaitFor(n int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if r.Count() >= n {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return r.Count() >= n
}

// DrainSamples returns and clears all samples recorded so far.
func (r *TopicRecorder) DrainSamples() []dds.Sample {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := r.samples
	r.samples = nil
	return s
}
