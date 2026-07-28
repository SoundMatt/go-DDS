// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// QoS Enforcement — Active Policy (Milestone 14, ROADMAP.md), mock backend.
//
// Mirrors the rtps package's enforcement of the same four QoS policies
// (rtps/qos_enforce.go), adapted to the mock's single-process, wire-free
// broker: there is no SEDP round trip here, so "matching" is simply looking
// up shared, topic-keyed state the first time it is needed.
package mock

import (
	"bytes"
	"sync"
	"time"

	dds "github.com/SoundMatt/go-DDS"
)

// ── Partition ─────────────────────────────────────────────────────────────────

// partitionsMatch implements the DDS PARTITION QoS matching rule: two
// endpoints match iff their partition name sets intersect. An empty set is
// equivalent to the single default partition "".
func partitionsMatch(a, b []string) bool {
	an, bn := a, b
	if len(an) == 0 {
		an = []string{""}
	}
	if len(bn) == 0 {
		bn = []string{""}
	}
	for _, x := range an {
		for _, y := range bn {
			if x == y {
				return true
			}
		}
	}
	return false
}

// ── Ownership ─────────────────────────────────────────────────────────────────

// guidLess provides a total, deterministic order over GUIDs so ownership-
// strength ties are broken the same way for every observer.
func guidLess(a, b dds.GUID) bool {
	return bytes.Compare(a[:], b[:]) < 0
}

// mockOwnershipState is the broker's per-topic Ownership QoS arbitration
// state — the mock-package analogue of rtps's ownershipState.
type mockOwnershipState struct {
	mu        sync.Mutex
	strengths map[dds.GUID]int32
	active    dds.GUID
}

func (o *mockOwnershipState) register(g dds.GUID, strength int32) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.strengths == nil {
		o.strengths = make(map[dds.GUID]int32)
	}
	o.strengths[g] = strength
	o.recomputeLocked()
}

func (o *mockOwnershipState) unregister(g dds.GUID) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.strengths == nil {
		return
	}
	delete(o.strengths, g)
	o.recomputeLocked()
}

func (o *mockOwnershipState) recomputeLocked() {
	var best dds.GUID
	var bestStrength int32
	have := false
	for g, s := range o.strengths {
		if !have || s > bestStrength || (s == bestStrength && guidLess(g, best)) {
			best, bestStrength, have = g, s, true
		}
	}
	if have {
		o.active = best
	} else {
		o.active = dds.GUID{}
	}
}

func (o *mockOwnershipState) allows(g dds.GUID) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	if len(o.strengths) == 0 {
		return true
	}
	return g == o.active
}

// ownershipStateFor returns (creating on first access) the ownership
// arbitration state for topic.
func (b *broker) ownershipStateFor(topic string) *mockOwnershipState {
	if v, ok := b.ownership.Load(topic); ok {
		if os, ok2 := v.(*mockOwnershipState); ok2 {
			return os
		}
	}
	os := &mockOwnershipState{}
	actual, _ := b.ownership.LoadOrStore(topic, os)
	if os2, ok := actual.(*mockOwnershipState); ok {
		return os2
	}
	return os
}

// ownershipAllows reports whether a sample from writer g should be delivered
// on topic. Always true until an ExclusiveOwnership writer has registered.
func (b *broker) ownershipAllows(topic string, g dds.GUID) bool {
	v, ok := b.ownership.Load(topic)
	if !ok {
		return true
	}
	os, ok2 := v.(*mockOwnershipState)
	if !ok2 {
		return true
	}
	return os.allows(g)
}

// ── Time-Based Filter ─────────────────────────────────────────────────────────

// timeBasedFilterState implements the MinSeparation QoS for a single
// subscription, tracked independently per writer GUID so one fast writer
// cannot starve delivery from another matched writer.
type timeBasedFilterState struct {
	mu   sync.Mutex
	last map[dds.GUID]time.Time
}

func (t *timeBasedFilterState) passes(writer dds.GUID, now time.Time, minSep time.Duration) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.last == nil {
		t.last = make(map[dds.GUID]time.Time)
	}
	if last, ok := t.last[writer]; ok && now.Sub(last) < minSep {
		return false
	}
	t.last[writer] = now
	return true
}

// ── Liveliness ────────────────────────────────────────────────────────────────

// livelinessPollInterval is how often a topic's liveliness state is scanned
// for expired writer leases.
const livelinessPollInterval = 100 * time.Millisecond

// minLivelinessAssertPeriod floors a publisher's assertion ticker so a very
// short QoS.LivelinessLeaseDuration (e.g. in tests) cannot spin a tight loop.
const minLivelinessAssertPeriod = 10 * time.Millisecond

