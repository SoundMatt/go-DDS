// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package rtps

import (
	"fmt"
	"sync"
	"sync/atomic"

	dds "github.com/SoundMatt/go-DDS"
)

// participant implements dds.Participant over real RTPS/UDP.
type participant struct {
	domain     dds.Domain
	guidPrefix GuidPrefix

	// Sockets.
	mcastSock *udpSocket // SPDP multicast receive
	metaSock  *udpSocket // SPDP send + SEDP send/receive (unicast)
	dataSock  *udpSocket // User DATA send/receive (unicast)

	// Discovery services.
	spdp *spdpService
	sedp *sedpService

	// Endpoint registry.
	mu             sync.Mutex
	closed         bool
	writers        map[EntityId]*rtpsWriter
	readers        map[EntityId]*rtpsReader
	writerLocators map[GUID]Locator // remote writer → data delivery locator
	entityCounter  uint32           // monotonic counter for entity IDs
}

// New creates an RTPS participant joined to the given DDS domain.
// It binds UDP sockets, starts SPDP, and returns a dds.Participant.
func New(domain dds.Domain) (dds.Participant, error) {
	return newParticipant(domain)
}

func newParticipant(domain dds.Domain) (*participant, error) {
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

	// Multicast socket for SPDP reception.
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
	w := &rtpsWriter{p: p, topic: topic, eid: eid}
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
		p:     p,
		topic: topic,
		eid:   eid,
		ch:    make(chan dds.Sample, 64),
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
	p.spdp.close()
	p.sedp.close()
	p.mcastSock.close()
	p.metaSock.close()
	p.dataSock.close()
	return nil
}

// dataReceiveLoop reads user DATA submessages and dispatches to readers.
func (p *participant) dataReceiveLoop() {
	for pkt := range p.dataSock.recv {
		p.handleDataPacket(pkt.data)
	}
}

func (p *participant) handleDataPacket(data []byte) {
	hdr, ok := parseHeader(data)
	if !ok {
		return
	}
	_ = parseSubmessages(data[20:], func(id, _ byte, body []byte) error {
		if id != submsgDATA {
			return nil
		}
		ds, ok := parseDataSubmessage(flagEndianness|flagData, body)
		if !ok || ds.Payload == nil {
			return nil
		}
		rawPayload, ok := cdrUnwrapPayload(ds.Payload)
		if !ok {
			return nil
		}
		sourceGUID := GUID{Prefix: hdr.GuidPrefix, Entity: ds.WriterEntityId}
		p.dispatchToReaders(sourceGUID, "", rawPayload)
		return nil
	})
}

// dispatchToReaders delivers payload to all readers that have sourceGUID in
// their accept-list. topicFilter, when non-empty, restricts delivery to readers
// on that topic (used for intra-process dispatch where topic is known; UDP
// paths pass "" and rely on SEDP-based source GUID filtering instead).
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
			// Only include if SEDP has matched a reader for this topic.
			// For simplicity in phase 2, send to all known peers; recipients
			// discard packets for topics they don't subscribe to.
			locators = append(locators, peer.defaultUnicast)
		}
	}
	return locators
}

// ── Writer ────────────────────────────────────────────────────────────────────

type rtpsWriter struct {
	p      *participant
	topic  string
	eid    EntityId
	mu     sync.Mutex
	closed bool
	seqHi  int32
	seqLo  uint32
}

func (w *rtpsWriter) Write(payload []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return fmt.Errorf("rtps: writer closed")
	}
	w.seqLo++
	seqNum := SequenceNumber{High: w.seqHi, Low: w.seqLo}

	wrapped := cdrWrapPayload(payload)
	submsg := marshalDataSubmessage(w.eid, EntityIdUnknown, seqNum, wrapped)
	msg := wrapInRTPSMessage(w.p.guidPrefix, submsg)

	// Deliver locally (same process). Copy payload so caller mutations after
	// Write do not affect the already-queued sample.
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
	return nil
}

func (w *rtpsWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.closed = true
	return nil
}

// ── Reader ────────────────────────────────────────────────────────────────────

type rtpsReader struct {
	p     *participant
	topic string
	eid   EntityId
	ch    chan dds.Sample
	mu    sync.RWMutex
	// sources is the set of remote writer GUIDs whose DATA this reader accepts.
	sources map[GUID]struct{}
	once    sync.Once
}

func (r *rtpsReader) addSourceGUID(g GUID) {
	r.mu.Lock()
	if r.sources == nil {
		r.sources = make(map[GUID]struct{})
	}
	r.sources[g] = struct{}{}
	r.mu.Unlock()
}

func (r *rtpsReader) acceptsSource(g GUID) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.sources) == 0 {
		// Before any SEDP matching, accept from our own participant.
		return g.Prefix == r.p.guidPrefix
	}
	// Accept from local participant (intra-process) or matched remote writers.
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
