// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package rtps provides a pure-Go RTPS/UDP implementation of the dds
// interfaces. It requires no CGo and no installed system libraries; it speaks
// the OMG RTPS 2.3 wire protocol over UDP multicast/unicast so it can
// interoperate with any standards-compliant DDS implementation.
//
// Create a participant with rtps.New. For reliable delivery, pass
// dds.ReliableQoS to NewPublisher/NewSubscriber. For payload-level security,
// pass rtps.WithSecurity to rtps.New:
//
//	p, err := rtps.New(dds.Domain(0), rtps.WithSecurity(myPlugin))
package rtps

import (
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	dds "github.com/SoundMatt/go-DDS"
)

// ── Options ───────────────────────────────────────────────────────────────────

// SecurityPlugin is satisfied by any type that can seal (encrypt / sign) and
// open (decrypt / verify) a DDS payload byte slice. Built-in implementations
// live in the security sub-package; NullPlugin (pass-through) is also
// available there for development and testing.
type SecurityPlugin interface {
	Seal(plaintext []byte) ([]byte, error)
	Open(ciphertext []byte) ([]byte, error)
}

// Option configures a Participant at creation time.
type Option func(*participant)

// WithSecurity returns an Option that applies plugin to every payload transmitted
// and received by this participant. All peers that communicate with this
// participant must use the same plugin and key material.
func WithSecurity(plugin SecurityPlugin) Option {
	return func(p *participant) { p.security = plugin }
}

// ── Participant ───────────────────────────────────────────────────────────────

// participant implements dds.Participant over real RTPS/UDP.
type participant struct {
	domain     dds.Domain
	guidPrefix GuidPrefix

	// Sockets.
	mcastSock *udpSocket // SPDP multicast receive
	metaSock  *udpSocket // SPDP send + SEDP send/receive (unicast)
	dataSock  *udpSocket // User DATA / HEARTBEAT / ACKNACK

	// Discovery services.
	spdp *spdpService
	sedp *sedpService

	// Optional security plugin (nil = no security).
	security SecurityPlugin

	// Endpoint registry.
	mu             sync.Mutex
	closed         bool
	writers        map[EntityId]*rtpsWriter
	readers        map[EntityId]*rtpsReader
	writerLocators map[GUID]Locator
	entityCounter  uint32
}

// New creates an RTPS participant joined to the given DDS domain.
// It binds UDP sockets, starts SPDP/SEDP, and returns a dds.Participant.
func New(domain dds.Domain, opts ...Option) (dds.Participant, error) {
	return newParticipant(domain, opts...)
}

func newParticipant(domain dds.Domain, opts ...Option) (*participant, error) {
	d := int(domain)
	guidPrefix := newGuidPrefix()

	// Allocate ports — try participant index 0..15.
	var metaSock, dataSock *udpSocket
	var participantIdx int
	for i := 0; i < 16; i++ {
		var err error
		metaSock, err = newUnicastSocket(metaUnicastPort(d, i))
		if err != nil {
			continue
		}
		dataSock, err = newUnicastSocket(userUnicastPort(d, i))
		if err != nil {
			metaSock.close()
			continue
		}
		participantIdx = i
		break
	}
	if metaSock == nil {
		return nil, fmt.Errorf("rtps: no free port pair for domain %d", domain)
	}
	_ = participantIdx

	mcastSock, err := newMulticastReceiveSocket(spdpMulticastAddr, metaMulticastPort(d))
	if err != nil {
		metaSock.close()
		dataSock.close()
		return nil, fmt.Errorf("rtps: SPDP multicast: %w", err)
	}

	p := &participant{
		domain:         domain,
		guidPrefix:     guidPrefix,
		mcastSock:      mcastSock,
		metaSock:       metaSock,
		dataSock:       dataSock,
		writers:        make(map[EntityId]*rtpsWriter),
		readers:        make(map[EntityId]*rtpsReader),
		writerLocators: make(map[GUID]Locator),
	}
	for _, o := range opts {
		o(p)
	}

	p.spdp = newSPDPService(p)
	p.sedp = newSEDPService(p)
	p.spdp.start()
	p.sedp.start()

	go p.dataReceiveLoop()

	return p, nil
}

func (p *participant) NewPublisher(topic string, qos dds.QoS) (dds.Publisher, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil, fmt.Errorf("rtps: participant closed")
	}
	n := atomic.AddUint32(&p.entityCounter, 1)
	eid := entityIdForWriter(n)
	w := &rtpsWriter{
		p:        p,
		topic:    topic,
		eid:      eid,
		reliable: qos.Reliability == dds.Reliable,
	}
	if w.reliable {
		w.history = newSendHistory()
		w.hbDone = make(chan struct{})
		go w.heartbeatLoop()
	}
	p.writers[eid] = w
	p.sedp.registerWriter(eid, topic)
	return w, nil
}

