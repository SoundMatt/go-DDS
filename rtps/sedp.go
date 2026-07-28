// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// SEDP — Simple Endpoint Discovery Protocol (RTPS 2.3 §8.5.4 / §9.6.2).
//
// When a local writer or reader is created, SEDP sends a publication or
// subscription announcement to every known participant's meta-unicast port.
// When a remote announcement arrives, the topic-name is matched against
// local endpoints and subscriptions are linked to their matching writers.

package rtps

//fusa:req REQ-DISC-008
//fusa:req REQ-DISC-009
//fusa:req REQ-DISC-010
//fusa:req REQ-DISC-013
//fusa:req REQ-DISC-014

import (
	"net"
	"sync"
	"sync/atomic"
	"time"

	dds "github.com/SoundMatt/go-DDS"
)

// endpointInfo describes a local or remote DDS endpoint.
type endpointInfo struct {
	guid      GUID
	topicName string
	isWriter  bool

	// Milestone 14 "QoS Enforcement — Active Policy" fields, carried over the
	// wire by buildEndpointData/handleEndpointAnnounce for remote endpoints,
	// and populated directly from qos for local endpoints.
	partitions []string // qos.Partition; set for both writers and readers

	// Writer-only fields (isWriter == true); zero value for readers.
	ownershipExclusive bool          // qos.Ownership == dds.ExclusiveOwnership
	ownershipStrength  int32         // qos.OwnershipStrength
	livelinessLease    time.Duration // qos.LivelinessLeaseDuration
}

// sedpService manages endpoint discovery and matching.
type sedpService struct {
	p  *participant
	mu sync.RWMutex
	// Local endpoints registered by this participant.
	localWriters map[EntityId]*endpointInfo
	localReaders map[EntityId]*endpointInfo
	// Remote endpoints discovered from peers.
	remoteWriters    map[GUID]*endpointInfo
	remoteReaders    map[GUID]*endpointInfo // remote subscriptions, keyed by reader GUID
	remoteReaderLocs map[GUID]Locator       // data-unicast locator for each remote reader
	stop             chan struct{}

	// Cumulative count of local↔remote topic endpoint matches.
	endpointMatches atomic.Uint64
}

func newSEDPService(p *participant) *sedpService {
	return &sedpService{
		p:                p,
		localWriters:     make(map[EntityId]*endpointInfo),
		localReaders:     make(map[EntityId]*endpointInfo),
		remoteWriters:    make(map[GUID]*endpointInfo),
		remoteReaders:    make(map[GUID]*endpointInfo),
		remoteReaderLocs: make(map[GUID]Locator),
		stop:             make(chan struct{}),
	}
}

func (s *sedpService) start() {
	go s.receiveLoop()
}

func (s *sedpService) close() {
	close(s.stop)
}

// registerWriter records a local writer and announces it to all known peers.
func (s *sedpService) registerWriter(eid EntityId, topicName string, qos dds.QoS) {
	info := &endpointInfo{
		guid:               GUID{Prefix: s.p.guidPrefix, Entity: eid},
		topicName:          topicName,
		isWriter:           true,
		partitions:         qos.Partition,
		ownershipExclusive: qos.Ownership == dds.ExclusiveOwnership,
		ownershipStrength:  qos.OwnershipStrength,
		livelinessLease:    qos.LivelinessLeaseDuration,
	}
	s.mu.Lock()
	s.localWriters[eid] = info
	s.mu.Unlock()
	s.announceWriter(info, nil)
}

// registerReader records a local reader, announces it, and links any already-
// discovered remote writers with a matching topic whose Partition QoS
// intersects qos.Partition (Milestone 14, "QoS Enforcement — Active Policy";
// see partitionsMatch).
func (s *sedpService) registerReader(eid EntityId, topicName string, qos dds.QoS, r *rtpsReader) {
	info := &endpointInfo{
		guid:       GUID{Prefix: s.p.guidPrefix, Entity: eid},
		topicName:  topicName,
		isWriter:   false,
		partitions: qos.Partition,
	}
	s.mu.Lock()
	s.localReaders[eid] = info
	// Match against already-discovered remote writers.
	for _, rw := range s.remoteWriters {
		if rw.topicName == topicName && partitionsMatch(rw.partitions, qos.Partition) {
			r.addSourceGUID(rw.guid)
			if rw.livelinessLease > 0 {
				r.registerWriterLease(rw.guid, rw.livelinessLease)
			}
		}
	}
	s.mu.Unlock()
	s.announceReader(info, nil)
}

// onNewPeer is called by SPDP when a new participant is discovered.
// Announces all local endpoints to the new participant and requests its endpoints.
func (s *sedpService) onNewPeer(proxy *participantProxy) {
	s.mu.RLock()
	writers := make([]*endpointInfo, 0, len(s.localWriters))
	for _, w := range s.localWriters {
		writers = append(writers, w)
	}
	readers := make([]*endpointInfo, 0, len(s.localReaders))
	for _, r := range s.localReaders {
		readers = append(readers, r)
	}
	s.mu.RUnlock()

	for _, w := range writers {
		s.announceWriter(w, proxy)
	}
	for _, r := range readers {
		s.announceReader(r, proxy)
	}
}

