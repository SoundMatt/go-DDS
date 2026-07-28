// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// QoS Enforcement — Active Policy (Milestone 14, ROADMAP.md).
//
// This file holds the mechanisms shared by the four actively-enforced QoS
// policies that were previously only passively modeled (Liveliness,
// Partition) or entirely absent (Ownership, Time-Based Filter):
//
//   - Partition:      partitionsMatch — the DDS partition-intersection rule,
//     applied both to same-process matches (see dispatchToReaders) and to
//     SEDP-discovered remote matches (see sedp.go).
//   - Ownership:       ownershipState — per-topic strength arbitration so
//     only the highest-OwnershipStrength ExclusiveOwnership writer's samples
//     reach subscribers.
//   - Liveliness:      the rtpsWriter liveliness-assertion loop and the
//     rtpsReader per-writer lease monitor, wired through a dedicated HEARTBEAT
//     "L" flag (see hbFlagLiveliness in message.go) so a pure liveliness ping
//     never perturbs the reliability tracker in reliable.go.
//   - Time-Based Filter: rtpsReader.passesTimeBasedFilter, the MinSeparation
//     QoS enforced per (reader, writer) pair at delivery time.
package rtps

import (
	"bytes"
	"sync"
	"time"

	dds "github.com/SoundMatt/go-DDS"
)

// ── Partition ─────────────────────────────────────────────────────────────────

// partitionsMatch implements the DDS PARTITION QoS matching rule: two
// endpoints match iff their partition name sets intersect. An empty set is
// equivalent to the single default partition "" — so two endpoints that both
// leave Partition unset still match each other, but an endpoint with a named
// partition does NOT match one using only the default partition (standard DDS
// semantics: "" is itself a partition name, not a wildcard).
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

// guidLess provides a total, deterministic order over GUIDs so that
// ownership-strength ties are broken identically by every participant that
// observes the same set of writers.
func guidLess(a, b GUID) bool {
	if a.Prefix != b.Prefix {
		return bytes.Compare(a.Prefix[:], b.Prefix[:]) < 0
	}
	return bytes.Compare(a.Entity[:], b.Entity[:]) < 0
}

// ownershipState tracks the ExclusiveOwnership writers competing for a single
// topic and arbitrates which one is "active" — the only one whose samples are
// delivered to subscribers on that topic. Shared-ownership writers never
// register here and are therefore never filtered (allows always returns true
// when no exclusive writer has registered).
type ownershipState struct {
	mu        sync.Mutex
	strengths map[GUID]int32
	active    GUID
}

// register (re)records g's OwnershipStrength and recomputes the active owner.
// Called when an ExclusiveOwnership writer is created locally or discovered
// via SEDP.
func (o *ownershipState) register(g GUID, strength int32) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.strengths == nil {
		o.strengths = make(map[GUID]int32)
	}
	o.strengths[g] = strength
	o.recomputeLocked()
}

// unregister removes g from the arbitration set (writer closed, or its remote
// participant was evicted) and recomputes the active owner so a lower-strength
// writer can take over — the "silenced until primary fails" behaviour.
func (o *ownershipState) unregister(g GUID) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.strengths == nil {
		return
	}
	delete(o.strengths, g)
	o.recomputeLocked()
}

// recomputeLocked must be called with o.mu held.
func (o *ownershipState) recomputeLocked() {
	var best GUID
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
		o.active = GUID{}
	}
}

// allows reports whether samples from writer g should be delivered: true when
// no ExclusiveOwnership writer is registered for this topic (Shared/default
// behaviour, unchanged from pre-v1.0), or when g is the current active owner.
func (o *ownershipState) allows(g GUID) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	if len(o.strengths) == 0 {
		return true
	}
	return g == o.active
}