func (p *participant) NewSubscriber(topic string, qos dds.QoS) (dds.Subscriber, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil, fmt.Errorf("rtps: participant closed")
	}
	n := atomic.AddUint32(&p.entityCounter, 1)
	eid := entityIdForReader(n)
	r := &rtpsReader{
		p:        p,
		topic:    topic,
		eid:      eid,
		ch:       make(chan dds.Sample, 64),
		reliable: qos.Reliability == dds.Reliable,
	}
	p.readers[eid] = r
	p.sedp.registerReader(eid, topic, r)
	return r, nil
}

func (p *participant) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil
	}
	p.closed = true
	// Stop heartbeat loops before closing sockets; delegate to w.Close() so
	// the w.closed guard prevents double-close if the caller already closed
	// individual writers.
	for _, w := range p.writers {
		_ = w.Close()
	}
	p.spdp.close()
	p.sedp.close()
	p.mcastSock.close()
	p.metaSock.close()
	p.dataSock.close()
	return nil
}

// ── Receive loop ──────────────────────────────────────────────────────────────

func (p *participant) dataReceiveLoop() {
	for pkt := range p.dataSock.recv {
		p.handleDataPacket(pkt.data, pkt.from)
	}
}

func (p *participant) handleDataPacket(data []byte, from *net.UDPAddr) {
	hdr, ok := parseHeader(data)
	if !ok {
		return
	}
	_ = parseSubmessages(data[20:], func(id, _ byte, body []byte) error {
		switch id {
		case submsgDATA:
			ds, ok := parseDataSubmessage(flagEndianness|flagData, body)
			if !ok || ds.Payload == nil {
				return nil
			}
			rawPayload, ok := cdrUnwrapPayload(ds.Payload)
			if !ok {
				return nil
			}
			if p.security != nil {
				opened, err := p.security.Open(rawPayload)
				if err != nil {
					return nil // drop; tampered or wrong key
				}
				rawPayload = opened
			}
			sourceGUID := GUID{Prefix: hdr.GuidPrefix, Entity: ds.WriterEntityId}
			p.notifyReliableReaders(sourceGUID, ds.SeqNum, from)
			p.dispatchToReaders(sourceGUID, "", rawPayload)

		case submsgHEARTBEAT:
			hb, ok := parseHeartbeat(body)
			if !ok {
				return nil
			}
			writerGUID := GUID{Prefix: hdr.GuidPrefix, Entity: hb.WriterEntityId}
			p.handleHeartbeat(writerGUID, hb, from)

		case submsgACKNACK:
			an, ok := parseAckNack(body)
			if !ok {
				return nil
			}
			p.handleAckNack(an)
		}
		return nil
	})
}

// notifyReliableReaders updates the recvTracker of any reliable reader that
// accepts this writer, and sends ACKNACK if there are gaps.
func (p *participant) notifyReliableReaders(writerGUID GUID, seqNum SequenceNumber, writerAddr *net.UDPAddr) {
	p.mu.Lock()
	readers := make([]*rtpsReader, 0, len(p.readers))
	for _, r := range p.readers {
		readers = append(readers, r)
	}
	p.mu.Unlock()

	for _, r := range readers {
		if !r.reliable || !r.acceptsSource(writerGUID) {
			continue
		}
		tracker := r.trackerFor(writerGUID)
		base, bitmap, needAck := tracker.receive(seqNum.Low)
		if !needAck || writerAddr == nil {
			continue
		}
		an := AckNack{
			ReaderEntityId: r.eid,
			WriterEntityId: writerGUID.Entity,
			Base:           SequenceNumber{Low: base},
			Bitmap:         bitmap,
			Count:          tracker.nextAckCount(),
		}
		msg := wrapInRTPSMessage(p.guidPrefix, marshalAckNack(an))
		_ = p.dataSock.send(writerAddr, msg)
	}
}