// announceWriter sends a SEDP publications writer announcement.
// If proxy is nil, sends to all known peers.
func (s *sedpService) announceWriter(info *endpointInfo, proxy *participantProxy) {
	payload := s.buildEndpointData(info)
	submsg := marshalDataSubmessage(
		EntityIdSEDPPubWriter,
		EntityIdSEDPPubReader,
		SequenceNumber{High: 0, Low: s.p.nextSeqNum()},
		payload,
	)
	msg := wrapInRTPSMessage(s.p.guidPrefix, submsg)
	s.broadcast(msg, proxy)
}

// announceReader sends a SEDP subscriptions writer announcement.
func (s *sedpService) announceReader(info *endpointInfo, proxy *participantProxy) {
	payload := s.buildEndpointData(info)
	submsg := marshalDataSubmessage(
		EntityIdSEDPSubWriter,
		EntityIdSEDPSubReader,
		SequenceNumber{High: 0, Low: s.p.nextSeqNum()},
		payload,
	)
	msg := wrapInRTPSMessage(s.p.guidPrefix, submsg)
	s.broadcast(msg, proxy)
}

func (s *sedpService) buildEndpointData(info *endpointInfo) []byte {
	enc := newPLCDREncoder()
	enc.addGUID(pidEndpointGUID, info.guid)
	enc.addString(pidTopicName, info.topicName)
	enc.addString(pidTypeName, "CDR_BLOB") // opaque type for raw byte payloads
	userLocator := locatorFromUDP(&net.UDPAddr{IP: net.IPv4zero}, s.p.dataSock.port)
	enc.addLocator(pidDefaultUnicastLocator, userLocator)
	// Milestone 14 "QoS Enforcement — Active Policy": Partition applies to
	// both writers and readers; Ownership/Liveliness only matter on writers.
	for _, part := range info.partitions {
		enc.addString(pidPartition, part)
	}
	if info.isWriter {
		if info.ownershipExclusive {
			enc.addUint32(pidOwnership, 1)
			enc.addUint32(pidOwnershipStrength, uint32(info.ownershipStrength))
		}
		if info.livelinessLease > 0 {
			enc.addInt64(pidLivelinessLeaseDuration, int64(info.livelinessLease))
		}
	}
	if ep, ok := s.p.discoveryPlugin.(EndpointPlugin); ok {
		tag := ep.SignEndpoint(s.p.guidPrefix[:], info.topicName)
		enc.addBytes(pidEndpointToken, tag)
	}
	return enc.finish()
}

func (s *sedpService) broadcast(msg []byte, onlyTo *participantProxy) {
	if onlyTo != nil {
		s.sendTo(msg, onlyTo)
		return
	}
	for _, peer := range s.p.spdp.allPeers() {
		s.sendTo(msg, peer)
	}
}

func (s *sedpService) sendTo(msg []byte, peer *participantProxy) {
	dst := peer.metatrafficUnicast.udpAddr()
	if dst == nil {
		return
	}
	s.p.sendUnicast(s.p.metaSock, dst, msg)
}

func (s *sedpService) receiveLoop() {
	for {
		select {
		case <-s.stop:
			return
		case pkt, ok := <-s.p.metaSock.recv:
			if !ok {
				return
			}
			s.handlePacket(pkt.data, pkt.from)
		}
	}
}

func (s *sedpService) handlePacket(data []byte, from *net.UDPAddr) {
	hdr, ok := parseHeader(data)
	if !ok {
		return
	}
	if hdr.GuidPrefix == s.p.guidPrefix {
		return // own packet
	}
	_ = parseSubmessages(data[20:], func(id, _ byte, body []byte) error {
		if id != submsgDATA {
			return nil
		}
		ds, ok := parseDataSubmessage(flagEndianness|flagData, body)
		if !ok || ds.Payload == nil {
			return nil
		}
		switch ds.WriterEntityId {
		case EntityIdSEDPPubWriter:
			s.handleEndpointAnnounce(hdr.GuidPrefix, ds.Payload, true, from)
		case EntityIdSEDPSubWriter:
			s.handleEndpointAnnounce(hdr.GuidPrefix, ds.Payload, false, from)
		}
		return nil
	})
}