// ownershipStateFor returns (creating on first access) the ownershipState for
// topic. Safe for concurrent use; backed by participant.ownership (sync.Map).
func (p *participant) ownershipStateFor(topic string) *ownershipState {
	if v, ok := p.ownership.Load(topic); ok {
		if os, ok2 := v.(*ownershipState); ok2 {
			return os
		}
	}
	os := &ownershipState{}
	actual, _ := p.ownership.LoadOrStore(topic, os)
	if os2, ok := actual.(*ownershipState); ok {
		return os2
	}
	return os
}

// ownershipAllows reports whether a sample from writer source should be
// delivered to readers on topic, per the Ownership QoS arbitration above. When
// no ownershipState has ever been created for topic (no exclusive writer has
// ever registered), it always allows delivery — this is the common case and
// must stay a cheap sync.Map lookup.
func (p *participant) ownershipAllows(topic string, source GUID) bool {
	v, ok := p.ownership.Load(topic)
	if !ok {
		return true
	}
	os, ok2 := v.(*ownershipState)
	if !ok2 {
		return true
	}
	return os.allows(source)
}

// ── Liveliness (writer side) ─────────────────────────────────────────────────

// minLivelinessAssertPeriod floors the writer's assertion ticker so a very
// short QoS.LivelinessLeaseDuration (e.g. in tests) cannot spin a tight loop.
const minLivelinessAssertPeriod = 10 * time.Millisecond

// livelinessLoop periodically sends a liveliness-only HEARTBEAT (the "L" flag
// set — see hbFlagLiveliness) to every matched reader, at roughly a third of
// the configured lease so ordinary network jitter does not spuriously trip a
// subscriber's LivelinessLostCallback. Runs for as long as the writer is open;
// stopped by closing done (see rtpsWriter.Close). Only started for
// AutomaticLiveliness — see participant.NewPublisher.
func (w *rtpsWriter) livelinessLoop(done <-chan struct{}) {
	period := w.qos.LivelinessLeaseDuration / 3
	if period < minLivelinessAssertPeriod {
		period = minLivelinessAssertPeriod
	}
	t := time.NewTicker(period)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			w.assertLiveliness()
		case <-done:
			return
		}
	}
}

// assertLiveliness sends a single liveliness-only HEARTBEAT to every remote
// reader currently matched to this writer's topic (and Partition — see
// matchedReaderLocators), and directly touches any same-process (local)
// matched reader's liveliness clock — local delivery never round-trips
// through a socket, so there is no wire message for a local reader to
// receive. It is independent of the reliable retransmission HEARTBEAT in
// reliable.go: a best-effort writer has no send history to advertise, so
// FirstSN/LastSN are left zero and remote readers must ignore them (enforced
// by checking Heartbeat.Liveliness before touching the reliability tracker —
// see participant.handleHeartbeat).
func (w *rtpsWriter) assertLiveliness() {
	w.mu.Lock()
	closed := w.closed
	w.mu.Unlock()
	if closed {
		return
	}
	writerGUID := GUID{Prefix: w.p.guidPrefix, Entity: w.eid}

	hb := Heartbeat{
		ReaderEntityId: EntityIdUnknown,
		WriterEntityId: w.eid,
		Count:          int32(w.livelinessSeq.Add(1)),
		Liveliness:     true,
	}
	msg := wrapInRTPSMessage(w.p.guidPrefix, marshalHeartbeat(hb))
	sock := w.sendSock()
	for _, lm := range w.p.matchedReaderLocators(w.topic, w.partitions) {
		if dst := lm.Loc.udpAddr(); dst != nil {
			w.p.sendUnicast(sock, dst, lm.Prefix, msg, true)
		}
	}

	w.p.mu.Lock()
	readers := make([]*rtpsReader, 0, len(w.p.readers))
	for _, r := range w.p.readers {
		readers = append(readers, r)
	}
	w.p.mu.Unlock()
	for _, r := range readers {
		if r.topic == w.topic && partitionsMatch(w.partitions, r.partitions) {
			r.touchLiveliness(writerGUID)
		}
	}
}

// ── Liveliness (reader side) ─────────────────────────────────────────────────

// livelinessPollInterval is how often a subscriber's lease monitor checks its
// matched writers for an expired lease. Independent of any single writer's
// lease so one reader can watch writers with different lease durations.
const livelinessPollInterval = 100 * time.Millisecond