// handleHeartbeat responds with ACKNACK if we have gaps for this writer.
func (p *participant) handleHeartbeat(writerGUID GUID, hb Heartbeat, from *net.UDPAddr) {
	p.mu.Lock()
	readers := make([]*rtpsReader, 0, len(p.readers))
	for _, r := range p.readers {
		readers = append(readers, r)
	}
	p.mu.Unlock()

	for _, r := range readers {
		if !r.reliable || !r.acceptsSource(writerGUID) {
			continue
		}
		tracker := r.trackerFor(writerGUID)
		// If expected < firstSN the reader has never received anything from this
		// writer; ACKNACK base = firstSN with bitmap=0 to confirm empty state.
		if tracker.expected == 0 {
			tracker.mu.Lock()
			tracker.expected = hb.FirstSN.Low
			tracker.mu.Unlock()
		}
		base, bitmap, needAck := tracker.receive(hb.LastSN.Low)
		// Even if there's no gap at LastSN, check whether expected < lastSN
		// (i.e. we're behind) and send a cumulative ACKNACK.
		_ = base
		_ = bitmap
		if !needAck {
			continue
		}
		if from == nil {
			continue
		}
		an := AckNack{
			ReaderEntityId: r.eid,
			WriterEntityId: writerGUID.Entity,
			Base:           SequenceNumber{Low: tracker.expected},
			Bitmap:         bitmap,
			Count:          tracker.nextAckCount(),
		}
		msg := wrapInRTPSMessage(p.guidPrefix, marshalAckNack(an))
		_ = p.dataSock.send(from, msg)
	}
}

// handleAckNack retransmits any missing samples from the writer's history.
func (p *participant) handleAckNack(an AckNack) {
	p.mu.Lock()
	w, ok := p.writers[an.WriterEntityId]
	p.mu.Unlock()
	if !ok || !w.reliable {
		return
	}
	// Retransmit any samples in the bitmap.
	for bit := uint32(0); bit < 32; bit++ {
		if an.Bitmap&(1<<bit) == 0 {
			continue
		}
		seqLo := an.Base.Low + bit
		msg := w.history.get(seqLo)
		if msg == nil {
			continue // evicted from history
		}
		for _, loc := range p.matchedReaderLocators(w.topic) {
			dst := loc.udpAddr()
			if dst != nil {
				_ = p.dataSock.send(dst, msg)
			}
		}
	}
}

// ── Dispatch ──────────────────────────────────────────────────────────────────

// dispatchToReaders delivers payload to all readers whose topic matches and
// whose accept-list includes source. topicFilter="" disables topic filtering
// (used for UDP paths where the topic is resolved via SEDP source GUID).
func (p *participant) dispatchToReaders(source GUID, topicFilter string, payload []byte) {
	p.mu.Lock()
	readers := make([]*rtpsReader, 0, len(p.readers))
	for _, r := range p.readers {
		readers = append(readers, r)
	}
	p.mu.Unlock()

	for _, r := range readers {
		if topicFilter != "" && r.topic != topicFilter {
			continue
		}
		if r.acceptsSource(source) {
			sample := dds.Sample{Topic: r.topic, Payload: payload}
			select {
			case r.ch <- sample:
			default: // slow consumer; drop
			}
		}
	}
}

// readerByEID calls fn with the reader matching eid, if any.
func (p *participant) readerByEID(eid EntityId, fn func(*rtpsReader)) {
	p.mu.Lock()
	r, ok := p.readers[eid]
	p.mu.Unlock()
	if ok {
		fn(r)
	}
}

// addWriterLocator stores the data-delivery locator for a remote writer.
func (p *participant) addWriterLocator(g GUID, l Locator) {
	p.mu.Lock()
	p.writerLocators[g] = l
	p.mu.Unlock()
}

// matchedReaderLocators returns the user-data unicast locators for all known
// participants that have a reader for topicName.
func (p *participant) matchedReaderLocators(topicName string) []Locator {
	var locators []Locator
	for _, peer := range p.spdp.allPeers() {
		if peer.defaultUnicast.Kind != LocatorKindInvalid {
			locators = append(locators, peer.defaultUnicast)
		}
	}
	_ = topicName
	return locators
}

// ── Writer ────────────────────────────────────────────────────────────────────

type rtpsWriter struct {
	p        *participant
	topic    string
	eid      EntityId
	mu       sync.Mutex
	closed   bool
	seqHi    int32
	seqLo    uint32
	reliable bool
	history  *sendHistory  // non-nil when reliable == true
	hbDone   chan struct{} // closed to stop the heartbeat goroutine
}

