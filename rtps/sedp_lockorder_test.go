// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Regression test for go-DDS#130: a lock-order inversion between
// participant.mu and sedpService.mu that could deadlock two goroutines,
// one on each of the two call paths that need both locks:
//
//   - participant.newPublisherLocked / newSubscriberLocked hold p.mu across
//     the call into sedpService.registerWriter / registerReader, which
//     acquires sedp.mu — i.e. p.mu, then sedp.mu.
//   - sedpService.onRemoteWriter used to hold sedp.mu across the call back
//     into participant.readerByEID / addWriterLocator, which acquire p.mu —
//     i.e. sedp.mu, then p.mu. The reverse order.
//
// Two goroutines, one on each path, each holding its first lock and blocked
// acquiring the other's, is a classic AB-BA deadlock — and it hung a real CI
// run for the full 10-minute test timeout (see the issue for the goroutine
// dump). The fix (see sedpService.onRemoteWriter) makes sedp.mu and p.mu
// never held simultaneously in that function at all, which makes the cycle
// structurally impossible rather than merely less likely — this test stress-
// drives both paths concurrently, under -race, and fails fast on a timeout
// instead of hanging for the CI run's full test deadline if the invariant is
// ever reintroduced.
package rtps

import (
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	dds "github.com/SoundMatt/go-DDS"
)

func TestSEDP_OnRemoteWriter_NoLockOrderDeadlockWithParticipant(t *testing.T) {
	p := testPart(t)

	const sharedTopic = "lockorder/shared"

	// Pre-register several local readers on sharedTopic so every
	// onRemoteWriter call below actually matches at least one of them and
	// takes the participant-callback path (p.readerByEID / p.addWriterLocator)
	// that used to be reached while sedp.mu was still held.
	const preRegisteredReaders = 8
	for i := 0; i < preRegisteredReaders; i++ {
		sub, err := p.NewSubscriber(sharedTopic, dds.DefaultQoS)
		if err != nil {
			t.Fatalf("NewSubscriber(pre-register %d): %v", i, err)
		}
		defer sub.Close()
	}

	loc := Locator{Kind: LocatorKindUDPv4, Port: 7777}
	copy(loc.Address[12:], net.ParseIP("127.0.0.1").To4())

	const goroutinesPerSide = 16
	const iterationsPerGoroutine = 200

	start := make(chan struct{})
	var wg sync.WaitGroup

	// Side A: repeatedly exercises the p.mu -> sedp.mu order via the real
	// public API (NewPublisher/NewSubscriber -> sedp.registerWriter/
	// registerReader), exactly as newPublisherLocked/newSubscriberLocked do.
	for g := 0; g < goroutinesPerSide; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			<-start
			for i := 0; i < iterationsPerGoroutine; i++ {
				topic := fmt.Sprintf("lockorder/new/%d/%d", g, i)
				if g%2 == 0 {
					sub, err := p.NewSubscriber(topic, dds.DefaultQoS)
					if err != nil {
						t.Errorf("NewSubscriber: %v", err)
						return
					}
					sub.Close()
				} else {
					pub, err := p.NewPublisher(topic, dds.DefaultQoS)
					if err != nil {
						t.Errorf("NewPublisher: %v", err)
						return
					}
					pub.Close()
				}
			}
		}(g)
	}

	// Side B: repeatedly exercises sedpService.onRemoteWriter directly — the
	// path that used to hold sedp.mu across the call back into the
	// participant (sedp.mu -> p.mu, the reverse order). Every call matches
	// sharedTopic's pre-registered local readers, so it always takes the
	// p.readerByEID / p.addWriterLocator branch.
	for g := 0; g < goroutinesPerSide; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			<-start
			for i := 0; i < iterationsPerGoroutine; i++ {
				info := &endpointInfo{
					guid:      GUID{Prefix: newGuidPrefix(), Entity: entityIdForWriter(uint32(i + 1))},
					topicName: sharedTopic,
					isWriter:  true,
				}
				p.sedp.onRemoteWriter(info, loc)
			}
		}(g)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	close(start) // release every goroutine at once to maximize contention

	select {
	case <-done:
		// All goroutines completed: no deadlock.
	case <-time.After(20 * time.Second):
		t.Fatal("deadlock: participant.mu / sedpService.mu lock-order inversion " +
			"(go-DDS#130) reproduced — goroutines did not complete within the timeout")
	}
}

// TestSEDP_OnRemoteWriter_MatchesLocalReader is a plain functional regression
// test for the onRemoteWriter refactor in the go-DDS#130 fix: releasing
// sedp.mu before calling back into the participant must not change the
// observable outcome (the matched reader still learns the remote writer's
// GUID, and the participant still records the writer's data locator).
func TestSEDP_OnRemoteWriter_MatchesLocalReader(t *testing.T) {
	p := testPart(t)

	const topic = "lockorder/functional"
	sub, err := p.NewSubscriber(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()

	r, ok := sub.(*rtpsReader)
	if !ok {
		t.Fatalf("subscriber is %T, want *rtpsReader", sub)
	}

	writerGUID := GUID{Prefix: newGuidPrefix(), Entity: entityIdForWriter(1)}
	loc := Locator{Kind: LocatorKindUDPv4, Port: 7778}
	copy(loc.Address[12:], net.ParseIP("127.0.0.1").To4())

	p.sedp.onRemoteWriter(&endpointInfo{
		guid:      writerGUID,
		topicName: topic,
		isWriter:  true,
	}, loc)

	if !r.acceptsSource(writerGUID) {
		t.Error("matched reader did not learn the remote writer's GUID")
	}
	p.mu.Lock()
	gotLoc, ok := p.writerLocators[writerGUID]
	p.mu.Unlock()
	if !ok || gotLoc != loc {
		t.Errorf("participant.writerLocators[%v] = %v, %v; want %v, true", writerGUID, gotLoc, ok, loc)
	}
}