func (s *sedpService) handleEndpointAnnounce(remotePrefix GuidPrefix, payload []byte, isWriter bool, from *net.UDPAddr) {
	dec, ok := newPLCDRDecoder(payload)
	if !ok {
		return
	}
	var info endpointInfo
	info.isWriter = isWriter
	info.guid.Prefix = remotePrefix

	var dataLocator Locator
	var endpointTag []byte
	for {
		p, ok := dec.next()
		if !ok {
			break
		}
		switch p.pid {
		case pidEndpointGUID:
			if g, ok := decodeGUID(p.value); ok {
				info.guid = g
			}
		case pidTopicName:
			if t, ok := decodeString(p.value); ok {
				info.topicName = t
			}
		case pidDefaultUnicastLocator:
			if l, ok := unmarshalLocator(p.value); ok {
				dataLocator = l
				if dataLocator.Address == ([16]byte{}) {
					ip4 := from.IP.To4()
					if ip4 != nil {
						copy(dataLocator.Address[12:], ip4)
					}
				}
			}
		case pidEndpointToken:
			endpointTag = make([]byte, len(p.value))
			copy(endpointTag, p.value)
		case pidPartition:
			if part, ok := decodeString(p.value); ok {
				info.partitions = append(info.partitions, part)
			}
		case pidOwnership:
			if v, ok := decodeUint32(p.value); ok && v != 0 {
				info.ownershipExclusive = true
			}
		case pidOwnershipStrength:
			if v, ok := decodeUint32(p.value); ok {
				info.ownershipStrength = int32(v)
			}
		case pidLivelinessLeaseDuration:
			if v, ok := decodeInt64(p.value); ok {
				info.livelinessLease = time.Duration(v)
			}
		}
	}
	if info.topicName == "" {
		return
	}

	// Reject the announcement if the local participant enforces endpoint security
	// and the tag is absent or invalid.
	if ep, ok := s.p.discoveryPlugin.(EndpointPlugin); ok {
		if !ep.VerifyEndpoint(remotePrefix[:], info.topicName, endpointTag) {
			return
		}
	}

	if isWriter {
		s.onRemoteWriter(&info, dataLocator)
	} else {
		s.onRemoteReader(&info, dataLocator)
	}
}

func (s *sedpService) onRemoteWriter(info *endpointInfo, dataLocator Locator) {
	s.mu.Lock()
	s.remoteWriters[info.guid] = info
	// Ownership QoS (Milestone 14): a remote ExclusiveOwnership writer
	// competes for "active writer" on this topic exactly like a local one.
	if info.ownershipExclusive {
		s.p.ownershipStateFor(info.topicName).register(info.guid, info.ownershipStrength)
	}
	// Match against local readers for this topic whose Partition QoS
	// intersects the writer's (Milestone 14; see partitionsMatch).
	for _, lr := range s.localReaders {
		if lr.topicName == info.topicName && partitionsMatch(lr.partitions, info.partitions) {
			s.endpointMatches.Add(1)
			// Notify the reader so it can accept DATA from this writer.
			s.p.readerByEID(lr.guid.Entity, func(r *rtpsReader) {
				r.addSourceGUID(info.guid)
				if info.livelinessLease > 0 {
					r.registerWriterLease(info.guid, info.livelinessLease)
				}
				// Register the writer's data-delivery address on the participant.
				s.p.addWriterLocator(info.guid, dataLocator)
			})
		}
	}
	s.mu.Unlock()
}

// onRemoteReader stores a remote subscription and its delivery locator so that
// matchedReaderLocators can return only the peers interested in each topic.
func (s *sedpService) onRemoteReader(info *endpointInfo, dataLocator Locator) {
	s.mu.Lock()
	s.remoteReaders[info.guid] = info
	s.remoteReaderLocs[info.guid] = dataLocator
	s.mu.Unlock()
}

// onPeerEvicted removes all remote endpoints belonging to prefix from the SEDP
// tables. Called by the SPDP eviction loop when a participant's lease expires.
func (s *sedpService) onPeerEvicted(prefix GuidPrefix) {
	s.mu.Lock()
	for guid, info := range s.remoteWriters {
		if guid.Prefix == prefix {
			// Ownership QoS (Milestone 14): step the evicted writer down from
			// arbitration so a surviving lower-strength writer can take over.
			if info.ownershipExclusive {
				s.p.ownershipStateFor(info.topicName).unregister(guid)
			}
			delete(s.remoteWriters, guid)
		}
	}
	for guid := range s.remoteReaders {
		if guid.Prefix == prefix {
			delete(s.remoteReaders, guid)
			delete(s.remoteReaderLocs, guid)
		}
	}
	s.mu.Unlock()
	// Remove the evicted participant's writer locators from the participant.
	s.p.mu.Lock()
	for guid := range s.p.writerLocators {
		if guid.Prefix == prefix {
			delete(s.p.writerLocators, guid)
		}
	}
	s.p.mu.Unlock()
}

// seqCounter is a process-wide monotonic sequence number generator.
var seqCounter atomic.Uint32

// nextSeqNum returns the next sequence number for submessage generation.
func (p *participant) nextSeqNum() uint32 {
	return seqCounter.Add(1)
}
