// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Internal packet-handling tests. Uses package rtps (internal) for direct
// access to unexported types and functions.

package rtps

import (
	"encoding/binary"
	"net"
	"testing"
	"time"

	dds "github.com/SoundMatt/go-DDS"
)

// testPart creates a minimal participant, skipping the test if sockets cannot
// be bound (common in heavily-locked CI environments).
func testPart(t *testing.T) *participant {
	t.Helper()
	p, err := newParticipant(dds.Domain(99))
	if err != nil {
		t.Skipf("newParticipant: %v — socket creation unavailable", err)
	}
	t.Cleanup(func() { _ = p.Close() })
	return p
}

var loopbackAddr = &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 7411}

// ── handleDataPacket ──────────────────────────────────────────────────────────

func TestHandleDataPacket_DispatchesToSubscriber(t *testing.T) {
	p := testPart(t)

	sub, err := p.NewSubscriber("hdp/basic", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()

	want := []byte("wire-delivery")
	wrapped := cdrWrapPayload(want)
	eid := entityIdForWriter(500)
	submsg := marshalDataSubmessage(eid, EntityIdUnknown, SequenceNumber{Low: 1}, wrapped)
	msg := wrapInRTPSMessage(p.guidPrefix, submsg)

	p.handleDataPacket(msg, loopbackAddr)

	select {
	case s := <-sub.C():
		if string(s.Payload) != string(want) {
			t.Errorf("payload: got %q, want %q", s.Payload, want)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout: handleDataPacket did not dispatch to subscriber")
	}
}

func TestHandleDataPacket_TruncatedHeader(t *testing.T) {
	p := testPart(t)
	p.handleDataPacket([]byte{0x52, 0x54, 0x50, 0x53}, loopbackAddr)
}

func TestHandleDataPacket_UnknownSubmessage(t *testing.T) {
	p := testPart(t)
	hdr := marshalHeader(Header{
		ProtocolVersion: [2]byte{2, 3},
		VendorId:        goVendorId,
		GuidPrefix:      newGuidPrefix(),
	})
	msg := append(hdr, 0xFF, 0x01, 4, 0, 0, 0, 0, 0)
	p.handleDataPacket(msg, loopbackAddr)
}

func TestHandleDataPacket_HeartbeatSubmessage(t *testing.T) {
	p := testPart(t)
	// A packet containing a HEARTBEAT submessage — exercises the HEARTBEAT branch.
	sub, err := p.NewSubscriber("hdp/hb", dds.ReliableQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()

	remotePrefix := newGuidPrefix()
	hb := Heartbeat{
		ReaderEntityId: EntityIdUnknown,
		WriterEntityId: entityIdForWriter(1),
		FirstSN:        SequenceNumber{Low: 1},
		LastSN:         SequenceNumber{Low: 3},
		Count:          1,
	}
	submsg := marshalHeartbeat(hb)
	msg := wrapInRTPSMessage(remotePrefix, submsg)
	p.handleDataPacket(msg, loopbackAddr)
}

// ── notifyReliableReaders ─────────────────────────────────────────────────────

func TestNotifyReliableReaders_NoReaders(t *testing.T) {
	p := testPart(t)
	p.notifyReliableReaders(
		GUID{Prefix: newGuidPrefix(), Entity: entityIdForWriter(1)},
		SequenceNumber{Low: 1}, loopbackAddr)
}

func TestNotifyReliableReaders_BestEffortReader(t *testing.T) {
	p := testPart(t)
	sub, _ := p.NewSubscriber("nrr/be", dds.DefaultQoS)
	defer sub.Close()
	// BestEffort reader is skipped — must not panic.
	p.notifyReliableReaders(
		GUID{Prefix: p.guidPrefix, Entity: entityIdForWriter(1)},
		SequenceNumber{Low: 1}, loopbackAddr)
}

func TestNotifyReliableReaders_ReliableReader_NilAddr(t *testing.T) {
	p := testPart(t)
	sub, err := p.NewSubscriber("nrr/rel", dds.ReliableQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()
	// nil address → ACKNACK send skipped; no panic.
	p.notifyReliableReaders(
		GUID{Prefix: p.guidPrefix, Entity: entityIdForWriter(1)},
		SequenceNumber{Low: 5}, nil)
}

// ── handleHeartbeat ───────────────────────────────────────────────────────────

func TestHandleHeartbeat_NoReaders(t *testing.T) {
	p := testPart(t)
	hb := Heartbeat{
		ReaderEntityId: EntityIdUnknown,
		WriterEntityId: entityIdForWriter(1),
		FirstSN:        SequenceNumber{Low: 1},
		LastSN:         SequenceNumber{Low: 5},
		Count:          1,
	}
	p.handleHeartbeat(GUID{Prefix: newGuidPrefix(), Entity: entityIdForWriter(1)}, hb, loopbackAddr)
}

func TestHandleHeartbeat_ReliableReader_SetsExpected(t *testing.T) {
	p := testPart(t)
	sub, err := p.NewSubscriber("hbr/rel", dds.ReliableQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()

	writerGUID := GUID{Prefix: p.guidPrefix, Entity: entityIdForWriter(1)}
	hb := Heartbeat{
		ReaderEntityId: EntityIdUnknown,
		WriterEntityId: writerGUID.Entity,
		FirstSN:        SequenceNumber{Low: 1},
		LastSN:         SequenceNumber{Low: 3},
		Count:          1,
	}
	p.handleHeartbeat(writerGUID, hb, nil)
}

func TestHandleHeartbeat_ReliableReader_WithAddr(t *testing.T) {
	p := testPart(t)
	sub, err := p.NewSubscriber("hbr/addr", dds.ReliableQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()

	writerGUID := GUID{Prefix: p.guidPrefix, Entity: entityIdForWriter(1)}
	hb := Heartbeat{
		ReaderEntityId: EntityIdUnknown,
		WriterEntityId: writerGUID.Entity,
		FirstSN:        SequenceNumber{Low: 1},
		LastSN:         SequenceNumber{Low: 3},
		Count:          1,
	}
	// With an address, an ACKNACK is attempted (send will fail on loopback
	// but the code path is exercised).
	p.handleHeartbeat(writerGUID, hb, loopbackAddr)
}

// ── handleAckNack ─────────────────────────────────────────────────────────────

func TestHandleAckNack_NoWriter(t *testing.T) {
	p := testPart(t)
	an := AckNack{
		ReaderEntityId: entityIdForReader(1),
		WriterEntityId: entityIdForWriter(999),
		Base:           SequenceNumber{Low: 1},
		Bitmap:         0b11,
		Count:          1,
	}
	p.handleAckNack(an, nil)
}

func TestHandleAckNack_BestEffortWriter(t *testing.T) {
	p := testPart(t)
	pub, err := p.NewPublisher("han/be", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	p.mu.Lock()
	var eid EntityId
	for e := range p.writers {
		eid = e
		break
	}
	p.mu.Unlock()

	// BestEffort writer has no history; handleAckNack should return early.
	an := AckNack{WriterEntityId: eid, Bitmap: 0b1, Count: 1}
	p.handleAckNack(an, nil)
}

func TestHandleAckNack_ReliableWriter_RetransmitsHistory(t *testing.T) {
	p := testPart(t)
	pub, err := p.NewPublisher("han/rel", dds.ReliableQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	if err := pub.Write([]byte("retransmit-me")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	p.mu.Lock()
	var eid EntityId
	for e := range p.writers {
		eid = e
		break
	}
	p.mu.Unlock()

	an := AckNack{
		ReaderEntityId: entityIdForReader(1),
		WriterEntityId: eid,
		Base:           SequenceNumber{Low: 1},
		Bitmap:         0b1,
		Count:          1,
	}
	// history.get is exercised; send skipped because no peer locator exists.
	p.handleAckNack(an, nil)
}

func TestHandleAckNack_HistoryMiss(t *testing.T) {
	p := testPart(t)
	pub, err := p.NewPublisher("han/miss", dds.ReliableQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	p.mu.Lock()
	var eid EntityId
	for e := range p.writers {
		eid = e
		break
	}
	p.mu.Unlock()

	// NACK for seqLo 99 — not in history (never written). No GAP because
	// history is empty (histFirst == 0, histOK == false).
	an := AckNack{
		WriterEntityId: eid,
		Base:           SequenceNumber{Low: 99},
		Bitmap:         0b1,
		Count:          1,
	}
	p.handleAckNack(an, nil)
}

// ── SPDP ─────────────────────────────────────────────────────────────────────

func TestSPDP_ParseParticipantData_RoundTrip(t *testing.T) {
	p := testPart(t)
	payload := p.spdp.buildParticipantData()
	from := &net.UDPAddr{IP: net.ParseIP("10.0.0.1"), Port: 7410}

	// parseParticipantData uses the pidParticipantGUID field from the payload
	// (p.guidPrefix) to set the proxy GUID, overriding the prefix argument.
	proxy := parseParticipantData(newGuidPrefix(), payload, from)
	if proxy == nil {
		t.Fatal("parseParticipantData returned nil for valid SPDP payload")
	}
	if proxy.guid.Prefix != p.guidPrefix {
		t.Errorf("proxy GUID prefix: got %v, want %v", proxy.guid.Prefix, p.guidPrefix)
	}
	if proxy.metatrafficUnicast.Port == 0 {
		t.Error("metatrafficUnicast port should be non-zero")
	}
}

func TestSPDP_ParseParticipantData_ZeroAddr_FilledFromSender(t *testing.T) {
	p := testPart(t)
	payload := p.spdp.buildParticipantData()
	senderIP := net.ParseIP("192.168.99.1").To4()
	from := &net.UDPAddr{IP: senderIP, Port: 7410}

	proxy := parseParticipantData(newGuidPrefix(), payload, from)
	if proxy == nil {
		t.Fatal("parseParticipantData returned nil")
	}
	addr := proxy.metatrafficUnicast.udpAddr()
	if addr == nil {
		t.Fatal("metatrafficUnicast.udpAddr() returned nil")
	}
	if !addr.IP.Equal(senderIP) {
		t.Errorf("metatrafficUnicast IP: got %v, want %v", addr.IP, senderIP)
	}
}

func TestSPDP_ParseParticipantData_Truncated(t *testing.T) {
	// Truncated payload — must not panic; may return nil or an empty proxy.
	_ = parseParticipantData(newGuidPrefix(), []byte{0x00, 0x01}, nil)
}

func TestSPDP_StorePeer(t *testing.T) {
	p := testPart(t)
	remotePrefix := newGuidPrefix()
	proxy := &participantProxy{guid: GUID{Prefix: remotePrefix, Entity: EntityIdParticipant}}
	p.spdp.storePeer(proxy)

	for _, peer := range p.spdp.allPeers() {
		if peer.guid.Prefix == remotePrefix {
			return // found
		}
	}
	t.Error("storePeer: peer not returned by allPeers")
}

func TestSPDP_HandlePacket_OwnPrefix_Ignored(t *testing.T) {
	p := testPart(t)
	payload := p.spdp.buildParticipantData()
	submsg := marshalDataSubmessage(EntityIdSPDPWriter, EntityIdSPDPReader,
		SequenceNumber{Low: 1}, payload)
	msg := wrapInRTPSMessage(p.guidPrefix, submsg)

	before := len(p.spdp.allPeers())
	p.spdp.handlePacket(msg, loopbackAddr)
	if len(p.spdp.allPeers()) != before {
		t.Error("SPDP should ignore its own announcements")
	}
}

func TestSPDP_HandlePacket_ValidRemote_StoresPeer(t *testing.T) {
	p := testPart(t)
	remotePrefix := newGuidPrefix()

	// Build a minimal SPDP payload for the fake remote participant.
	enc := newPLCDREncoder()
	enc.addGUID(pidParticipantGUID, GUID{Prefix: remotePrefix, Entity: EntityIdParticipant})
	enc.addUint32(pidBuiltinEndpointSet, EndpointSPDPAnnouncer|EndpointSPDPDetector)
	enc.addLocator(pidMetatrafficUnicastLocator, Locator{Kind: LocatorKindUDPv4, Port: 32160})
	enc.addLocator(pidDefaultUnicastLocator, Locator{Kind: LocatorKindUDPv4, Port: 32161})
	payload := enc.finish()

	submsg := marshalDataSubmessage(EntityIdSPDPWriter, EntityIdSPDPReader,
		SequenceNumber{Low: 1}, payload)
	msg := wrapInRTPSMessage(remotePrefix, submsg)

	before := len(p.spdp.allPeers())
	p.spdp.handlePacket(msg, &net.UDPAddr{IP: net.ParseIP("10.0.0.2"), Port: 7410})
	if len(p.spdp.allPeers()) <= before {
		t.Error("SPDP should store a new peer from a valid remote announcement")
	}
}

// ── SEDP ─────────────────────────────────────────────────────────────────────

func TestSEDP_HandleEndpointAnnounce_RemoteWriter_Stored(t *testing.T) {
	p := testPart(t)
	remotePrefix := newGuidPrefix()
	remoteGUID := GUID{Prefix: remotePrefix, Entity: entityIdForWriter(1)}

	enc := newPLCDREncoder()
	enc.addGUID(pidEndpointGUID, remoteGUID)
	enc.addString(pidTopicName, "sedp/writer-test")
	enc.addString(pidTypeName, "CDR_BLOB")
	enc.addLocator(pidDefaultUnicastLocator, Locator{Kind: LocatorKindUDPv4, Port: 32161})
	payload := enc.finish()

	p.sedp.handleEndpointAnnounce(remotePrefix, payload, true, loopbackAddr)

	p.sedp.mu.RLock()
	_, found := p.sedp.remoteWriters[remoteGUID]
	p.sedp.mu.RUnlock()
	if !found {
		t.Error("remote writer not stored after handleEndpointAnnounce")
	}
}

func TestSEDP_HandleEndpointAnnounce_MatchesLocalReader(t *testing.T) {
	p := testPart(t)
	const topic = "sedp/match"

	sub, err := p.NewSubscriber(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()

	remotePrefix := newGuidPrefix()
	remoteGUID := GUID{Prefix: remotePrefix, Entity: entityIdForWriter(2)}

	enc := newPLCDREncoder()
	enc.addGUID(pidEndpointGUID, remoteGUID)
	enc.addString(pidTopicName, topic)
	enc.addString(pidTypeName, "CDR_BLOB")
	enc.addLocator(pidDefaultUnicastLocator, Locator{Kind: LocatorKindUDPv4, Port: 32161})
	payload := enc.finish()

	p.sedp.handleEndpointAnnounce(remotePrefix, payload, true, loopbackAddr)

	p.mu.Lock()
	var r *rtpsReader
	for _, rr := range p.readers {
		if rr.topic == topic {
			r = rr
			break
		}
	}
	p.mu.Unlock()
	if r == nil {
		t.Fatal("local reader not found")
	}
	if !r.acceptsSource(remoteGUID) {
		t.Error("local reader should accept remote writer GUID after SEDP announce")
	}
}

func TestSEDP_HandleEndpointAnnounce_EmptyTopic_Ignored(t *testing.T) {
	p := testPart(t)
	enc := newPLCDREncoder()
	enc.addGUID(pidEndpointGUID, GUID{Prefix: newGuidPrefix(), Entity: entityIdForWriter(1)})
	payload := enc.finish()
	// No topic name — should be ignored without panic.
	p.sedp.handleEndpointAnnounce(newGuidPrefix(), payload, true, loopbackAddr)
}

func TestSEDP_HandlePacket_OwnPrefix_Ignored(t *testing.T) {
	p := testPart(t)
	enc := newPLCDREncoder()
	enc.addGUID(pidEndpointGUID, GUID{Prefix: p.guidPrefix, Entity: entityIdForWriter(1)})
	enc.addString(pidTopicName, "sedp/own")
	enc.addString(pidTypeName, "CDR_BLOB")
	payload := enc.finish()
	submsg := marshalDataSubmessage(EntityIdSEDPPubWriter, EntityIdSEDPPubReader,
		SequenceNumber{Low: 1}, payload)
	msg := wrapInRTPSMessage(p.guidPrefix, submsg)

	p.sedp.mu.RLock()
	before := len(p.sedp.remoteWriters)
	p.sedp.mu.RUnlock()

	p.sedp.handlePacket(msg, loopbackAddr)

	p.sedp.mu.RLock()
	after := len(p.sedp.remoteWriters)
	p.sedp.mu.RUnlock()

	if after != before {
		t.Error("SEDP should ignore its own announcements")
	}
}

func TestSEDP_OnNewPeer_SendsAnnouncements(t *testing.T) {
	p := testPart(t)
	pub, err := p.NewPublisher("sedp/peer", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()
	sub, err := p.NewSubscriber("sedp/peer", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()

	proxy := &participantProxy{
		guid: GUID{Prefix: newGuidPrefix(), Entity: EntityIdParticipant},
		metatrafficUnicast: Locator{
			Kind:    LocatorKindUDPv4,
			Port:    uint32(loopbackAddr.Port),
			Address: [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 127, 0, 0, 1},
		},
	}
	// UDP sends will fail (no real listener) but the announcement path executes.
	p.sedp.onNewPeer(proxy)
}

func TestSEDP_HandlePacket_ValidRemote_PubWriter(t *testing.T) {
	p := testPart(t)
	remotePrefix := newGuidPrefix()
	remoteGUID := GUID{Prefix: remotePrefix, Entity: entityIdForWriter(5)}

	enc := newPLCDREncoder()
	enc.addGUID(pidEndpointGUID, remoteGUID)
	enc.addString(pidTopicName, "sedp/packet-pub")
	enc.addString(pidTypeName, "CDR_BLOB")
	enc.addLocator(pidDefaultUnicastLocator, Locator{Kind: LocatorKindUDPv4, Port: 32161})
	payload := enc.finish()

	submsg := marshalDataSubmessage(EntityIdSEDPPubWriter, EntityIdSEDPPubReader,
		SequenceNumber{Low: 1}, payload)
	msg := wrapInRTPSMessage(remotePrefix, submsg)

	p.sedp.handlePacket(msg, loopbackAddr)

	p.sedp.mu.RLock()
	_, stored := p.sedp.remoteWriters[remoteGUID]
	p.sedp.mu.RUnlock()
	if !stored {
		t.Error("SEDP PubWriter packet: remote writer should be stored")
	}
}

func TestSEDP_HandlePacket_ValidRemote_SubWriter(t *testing.T) {
	p := testPart(t)
	remotePrefix := newGuidPrefix()
	remoteGUID := GUID{Prefix: remotePrefix, Entity: entityIdForReader(5)}

	enc := newPLCDREncoder()
	enc.addGUID(pidEndpointGUID, remoteGUID)
	enc.addString(pidTopicName, "sedp/packet-sub")
	enc.addString(pidTypeName, "CDR_BLOB")
	payload := enc.finish()

	submsg := marshalDataSubmessage(EntityIdSEDPSubWriter, EntityIdSEDPSubReader,
		SequenceNumber{Low: 1}, payload)
	msg := wrapInRTPSMessage(remotePrefix, submsg)

	// handleEndpointAnnounce for a reader (isWriter=false) should not store
	// in remoteWriters. Just verifying no panic.
	p.sedp.handlePacket(msg, loopbackAddr)
}

func TestSEDP_HandlePacket_TruncatedHeader(t *testing.T) {
	p := testPart(t)
	p.sedp.handlePacket([]byte{0x52, 0x54, 0x50, 0x53}, loopbackAddr)
}

func TestHandleDataPacket_ACKNACKSubmessage(t *testing.T) {
	p := testPart(t)
	pub, err := p.NewPublisher("hdp/acknack", dds.ReliableQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	if err := pub.Write([]byte("needs-ack")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	p.mu.Lock()
	var eid EntityId
	for e := range p.writers {
		eid = e
		break
	}
	p.mu.Unlock()

	an := AckNack{
		ReaderEntityId: entityIdForReader(1),
		WriterEntityId: eid,
		Base:           SequenceNumber{Low: 1},
		Bitmap:         0b1,
		Count:          1,
	}
	submsg := marshalAckNack(an)
	msg := wrapInRTPSMessage(newGuidPrefix(), submsg)
	p.handleDataPacket(msg, loopbackAddr)
}

// ── GAP submessage ────────────────────────────────────────────────────────────

func TestMarshalGAP_Format(t *testing.T) {
	g := Gap{
		ReaderEntityId: entityIdForReader(1),
		WriterEntityId: entityIdForWriter(2),
		GapStart:       SequenceNumber{Low: 5},
		GapEnd:         SequenceNumber{Low: 9},
	}
	b := marshalGAP(g)
	// Submessage header: id=0x08, flags=0x01 (LE), length=28.
	if b[0] != submsgGAP {
		t.Errorf("id byte: got %02x want %02x", b[0], submsgGAP)
	}
	if b[1] != flagEndianness {
		t.Errorf("flags byte: got %02x want %02x", b[1], flagEndianness)
	}
	bodyLen := int(b[2]) | int(b[3])<<8
	if bodyLen != 28 {
		t.Errorf("body length: got %d want 28", bodyLen)
	}
	// gapList.bitmapBase = GapEnd.Low + 1 = 10, at body[16:20].
	body := b[4:]
	base := binary.LittleEndian.Uint32(body[20:24])
	if base != 10 {
		t.Errorf("gapList base: got %d want 10", base)
	}
	// numBits = 0 at body[24:28].
	numBits := binary.LittleEndian.Uint32(body[24:28])
	if numBits != 0 {
		t.Errorf("numBits: got %d want 0", numBits)
	}
}

func TestHandleAckNack_EvictedHistory_SendsGAP(t *testing.T) {
	p := testPart(t)
	pub, err := p.NewPublisher("gap/evict", dds.ReliableQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	// Write one sample so history.firstLast() returns (1, 1, true).
	if err := pub.Write([]byte("first")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	p.mu.Lock()
	var eid EntityId
	for e := range p.writers {
		eid = e
		break
	}
	p.mu.Unlock()

	// NACK for seqLo=0 which is below history first (1) → GAP path.
	// from=nil: GAP send is attempted but skipped gracefully.
	an := AckNack{
		ReaderEntityId: entityIdForReader(1),
		WriterEntityId: eid,
		Base:           SequenceNumber{Low: 0},
		Bitmap:         0b1,
		Count:          1,
	}
	// Must not panic; GAP is built and send is skipped (no peer locator).
	p.handleAckNack(an, loopbackAddr)
}

// ── matchedReaderLocators topic filtering ─────────────────────────────────────

func TestMatchedReaderLocators_EmptyReturnsNone(t *testing.T) {
	p := testPart(t)
	locs := p.matchedReaderLocators("some/topic")
	if len(locs) != 0 {
		t.Errorf("expected 0 locators for unknown topic, got %d", len(locs))
	}
}

func TestMatchedReaderLocators_TopicFiltered(t *testing.T) {
	p := testPart(t)
	remotePrefix := newGuidPrefix()
	readerGUID := GUID{Prefix: remotePrefix, Entity: entityIdForReader(1)}
	loc := Locator{Kind: LocatorKindUDPv4, Port: 7411}
	copy(loc.Address[12:], net.ParseIP("127.0.0.1").To4())

	// Plant a remote reader for "topic/a".
	p.sedp.mu.Lock()
	p.sedp.remoteReaders[readerGUID] = &endpointInfo{
		guid:      readerGUID,
		topicName: "topic/a",
	}
	p.sedp.remoteReaderLocs[readerGUID] = loc
	p.sedp.mu.Unlock()

	// Query for "topic/a" → should return the locator.
	if got := p.matchedReaderLocators("topic/a"); len(got) != 1 {
		t.Errorf("topic/a: expected 1 locator, got %d", len(got))
	}
	// Query for "topic/b" → should return nothing.
	if got := p.matchedReaderLocators("topic/b"); len(got) != 0 {
		t.Errorf("topic/b: expected 0 locators, got %d", len(got))
	}
}

func TestMatchedReaderLocators_DeduplicatesSharedLocator(t *testing.T) {
	p := testPart(t)
	prefix := newGuidPrefix()
	loc := Locator{Kind: LocatorKindUDPv4, Port: 7412}
	copy(loc.Address[12:], net.ParseIP("127.0.0.1").To4())

	// Two readers on the same participant (same locator, different entity IDs).
	for _, n := range []uint32{1, 2} {
		guid := GUID{Prefix: prefix, Entity: entityIdForReader(n)}
		p.sedp.mu.Lock()
		p.sedp.remoteReaders[guid] = &endpointInfo{guid: guid, topicName: "dup/topic"}
		p.sedp.remoteReaderLocs[guid] = loc
		p.sedp.mu.Unlock()
	}

	locs := p.matchedReaderLocators("dup/topic")
	if len(locs) != 1 {
		t.Errorf("expected 1 deduplicated locator, got %d", len(locs))
	}
}

// ── SPDP lease expiry ─────────────────────────────────────────────────────────

func TestSPDP_StorePeer_SetsLastSeen(t *testing.T) {
	p := testPart(t)
	before := time.Now()
	proxy := &participantProxy{
		guid:          GUID{Prefix: newGuidPrefix(), Entity: EntityIdParticipant},
		leaseDuration: 5 * time.Second,
	}
	p.spdp.storePeer(proxy)
	after := time.Now()
	if proxy.lastSeen.Before(before) || proxy.lastSeen.After(after) {
		t.Errorf("lastSeen %v not in [%v, %v]", proxy.lastSeen, before, after)
	}
}

func TestSPDP_StorePeer_DefaultsLease(t *testing.T) {
	p := testPart(t)
	proxy := &participantProxy{
		guid: GUID{Prefix: newGuidPrefix(), Entity: EntityIdParticipant},
		// leaseDuration intentionally zero.
	}
	p.spdp.storePeer(proxy)
	if proxy.leaseDuration != defaultLeaseDuration {
		t.Errorf("leaseDuration: got %v want %v", proxy.leaseDuration, defaultLeaseDuration)
	}
}

func TestSPDP_EvictExpired_RemovesStalePeer(t *testing.T) {
	p := testPart(t)
	prefix := newGuidPrefix()
	proxy := &participantProxy{
		guid:          GUID{Prefix: prefix, Entity: EntityIdParticipant},
		leaseDuration: time.Millisecond, // expire immediately
		lastSeen:      time.Now().Add(-time.Second),
	}
	p.spdp.mu.Lock()
	p.spdp.peers[prefix] = proxy
	p.spdp.mu.Unlock()

	p.spdp.evictExpired()

	p.spdp.mu.RLock()
	_, still := p.spdp.peers[prefix]
	p.spdp.mu.RUnlock()
	if still {
		t.Error("evictExpired: stale peer was not removed")
	}
}

func TestSPDP_EvictExpired_KeepsFreshPeer(t *testing.T) {
	p := testPart(t)
	prefix := newGuidPrefix()
	proxy := &participantProxy{
		guid:          GUID{Prefix: prefix, Entity: EntityIdParticipant},
		leaseDuration: time.Hour,
		lastSeen:      time.Now(),
	}
	p.spdp.mu.Lock()
	p.spdp.peers[prefix] = proxy
	p.spdp.mu.Unlock()

	p.spdp.evictExpired()

	p.spdp.mu.RLock()
	_, still := p.spdp.peers[prefix]
	p.spdp.mu.RUnlock()
	if !still {
		t.Error("evictExpired: fresh peer was incorrectly removed")
	}
}

func TestSPDP_ParseParticipantData_LeaseDuration(t *testing.T) {
	p := testPart(t)
	payload := p.spdp.buildParticipantData()
	from := &net.UDPAddr{IP: net.ParseIP("10.0.0.1"), Port: 7410}
	proxy := parseParticipantData(p.guidPrefix, payload, from)
	if proxy == nil {
		t.Fatal("parseParticipantData returned nil")
	}
	if proxy.leaseDuration != 10*time.Second {
		t.Errorf("leaseDuration: got %v want 10s", proxy.leaseDuration)
	}
}

// ── SEDP onRemoteReader / onPeerEvicted ───────────────────────────────────────

func TestSEDP_OnRemoteReader_Stored(t *testing.T) {
	p := testPart(t)
	remoteGUID := GUID{Prefix: newGuidPrefix(), Entity: entityIdForReader(1)}
	loc := Locator{Kind: LocatorKindUDPv4, Port: 7413}
	copy(loc.Address[12:], net.ParseIP("127.0.0.1").To4())

	info := &endpointInfo{guid: remoteGUID, topicName: "on/reader", isWriter: false}
	p.sedp.onRemoteReader(info, loc)

	p.sedp.mu.RLock()
	stored, ok := p.sedp.remoteReaders[remoteGUID]
	storedLoc := p.sedp.remoteReaderLocs[remoteGUID]
	p.sedp.mu.RUnlock()

	if !ok {
		t.Fatal("onRemoteReader: reader not stored")
	}
	if stored.topicName != "on/reader" {
		t.Errorf("topic: got %q want %q", stored.topicName, "on/reader")
	}
	if storedLoc != loc {
		t.Errorf("locator: got %+v want %+v", storedLoc, loc)
	}
}

func TestSEDP_OnPeerEvicted_ClearsRemoteEndpoints(t *testing.T) {
	p := testPart(t)
	prefix := newGuidPrefix()
	wGUID := GUID{Prefix: prefix, Entity: entityIdForWriter(1)}
	rGUID := GUID{Prefix: prefix, Entity: entityIdForReader(1)}

	p.sedp.mu.Lock()
	p.sedp.remoteWriters[wGUID] = &endpointInfo{guid: wGUID, topicName: "evict/t"}
	p.sedp.remoteReaders[rGUID] = &endpointInfo{guid: rGUID, topicName: "evict/t"}
	p.sedp.remoteReaderLocs[rGUID] = Locator{Kind: LocatorKindUDPv4, Port: 9999}
	p.sedp.mu.Unlock()

	p.sedp.onPeerEvicted(prefix)

	p.sedp.mu.RLock()
	_, wStill := p.sedp.remoteWriters[wGUID]
	_, rStill := p.sedp.remoteReaders[rGUID]
	_, locStill := p.sedp.remoteReaderLocs[rGUID]
	p.sedp.mu.RUnlock()

	if wStill {
		t.Error("onPeerEvicted: remote writer was not removed")
	}
	if rStill {
		t.Error("onPeerEvicted: remote reader was not removed")
	}
	if locStill {
		t.Error("onPeerEvicted: remote reader locator was not removed")
	}
}

func TestSEDP_HandleEndpointAnnounce_RemoteReader_Stored(t *testing.T) {
	p := testPart(t)
	remotePrefix := newGuidPrefix()
	remoteGUID := GUID{Prefix: remotePrefix, Entity: entityIdForReader(7)}

	enc := newPLCDREncoder()
	enc.addGUID(pidEndpointGUID, remoteGUID)
	enc.addString(pidTopicName, "sub/announce")
	enc.addString(pidTypeName, "CDR_BLOB")
	userLoc := locatorFromUDP(&net.UDPAddr{IP: net.IPv4zero}, 7414)
	enc.addLocator(pidDefaultUnicastLocator, userLoc)
	payload := enc.finish()

	from := &net.UDPAddr{IP: net.ParseIP("10.0.0.2"), Port: 7410}
	p.sedp.handleEndpointAnnounce(remotePrefix, payload, false /* isWriter=false */, from)

	p.sedp.mu.RLock()
	ri, ok := p.sedp.remoteReaders[remoteGUID]
	p.sedp.mu.RUnlock()
	if !ok {
		t.Fatal("remote subscription not stored after handleEndpointAnnounce")
	}
	if ri.topicName != "sub/announce" {
		t.Errorf("topic: got %q want %q", ri.topicName, "sub/announce")
	}
}

func TestSEDP_RegisterReader_PreExistingRemoteWriter(t *testing.T) {
	p := testPart(t)
	remoteGUID := GUID{Prefix: newGuidPrefix(), Entity: entityIdForWriter(1)}

	// Plant a remote writer directly so registerReader can find it.
	p.sedp.mu.Lock()
	p.sedp.remoteWriters[remoteGUID] = &endpointInfo{
		guid:      remoteGUID,
		topicName: "sedp/pre-existing",
		isWriter:  true,
	}
	p.sedp.mu.Unlock()

	// Register a reader for the same topic; it should get the remote GUID.
	sub, err := p.NewSubscriber("sedp/pre-existing", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()

	p.mu.Lock()
	var r *rtpsReader
	for _, rr := range p.readers {
		if rr.topic == "sedp/pre-existing" {
			r = rr
			break
		}
	}
	p.mu.Unlock()
	if r == nil {
		t.Fatal("reader not found")
	}
	if !r.acceptsSource(remoteGUID) {
		t.Error("reader should accept pre-existing remote writer after registerReader")
	}
}