// topicLiveliness is the broker's per-topic Liveliness QoS state: which
// writers currently offer a lease, when each was last heard from, and which
// subscribers asked to be told when a lease expires.
type topicLiveliness struct {
	mu        sync.Mutex
	lease     map[dds.GUID]time.Duration
	lastSeen  map[dds.GUID]time.Time
	fired     map[dds.GUID]bool
	callbacks map[*subscriber]func(dds.GUID)
}

// registerWriter records that writer g offers lease and starts its
// "last seen alive" clock now — giving it a full lease period before being
// declared lost.
func (t *topicLiveliness) registerWriter(g dds.GUID, lease time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.lease == nil {
		t.lease = make(map[dds.GUID]time.Duration)
		t.lastSeen = make(map[dds.GUID]time.Time)
		t.fired = make(map[dds.GUID]bool)
	}
	t.lease[g] = lease
	t.lastSeen[g] = time.Now()
	t.fired[g] = false
}

// unregisterWriter stops tracking g (writer closed).
func (t *topicLiveliness) unregisterWriter(g dds.GUID) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.lease, g)
	delete(t.lastSeen, g)
	delete(t.fired, g)
}

// touch records that writer g was just heard from (a Write or a liveliness
// assertion) and re-arms the "already fired" latch for its next silence.
func (t *topicLiveliness) touch(g dds.GUID) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.lease == nil {
		return
	}
	if _, tracked := t.lease[g]; !tracked {
		return
	}
	t.lastSeen[g] = time.Now()
	t.fired[g] = false
}

// addCallback registers sub's LivelinessLostCallback, keyed by sub's own
// pointer identity so removeCallback can undo it on Unsubscribe/Close.
func (t *topicLiveliness) addCallback(sub *subscriber, cb func(dds.GUID)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.callbacks == nil {
		t.callbacks = make(map[*subscriber]func(dds.GUID))
	}
	t.callbacks[sub] = cb
}

// removeCallback undoes addCallback.
func (t *topicLiveliness) removeCallback(sub *subscriber) {
	t.mu.Lock()
	delete(t.callbacks, sub)
	t.mu.Unlock()
}

// check scans every tracked writer lease and invokes every registered
// callback (edge-triggered: once per silence episode) for any writer whose
// lease has expired since it was last heard from.
func (t *topicLiveliness) check() {
	t.mu.Lock()
	var lost []dds.GUID
	now := time.Now()
	for g, lease := range t.lease {
		if t.fired[g] {
			continue
		}
		if now.Sub(t.lastSeen[g]) > lease {
			t.fired[g] = true
			lost = append(lost, g)
		}
	}
	cbs := make([]func(dds.GUID), 0, len(t.callbacks))
	for _, cb := range t.callbacks {
		cbs = append(cbs, cb)
	}
	t.mu.Unlock()
	if len(lost) == 0 {
		return
	}
	for _, g := range lost {
		for _, cb := range cbs {
			cb(g)
		}
	}
}

// livelinessStateFor returns (creating on first access) the topicLiveliness
// state for topic.
func (b *broker) livelinessStateFor(topic string) *topicLiveliness {
	if v, ok := b.topicLiveliness.Load(topic); ok {
		if tl, ok2 := v.(*topicLiveliness); ok2 {
			return tl
		}
	}
	tl := &topicLiveliness{}
	actual, _ := b.topicLiveliness.LoadOrStore(topic, tl)
	if tl2, ok := actual.(*topicLiveliness); ok {
		return tl2
	}
	return tl
}

// startLivelinessMonitor lazily starts (once per broker) a background
// goroutine that periodically scans every topic's liveliness state for
// expired leases. Safe to call repeatedly; only the first call has any
// effect. The goroutine runs for the process lifetime, matching the existing
// mock broker's lack of an explicit shutdown path (globalBroker is a
// package-level var with no Close).
func (b *broker) startLivelinessMonitor() {
	b.livelinessMonitorOnce.Do(func() {
		go func() {
			t := time.NewTicker(livelinessPollInterval)
			defer t.Stop()
			for range t.C {
				b.topicLiveliness.Range(func(_, v any) bool {
					if tl, ok := v.(*topicLiveliness); ok {
						tl.check()
					}
					return true
				})
			}
		}()
	})
}

// livelinessLoop periodically touches this publisher's liveliness state at
// roughly a third of its configured lease, for as long as the publisher is
// open. Runs even though the mock is in-process — a writer that stops
// calling Write should still be declared lost like a real network writer
// that stops asserting liveliness.
func (pub *publisher) livelinessLoop(done <-chan struct{}) {
	period := pub.qos.LivelinessLeaseDuration / 3
	if period < minLivelinessAssertPeriod {
		period = minLivelinessAssertPeriod
	}
	t := time.NewTicker(period)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			pub.broker.livelinessStateFor(pub.topic).touch(pub.guid)
		case <-done:
			return
		}
	}
}