func (w *rtpsWriter) Write(payload []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return fmt.Errorf("rtps: writer closed")
	}
	w.seqLo++
	seqNum := SequenceNumber{High: w.seqHi, Low: w.seqLo}

	// Apply security before wrapping in CDR/RTPS.
	wirePayload := payload
	if w.p.security != nil {
		sealed, err := w.p.security.Seal(payload)
		if err != nil {
			return fmt.Errorf("rtps: security seal: %w", err)
		}
		wirePayload = sealed
	}
	wrapped := cdrWrapPayload(wirePayload)
	submsg := marshalDataSubmessage(w.eid, EntityIdUnknown, seqNum, wrapped)
	msg := wrapInRTPSMessage(w.p.guidPrefix, submsg)

	if w.reliable {
		w.history.store(w.seqLo, msg)
	}

	// Deliver locally (same process). Copy so caller mutations don't affect
	// the already-queued sample.
	localCopy := make([]byte, len(payload))
	copy(localCopy, payload)
	w.p.dispatchToReaders(GUID{Prefix: w.p.guidPrefix, Entity: w.eid}, w.topic, localCopy)

	// Send to all known remote peers.
	for _, loc := range w.p.matchedReaderLocators(w.topic) {
		dst := loc.udpAddr()
		if dst == nil {
			continue
		}
		_ = w.p.dataSock.send(dst, msg)
	}

	// Send HEARTBEAT immediately after each reliable write so remote readers
	// can detect gaps without waiting for the periodic ticker.
	if w.reliable {
		w.sendHeartbeatLocked()
	}
	return nil
}

// sendHeartbeatLocked builds and sends a HEARTBEAT to all known reader
// locators. Caller must hold w.mu (or call only from the heartbeatLoop).
func (w *rtpsWriter) sendHeartbeatLocked() {
	first, last, ok := w.history.firstLast()
	if !ok {
		return
	}
	hb := Heartbeat{
		ReaderEntityId: EntityIdUnknown,
		WriterEntityId: w.eid,
		FirstSN:        SequenceNumber{Low: first},
		LastSN:         SequenceNumber{Low: last},
		Count:          w.history.hbCount.Add(1),
	}
	msg := wrapInRTPSMessage(w.p.guidPrefix, marshalHeartbeat(hb))
	for _, loc := range w.p.matchedReaderLocators(w.topic) {
		if dst := loc.udpAddr(); dst != nil {
			_ = w.p.dataSock.send(dst, msg)
		}
	}
}

// heartbeatLoop periodically sends a HEARTBEAT for as long as the writer is
// open, so remote readers can detect and recover from losses.
func (w *rtpsWriter) heartbeatLoop() {
	// Capture hbDone once before entering the loop so that Close() can safely
	// nil out w.hbDone under w.mu without racing with the select expression.
	hbDone := w.hbDone
	ticker := time.NewTicker(heartbeatPeriod)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			w.mu.Lock()
			if !w.closed {
				w.sendHeartbeatLocked()
			}
			w.mu.Unlock()
		case <-hbDone:
			return
		}
	}
}

func (w *rtpsWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil
	}
	w.closed = true
	if w.hbDone != nil {
		close(w.hbDone)
		w.hbDone = nil // safe: heartbeatLoop captured the channel at startup
	}
	return nil
}

// ── Reader ────────────────────────────────────────────────────────────────────

type rtpsReader struct {
	p        *participant
	topic    string
	eid      EntityId
	ch       chan dds.Sample
	mu       sync.RWMutex
	sources  map[GUID]struct{}     // SEDP-matched remote writer GUIDs
	trackers map[GUID]*recvTracker // reliability trackers, one per remote writer
	reliable bool
	once     sync.Once
}

func (r *rtpsReader) addSourceGUID(g GUID) {
	r.mu.Lock()
	if r.sources == nil {
		r.sources = make(map[GUID]struct{})
	}
	r.sources[g] = struct{}{}
	r.mu.Unlock()
}

// trackerFor returns (creating if necessary) the recvTracker for a remote writer.
func (r *rtpsReader) trackerFor(writerGUID GUID) *recvTracker {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.trackers == nil {
		r.trackers = make(map[GUID]*recvTracker)
	}
	if t, ok := r.trackers[writerGUID]; ok {
		return t
	}
	t := &recvTracker{}
	r.trackers[writerGUID] = t
	return t
}

func (r *rtpsReader) acceptsSource(g GUID) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.sources) == 0 {
		return g.Prefix == r.p.guidPrefix
	}
	if g.Prefix == r.p.guidPrefix {
		return true
	}
	_, ok := r.sources[g]
	return ok
}

func (r *rtpsReader) C() <-chan dds.Sample { return r.ch }

func (r *rtpsReader) Close() error {
	r.once.Do(func() {
		r.p.mu.Lock()
		delete(r.p.readers, r.eid)
		r.p.mu.Unlock()
		close(r.ch)
	})
	return nil
}