// registerWriterLease records that writer g offers QoS.LivelinessLeaseDuration
// lease and starts its "last seen alive" clock at the moment of matching —
// giving a freshly-matched writer a full lease period to send its first
// assertion before being declared lost. Called both for SEDP-discovered remote
// writers and for locally co-located writers on the same topic+partition (see
// participant.NewPublisher / NewSubscriber).
func (r *rtpsReader) registerWriterLease(g GUID, lease time.Duration) {
	r.livelinessMu.Lock()
	defer r.livelinessMu.Unlock()
	if r.writerLease == nil {
		r.writerLease = make(map[GUID]time.Duration)
		r.lastSeenAlive = make(map[GUID]time.Time)
		r.livelinessFired = make(map[GUID]bool)
	}
	r.writerLease[g] = lease
	r.lastSeenAlive[g] = time.Now()
	r.livelinessFired[g] = false
}

// touchLiveliness records that writer g was just heard from (a DATA sample or
// a liveliness HEARTBEAT). A no-op for writers this reader is not tracking a
// lease for. Re-arms the "already fired" latch so a writer that comes back
// after being declared lost can be declared lost again on its next silence.
func (r *rtpsReader) touchLiveliness(g GUID) {
	r.livelinessMu.Lock()
	defer r.livelinessMu.Unlock()
	if r.writerLease == nil {
		return
	}
	if _, tracked := r.writerLease[g]; !tracked {
		return
	}
	r.lastSeenAlive[g] = time.Now()
	r.livelinessFired[g] = false
}

// checkLiveliness scans every tracked writer lease and invokes the
// subscriber's LivelinessLostCallback (edge-triggered: once per silence
// episode) for any writer whose lease has expired since it was last heard
// from. Called periodically by livelinessMonitorLoop.
func (r *rtpsReader) checkLiveliness() {
	r.livelinessMu.Lock()
	var lost []GUID
	now := time.Now()
	for g, lease := range r.writerLease {
		if r.livelinessFired[g] {
			continue
		}
		if now.Sub(r.lastSeenAlive[g]) > lease {
			r.livelinessFired[g] = true
			lost = append(lost, g)
		}
	}
	cb := r.livelinessLostCb
	r.livelinessMu.Unlock()
	if cb == nil || len(lost) == 0 {
		return
	}
	for _, g := range lost {
		var ddsGUID dds.GUID
		copy(ddsGUID[:12], g.Prefix[:])
		copy(ddsGUID[12:], g.Entity[:])
		cb(ddsGUID)
	}
}

// livelinessMonitorLoop runs checkLiveliness on a fixed poll interval for as
// long as the subscriber is open. Started only when a LivelinessLostCallback
// is configured (see NewSubscriber); stopped by closing done (see
// rtpsReader.Close).
func (r *rtpsReader) livelinessMonitorLoop(done <-chan struct{}) {
	t := time.NewTicker(livelinessPollInterval)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			r.checkLiveliness()
		case <-done:
			return
		}
	}
}

// ── Time-Based Filter ─────────────────────────────────────────────────────────

// passesTimeBasedFilter implements the MinSeparation QoS: it returns false
// (drop the sample) when less than r.minSeparation has elapsed since the last
// sample from writer source was delivered to this reader. Tracked
// independently per writer so one fast writer cannot starve another matched
// writer's delivery window. A no-op (always true) when MinSeparation is 0.
func (r *rtpsReader) passesTimeBasedFilter(source GUID, now time.Time) bool {
	if r.minSeparation <= 0 {
		return true
	}
	r.tbfMu.Lock()
	defer r.tbfMu.Unlock()
	if r.lastDelivered == nil {
		r.lastDelivered = make(map[GUID]time.Time)
	}
	if last, ok := r.lastDelivered[source]; ok && now.Sub(last) < r.minSeparation {
		return false
	}
	r.lastDelivered[source] = now
	return true
}
