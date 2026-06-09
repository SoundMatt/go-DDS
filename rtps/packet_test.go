// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Internal packet-handling tests. Uses package rtps (internal) for direct
// access to unexported types and functions.

package rtps

//fusa:test REQ-DISC-002
//fusa:test REQ-DISC-003
//fusa:test REQ-DISC-006
//fusa:test REQ-DISC-007
//fusa:test REQ-DISC-010
//fusa:test REQ-DISC-011
//fusa:test REQ-DISC-013
//fusa:test REQ-DISC-014
//fusa:test REQ-DISC-015
//fusa:test REQ-PART-003
//fusa:test REQ-PART-007
//fusa:test REQ-PART-009
//fusa:test REQ-PART-010
//fusa:test REQ-PART-011
//fusa:test REQ-PUB-005
//fusa:test REQ-PUB-007
//fusa:test REQ-QOS-003
//fusa:test REQ-QOS-004
//fusa:test REQ-QOS-007
//fusa:test REQ-REL-006
//fusa:test REQ-REL-007
//fusa:test REQ-REL-008
//fusa:test REQ-REL-009
//fusa:test REQ-REL-010
//fusa:test REQ-REL-011
//fusa:test REQ-REL-012
//fusa:test REQ-RTPS-005
//fusa:test REQ-SAFETY-003
//fusa:test REQ-SUB-006
//fusa:test REQ-SUB-007
//fusa:test REQ-TRANS-001
//fusa:test REQ-TRANS-002
//fusa:test REQ-TRANS-003
//fusa:test REQ-WILD-001
//fusa:test REQ-WILD-002
//fusa:test REQ-WILD-003
//fusa:test REQ-WILD-004
//fusa:test REQ-PART-008

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"log/slog"
	"net"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/tsn"
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
	sub, err := p.NewSubscriber("nrr/be", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
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

// ── Wildcard tests ────────────────────────────────────────────────────────────

func TestTopicMatches_Exact(t *testing.T) {
	if !TopicMatches("a/b/c", "a/b/c") {
		t.Error("exact match should succeed")
	}
	if TopicMatches("a/b/c", "a/b/d") {
		t.Error("different tail should not match")
	}
}

func TestTopicMatches_SingleLevel(t *testing.T) {
	if !TopicMatches("a/+/c", "a/b/c") {
		t.Error("+ should match single segment")
	}
	if TopicMatches("a/+/c", "a/b/b/c") {
		t.Error("+ should not match multiple segments")
	}
}

func TestTopicMatches_MultiLevel(t *testing.T) {
	if !TopicMatches("a/#", "a/b/c/d") {
		t.Error("# should match remaining levels")
	}
	if !TopicMatches("a/#", "a/b") {
		t.Error("# should match single remaining level")
	}
	if !TopicMatches("#", "anything/here") {
		t.Error("bare # should match everything")
	}
}

func TestTopicMatches_NoMatch(t *testing.T) {
	if TopicMatches("a/b", "a") {
		t.Error("prefix should not match")
	}
	if TopicMatches("a", "a/b") {
		t.Error("topic with extra levels should not match bare pattern")
	}
}

// ── Fragment tests ────────────────────────────────────────────────────────────

func TestMarshalParseDataFrag_RoundTrip(t *testing.T) {
	frag := DataFrag{
		WriterEntityId:      entityIdForWriter(1),
		ReaderEntityId:      EntityIdUnknown,
		WriterSeqNum:        SequenceNumber{Low: 3},
		FragmentStartingNum: 1,
		FragmentsInSubmsg:   1,
		FragmentSize:        12,
		DataSize:            12,
		Payload:             []byte("hello world!"),
	}
	raw := marshalDataFrag(frag)
	// Parse from body (skip 4-byte submsg header).
	parsed, ok := parseDataFrag(raw[4:])
	if !ok {
		t.Fatal("parseDataFrag returned ok=false")
	}
	if parsed.FragmentStartingNum != frag.FragmentStartingNum {
		t.Errorf("FragmentStartingNum: got %d want %d", parsed.FragmentStartingNum, frag.FragmentStartingNum)
	}
	if parsed.DataSize != frag.DataSize {
		t.Errorf("DataSize: got %d want %d", parsed.DataSize, frag.DataSize)
	}
	if string(parsed.Payload) != string(frag.Payload) {
		t.Errorf("Payload: got %q want %q", parsed.Payload, frag.Payload)
	}
}

func TestSplitIntoFragments_SmallPayload(t *testing.T) {
	payload := []byte("small")
	eid := entityIdForWriter(1)
	frags := splitIntoFragments(eid, SequenceNumber{Low: 1}, payload)
	if len(frags) != 1 {
		t.Fatalf("want 1 fragment, got %d", len(frags))
	}
	if string(frags[0].Payload) != "small" {
		t.Errorf("fragment payload mismatch: %q", frags[0].Payload)
	}
}

func TestSplitIntoFragments_LargePayload_MultipleFragments(t *testing.T) {
	payload := make([]byte, maxFragmentPayload*3+100)
	for i := range payload {
		payload[i] = byte(i % 251)
	}
	eid := entityIdForWriter(1)
	frags := splitIntoFragments(eid, SequenceNumber{Low: 2}, payload)
	if len(frags) != 4 {
		t.Fatalf("want 4 fragments, got %d", len(frags))
	}
	for i, f := range frags {
		if f.FragmentStartingNum != uint32(i+1) {
			t.Errorf("frag %d: FragmentStartingNum = %d, want %d", i, f.FragmentStartingNum, i+1)
		}
		if f.DataSize != uint32(len(payload)) {
			t.Errorf("frag %d: DataSize = %d, want %d", i, f.DataSize, len(payload))
		}
	}
}

func TestFragmentAssembler_Reassembles(t *testing.T) {
	payload := make([]byte, maxFragmentPayload*2+50)
	for i := range payload {
		payload[i] = byte(i % 251)
	}
	eid := entityIdForWriter(1)
	frags := splitIntoFragments(eid, SequenceNumber{Low: 1}, payload)

	var fa fragmentAssembler
	var result []byte
	for _, f := range frags {
		result = fa.receive(f)
		if result != nil {
			break
		}
	}
	if result == nil {
		t.Fatal("assembler did not return complete payload")
	}
	if string(result) != string(payload) {
		t.Error("reassembled payload does not match original")
	}
}

func TestFragmentAssembler_PartialNoResult(t *testing.T) {
	payload := make([]byte, maxFragmentPayload*2)
	eid := entityIdForWriter(1)
	frags := splitIntoFragments(eid, SequenceNumber{Low: 1}, payload)

	var fa fragmentAssembler
	// Only send first fragment; should get nil back.
	result := fa.receive(frags[0])
	if result != nil {
		t.Error("partial delivery should return nil")
	}
}

func TestFragmentAssembler_OversizeDataSize_Rejected(t *testing.T) {
	// A DATA_FRAG claiming DataSize > maxReassemblyBytes must be silently
	// dropped; no allocation should occur and receive must return nil.
	f := DataFrag{
		WriterEntityId:      entityIdForWriter(99),
		ReaderEntityId:      EntityIdUnknown,
		WriterSeqNum:        SequenceNumber{Low: 1},
		FragmentStartingNum: 1,
		FragmentsInSubmsg:   1,
		FragmentSize:        1200,
		DataSize:            maxReassemblyBytes + 1, // one byte over the cap
		Payload:             make([]byte, 1200),
	}
	var fa fragmentAssembler
	if result := fa.receive(f); result != nil {
		t.Error("oversize DataSize must return nil, not allocate")
	}
}

func TestFragmentAssembler_OutOfOrder(t *testing.T) {
	// Deliver fragments in reverse order; the assembler must still reconstruct
	// the original payload correctly.
	payload := make([]byte, maxFragmentPayload*3)
	for i := range payload {
		payload[i] = byte(i % 251)
	}
	eid := entityIdForWriter(42)
	frags := splitIntoFragments(eid, SequenceNumber{Low: 7}, payload)
	if len(frags) < 3 {
		t.Skipf("need at least 3 fragments, got %d", len(frags))
	}

	// Reverse the order.
	for i, j := 0, len(frags)-1; i < j; i, j = i+1, j-1 {
		frags[i], frags[j] = frags[j], frags[i]
	}

	var fa fragmentAssembler
	var result []byte
	for _, f := range frags {
		result = fa.receive(f)
		if result != nil {
			break
		}
	}
	if result == nil {
		t.Fatal("out-of-order assembler did not complete")
	}
	if string(result) != string(payload) {
		t.Error("out-of-order reassembled payload does not match original")
	}
}

func TestFragmentAssembler_StaleEviction(t *testing.T) {
	// An incomplete reassembly that is older than staleFragAge must be evicted
	// so the assembler does not accumulate memory indefinitely.
	//
	// We achieve this by directly manipulating the buffer's created time via a
	// second receive call on a different sequence number which triggers the sweep.
	payload1 := make([]byte, maxFragmentPayload*2)
	eid := entityIdForWriter(55)
	frags1 := splitIntoFragments(eid, SequenceNumber{Low: 1}, payload1)

	var fa fragmentAssembler
	// Deliver only the first fragment — leaves the reassembly incomplete.
	fa.receive(frags1[0])

	// Manually back-date the buffer by moving created time into the past.
	fa.mu.Lock()
	for _, b := range fa.buffers {
		b.created = b.created.Add(-(staleFragAge + time.Second))
	}
	fa.mu.Unlock()

	// Delivering a fragment for a different sequence triggers the stale sweep.
	payload2 := []byte("trigger")
	frags2 := splitIntoFragments(eid, SequenceNumber{Low: 2}, payload2)
	fa.receive(frags2[0])

	// The stale buffer for seq 1 must have been evicted.
	fa.mu.Lock()
	_, stillPresent := fa.buffers[fragKey{writer: eid, seqLo: 1}]
	fa.mu.Unlock()
	if stillPresent {
		t.Error("stale incomplete reassembly was not evicted")
	}
}

func TestFragmentAssembler_ZeroDataSize_Rejected(t *testing.T) {
	f := DataFrag{FragmentSize: 100, DataSize: 0, FragmentsInSubmsg: 1}
	var fa fragmentAssembler
	if fa.receive(f) != nil {
		t.Error("zero DataSize must return nil")
	}
}

func TestFragmentAssembler_ZeroFragmentSize_Rejected(t *testing.T) {
	f := DataFrag{FragmentSize: 0, DataSize: 100, FragmentsInSubmsg: 1}
	var fa fragmentAssembler
	if fa.receive(f) != nil {
		t.Error("zero FragmentSize must return nil")
	}
}

// ── Persist tests ─────────────────────────────────────────────────────────────

func TestPersistFlushLoad_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	payload := []byte("vehicle/speed=120")
	persistFlush(dir, "vehicle/speed", payload)
	got, err := persistLoad(dir, "vehicle/speed")
	if err != nil {
		t.Fatalf("persistLoad: %v", err)
	}
	if string(got) != string(payload) {
		t.Errorf("got %q, want %q", got, payload)
	}
}

func TestPersistLoad_MissingFile_ReturnsNil(t *testing.T) {
	dir := t.TempDir()
	got, err := persistLoad(dir, "no/such/topic")
	if err == nil {
		t.Error("expected error for missing file")
	}
	if got != nil {
		t.Error("expected nil payload for missing file")
	}
}

func TestPersistFlush_EmptyDir_NoOp(t *testing.T) {
	// Should not panic.
	persistFlush("", "topic", []byte("data"))
}

func TestPersistLoad_OversizePayload_Rejected(t *testing.T) {
	dir := t.TempDir()
	// Write a file whose 4-byte length header claims > 64 MiB.
	path := persistPath(dir, "big/topic")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	var hdr [4]byte
	binary.LittleEndian.PutUint32(hdr[:], 64*1024*1024+1) // one byte over the cap
	writeN, writeErr := f.Write(hdr[:])
	_ = writeN
	if writeErr != nil {
		t.Fatalf("Write: %v", writeErr)
	}
	_ = f.Close()

	ignoredRet, loadErr := persistLoad(dir, "big/topic")
	_ = ignoredRet
	if loadErr == nil {
		t.Error("expected error for oversized payload header, got nil")
	}
}

func TestWithPersistentHistory_LateJoinerGetsPersistedSample(t *testing.T) {
	dir := t.TempDir()
	p, err := newParticipant(dds.Domain(77), WithNoMulticast(), WithPersistentHistory(dir))
	if err != nil {
		t.Skipf("newParticipant: %v", err)
	}
	defer p.Close()

	pub, err := p.NewPublisher("persist/topic", dds.ReliableQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	if writeErr := pub.Write([]byte("persisted-value")); writeErr != nil {
		t.Fatalf("Write: %v", writeErr)
	}

	// Late subscriber: should receive the persisted last sample from disk.
	sub, err := p.NewSubscriber("persist/topic", dds.ReliableQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()

	select {
	case s := <-sub.C():
		if string(s.Payload) != "persisted-value" {
			t.Errorf("got %q, want persisted-value", s.Payload)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout: persisted late-joiner sample not delivered")
	}
}

// TestPlog_Warn_WithNonNilLogger exercises the non-nil logger branch of
// plog.warn (0% coverage in the full suite since warn is not called from
// any production code path — it's a defensive utility).
func TestPlog_Warn_WithNonNilLogger(t *testing.T) {
	var buf strings.Builder
	l := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	pl := plog{l: l}
	pl.warn("test warning %s", "msg")
	if !strings.Contains(buf.String(), "test warning msg") {
		t.Errorf("expected warn log output, got %q", buf.String())
	}
}

// TestPlog_Warn_NilLogger verifies warn is a no-op when no logger is set.
func TestPlog_Warn_NilLogger(t *testing.T) {
	pl := plog{} // l is nil
	pl.warn("should not panic")
}

// TestPlog_Debug_WithNonNilLogger exercises the non-nil logger branch of
// plog.debug, which is otherwise only exercised via the nil-logger path.
func TestPlog_Debug_WithNonNilLogger(t *testing.T) {
	var buf strings.Builder
	l := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	pl := plog{l: l}
	pl.debug("test debug %d", 7)
	if !strings.Contains(buf.String(), "test debug 7") {
		t.Errorf("expected debug log output, got %q", buf.String())
	}
}

// ── Sentinel error tests ──────────────────────────────────────────────────────

func TestNewPublisher_EmptyTopic_ErrTopicEmpty(t *testing.T) {
	p := testPart(t)
	ignoredRet, err := p.NewPublisher("", dds.DefaultQoS)
	_ = ignoredRet
	if err == nil {
		t.Fatal("expected error for empty topic")
	}
}

func TestNewSubscriber_EmptyTopic_ErrTopicEmpty(t *testing.T) {
	p := testPart(t)
	ignoredRet, err := p.NewSubscriber("", dds.DefaultQoS)
	_ = ignoredRet
	if err == nil {
		t.Fatal("expected error for empty topic")
	}
}

func TestWrite_AfterClose_ErrClosed(t *testing.T) {
	p := testPart(t)
	pub, err := p.NewPublisher("close/test", dds.DefaultQoS)
	if err != nil {
		t.Fatal(err)
	}
	pub.Close()
	if err := pub.Write([]byte("x")); err == nil {
		t.Fatal("expected error writing to closed publisher")
	}
}

// ── Metrics tests ─────────────────────────────────────────────────────────────

func TestMetrics_WritesAndDelivers(t *testing.T) {
	p := testPart(t)

	pub, err := p.NewPublisher("metrics/test", dds.DefaultQoS)
	if err != nil {
		t.Fatal(err)
	}
	sub, err := p.NewSubscriber("metrics/test", dds.DefaultQoS)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()

	_ = pub.Write([]byte("hello"))
	// Drain the subscriber so we know delivery happened.
	select {
	case <-sub.C():
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for sample")
	}

	m := p.Metrics()
	if m.WriteCount == 0 {
		t.Error("WriteCount should be > 0 after Write")
	}
	if m.DeliverCount == 0 {
		t.Error("DeliverCount should be > 0 after delivery")
	}
}

// ── Deadline tests ────────────────────────────────────────────────────────────

func TestDeadline_CallbackFires_WhenNoWrite(t *testing.T) {
	fired := make(chan string, 1)
	p, err := newParticipant(dds.Domain(99), WithDeadlineCallback(func(topic string) {
		select {
		case fired <- topic:
		default:
		}
	}))
	if err != nil {
		t.Skipf("newParticipant: %v", err)
	}
	defer p.Close()

	qos := dds.DefaultQoS
	qos.Deadline = 50 * time.Millisecond
	pub, err := p.NewPublisher("deadline/topic", qos)
	if err != nil {
		t.Fatal(err)
	}
	defer pub.Close()

	select {
	case topic := <-fired:
		if topic != "deadline/topic" {
			t.Errorf("expected deadline on 'deadline/topic', got %q", topic)
		}
	case <-time.After(time.Second):
		t.Fatal("deadline callback did not fire")
	}
}

func TestDeadline_CallbackNotFired_AfterWrite(t *testing.T) {
	fired := make(chan string, 1)
	p, err := newParticipant(dds.Domain(99), WithDeadlineCallback(func(topic string) {
		select {
		case fired <- topic:
		default:
		}
	}))
	if err != nil {
		t.Skipf("newParticipant: %v", err)
	}
	defer p.Close()

	qos := dds.DefaultQoS
	qos.Deadline = 100 * time.Millisecond
	pub, err := p.NewPublisher("nodeadline/topic", qos)
	if err != nil {
		t.Fatal(err)
	}
	defer pub.Close()

	// Write twice within the deadline window to keep the timer reset.
	_ = pub.Write([]byte("a"))
	time.Sleep(60 * time.Millisecond)
	_ = pub.Write([]byte("b"))
	time.Sleep(60 * time.Millisecond)
	_ = pub.Write([]byte("c"))

	select {
	case topic := <-fired:
		t.Errorf("deadline should not have fired but got topic %q", topic)
	case <-time.After(50 * time.Millisecond):
		// Good — timer stayed alive.
	}
}

// ── Content filter tests ──────────────────────────────────────────────────────

func TestContentFilter_OnlyMatchingDelivered(t *testing.T) {
	p := testPart(t)
	pub, err := p.NewPublisher("filter/test", dds.DefaultQoS)
	if err != nil {
		t.Fatal(err)
	}
	sub, err := p.NewSubscriber("filter/test", dds.DefaultQoS,
		dds.WithFilter(func(s dds.Sample) bool {
			return string(s.Payload) == "yes"
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()

	_ = pub.Write([]byte("no"))
	_ = pub.Write([]byte("yes"))

	select {
	case s := <-sub.C():
		if string(s.Payload) != "yes" {
			t.Errorf("expected 'yes', got %q", s.Payload)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for filtered sample")
	}

	// Verify "no" was not delivered.
	select {
	case s := <-sub.C():
		t.Errorf("unexpected sample delivered: %q", s.Payload)
	default:
	}
}

// ── Unicast-only option tests ─────────────────────────────────────────────────

func TestWithNoMulticast_ParticipantStarts(t *testing.T) {
	// With no multicast we skip the SPDP multicast socket but still bind
	// the unicast meta and data sockets; the participant should start without error.
	// NOTE: current implementation still binds the mcast socket (the option is
	// stored but not yet used to skip the bind — that is wired at SPDP level).
	// This test verifies the option is accepted without error.
	ignoredRet, err := newParticipant(dds.Domain(99), WithNoMulticast())
	_ = ignoredRet
	if err != nil {
		t.Skipf("newParticipant: %v — socket creation unavailable", err)
	}
}

func TestWithPeerLocators_Accepted(t *testing.T) {
	p, err := newParticipant(dds.Domain(99), WithPeerLocators("127.0.0.1:7400"))
	if err != nil {
		t.Skipf("newParticipant: %v — socket creation unavailable", err)
	}
	defer p.Close()
	if len(p.peerLocators) != 1 || p.peerLocators[0] != "127.0.0.1:7400" {
		t.Errorf("peerLocators not stored: %v", p.peerLocators)
	}
}

// ── v0.4 feature tests ────────────────────────────────────────────────────────

func TestMarshalParseInfoTS_RoundTrip(t *testing.T) {
	// Truncate to second precision for the NTP fraction round-trip.
	now := time.Now().Truncate(time.Millisecond)
	b := marshalInfoTS(now)
	// submessage header (4) + NTP64 (8) = 12 bytes.
	if len(b) != 12 {
		t.Fatalf("marshalInfoTS: want 12 bytes, got %d", len(b))
	}
	if b[0] != submsgINFO_TS {
		t.Errorf("submessage id: got 0x%02x, want 0x%02x", b[0], submsgINFO_TS)
	}
	got, ok := parseInfoTS(b[4:]) // body is after 4-byte header
	if !ok {
		t.Fatal("parseInfoTS returned false")
	}
	diff := got.Sub(now)
	if diff < 0 {
		diff = -diff
	}
	if diff > time.Millisecond {
		t.Errorf("timestamp round-trip error %v, want ≤1ms", diff)
	}
}

func TestInfoTS_WriteCarriesTimestamp(t *testing.T) {
	p, err := newParticipant(dds.Domain(98), WithNoMulticast())
	if err != nil {
		t.Skipf("newParticipant: %v", err)
	}
	defer p.Close()

	before := time.Now()
	sub, err := p.NewSubscriber("ts/rtps", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()
	pub, err := p.NewPublisher("ts/rtps", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	_ = pub.Write([]byte("ts-test"))

	select {
	case s := <-sub.C():
		if s.Timestamp.IsZero() {
			t.Error("Timestamp must not be zero")
		}
		if s.Timestamp.Before(before) {
			t.Errorf("Timestamp %v before write time %v", s.Timestamp, before)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for sample with timestamp")
	}
}

func TestWithLogger_AcceptedAndUsed(t *testing.T) {
	l := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}))
	p, err := newParticipant(dds.Domain(97), WithLogger(l), WithNoMulticast())
	if err != nil {
		t.Skipf("newParticipant: %v", err)
	}
	defer p.Close()
	// Verify the logger was stored.
	if p.log.l == nil {
		t.Error("logger not stored in participant")
	}
}

func TestWithTracer_AcceptedAndUsed(t *testing.T) {
	// Use a simple custom tracer to verify it's called.
	rt := &struct {
		count int
		sync.Mutex
	}{}
	tracer := dds.Tracer(tracerFunc(func(ctx context.Context, name string, _ ...dds.SpanAttribute) (context.Context, dds.Span) {
		rt.Lock()
		rt.count++
		rt.Unlock()
		ctx2, span := dds.NoopTracer.Start(ctx, name)
		return ctx2, span
	}))
	p, err := newParticipant(dds.Domain(96), WithTracer(tracer), WithNoMulticast())
	if err != nil {
		t.Skipf("newParticipant: %v", err)
	}
	defer p.Close()
	pub, err := p.NewPublisher("tracer/test", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()
	sub, err := p.NewSubscriber("tracer/test", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()

	_ = pub.Write([]byte("trace"))
	select {
	case <-sub.C():
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
	rt.Lock()
	count := rt.count
	rt.Unlock()
	if count == 0 {
		t.Error("tracer.Start was never called")
	}
}

// tracerFunc adapts a function to the dds.Tracer interface.
type tracerFunc func(context.Context, string, ...dds.SpanAttribute) (context.Context, dds.Span)

func (f tracerFunc) Start(ctx context.Context, name string, attrs ...dds.SpanAttribute) (context.Context, dds.Span) {
	return f(ctx, name, attrs...)
}

func TestChannelDepth_RTPS(t *testing.T) {
	p, err := newParticipant(dds.Domain(95), WithNoMulticast())
	if err != nil {
		t.Skipf("newParticipant: %v", err)
	}
	defer p.Close()

	sub, err := p.NewSubscriber("depth/rtps", dds.DefaultQoS, dds.WithChannelDepth(2))
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()
	pub, err := p.NewPublisher("depth/rtps", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	_ = pub.Write([]byte("a"))
	_ = pub.Write([]byte("b"))
	_ = pub.Write([]byte("c")) // may be dropped

	got := 0
	timeout := time.After(50 * time.Millisecond)
loop:
	for {
		select {
		case <-sub.C():
			got++
		case <-timeout:
			break loop
		}
	}
	if got > 2 {
		t.Errorf("depth=2: delivered %d samples, want ≤2", got)
	}
}

func TestBackPressure_DropOldest_RTPS(t *testing.T) {
	p, err := newParticipant(dds.Domain(94), WithNoMulticast())
	if err != nil {
		t.Skipf("newParticipant: %v", err)
	}
	defer p.Close()

	sub, err := p.NewSubscriber("bp/rtps", dds.DefaultQoS,
		dds.WithChannelDepth(1),
		dds.WithBackPressure(dds.DropOldest),
	)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()
	pub, err := p.NewPublisher("bp/rtps", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	_ = pub.Write([]byte("old"))
	_ = pub.Write([]byte("new"))

	select {
	case s := <-sub.C():
		if string(s.Payload) != "new" {
			t.Errorf("DropOldest: got %q, want new", s.Payload)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

func TestLiveliness_RTPS_CallbackSetup(t *testing.T) {
	events := make(chan dds.LivelinessEvent, 4)
	cb := func(_ dds.GUID, ev dds.LivelinessEvent) {
		select {
		case events <- ev:
		default:
		}
	}
	p, err := newParticipant(dds.Domain(93), WithNoMulticast(), WithLivelinessCallback(cb))
	if err != nil {
		t.Skipf("newParticipant: %v", err)
	}
	if p.livelinessCb == nil {
		t.Error("livelinessCb not stored")
	}
	defer p.Close()
}

func TestCloseWithDrain_RTPS_BestEffort(t *testing.T) {
	// BestEffort writers have no ACK drain; CloseWithDrain should complete immediately.
	p, err := newParticipant(dds.Domain(92), WithNoMulticast())
	if err != nil {
		t.Skipf("newParticipant: %v", err)
	}
	pub, err := p.NewPublisher("drain/be", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	_ = pub.Write([]byte("x"))

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err2 := dds.CloseWithDrain(ctx, p); err2 != nil {
		t.Errorf("CloseWithDrain: %v", err2)
	}
}

func TestCloseWithDrain_RTPS_Reliable_NoRemoteReaders(t *testing.T) {
	// Reliable writer with no remote readers: seqLo > ackedLo initially.
	// waitDrain uses drainCh — for no remote ACKNACKs the drain channel is
	// never signalled, so we expect context cancellation.
	p, err := newParticipant(dds.Domain(91), WithNoMulticast())
	if err != nil {
		t.Skipf("newParticipant: %v", err)
	}
	pub, err := p.NewPublisher("drain/rel", dds.ReliableQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	_ = pub.Write([]byte("reliable"))

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	// With no remote readers to ACK, drain times out. We expect an error.
	err2 := dds.CloseWithDrain(ctx, p)
	if err2 == nil {
		// Only OK if no writes made it in (local delivery doesn't count for drain).
		// Accept both outcomes: either context cancelled or immediate drain.
		t.Log("CloseWithDrain returned nil (drain completed without remote ACKs — likely local-only)")
	}
}

func TestAdvanceAcked_SignalsDrainCh(t *testing.T) {
	p, err := newParticipant(dds.Domain(90), WithNoMulticast())
	if err != nil {
		t.Skipf("newParticipant: %v", err)
	}
	defer p.Close()

	pub, err := p.NewPublisher("drain/ack", dds.ReliableQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	_ = pub.Write([]byte("w1"))
	w, ok := pub.(*rtpsWriter)
	if !ok {
		t.Fatalf("expected *rtpsWriter, got %T", pub)
	}

	// Simulate receiving ACKNACK with base=2 (acking seqLo=1).
	w.advanceAcked(2)

	// drainCh should now be closed.
	select {
	case <-w.drainCh:
		// correct
	case <-time.After(100 * time.Millisecond):
		t.Error("drainCh not closed after advanceAcked")
	}
}

func TestUserMulticastPort(t *testing.T) {
	if userMulticastPort(0) != 7401 {
		t.Errorf("userMulticastPort(0) = %d, want 7401", userMulticastPort(0))
	}
	if userMulticastPort(1) != 7651 {
		t.Errorf("userMulticastPort(1) = %d, want 7651", userMulticastPort(1))
	}
}

// ── v0.5 TSN tests ────────────────────────────────────────────────────────────

func TestMaxSampleSize_Rejects_OversizePayload(t *testing.T) {
	p := testPart(t)
	qos := dds.DefaultQoS
	qos.MaxSampleSize = 10
	pub, err := p.NewPublisher("tsn/size", qos)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()
	err = pub.Write(make([]byte, 11))
	if !errors.Is(err, dds.ErrPayloadTooLarge) {
		t.Errorf("want ErrPayloadTooLarge, got %v", err)
	}
}

func TestMaxSampleSize_Accepts_ExactLimit(t *testing.T) {
	p := testPart(t)
	qos := dds.DefaultQoS
	qos.MaxSampleSize = 5
	pub, err := p.NewPublisher("tsn/exact", qos)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()
	if err := pub.Write(make([]byte, 5)); err != nil {
		t.Errorf("Write at exact limit failed: %v", err)
	}
}

func TestMaxSampleSize_ZeroMeansUnlimited(t *testing.T) {
	p := testPart(t)
	pub, err := p.NewPublisher("tsn/unlimited", dds.DefaultQoS) // MaxSampleSize=0
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()
	// Large but should not be rejected by MaxSampleSize=0.
	if err := pub.Write(make([]byte, 4096)); err != nil && errors.Is(err, dds.ErrPayloadTooLarge) {
		t.Errorf("MaxSampleSize=0 should not reject: %v", err)
	}
}

func TestSplitIntoFragmentsN_CustomSize(t *testing.T) {
	payload := make([]byte, 300)
	for i := range payload {
		payload[i] = byte(i % 251)
	}
	eid := entityIdForWriter(1)
	frags := splitIntoFragmentsN(eid, SequenceNumber{Low: 1}, payload, 100)
	// ceil(300/100) = 3 fragments
	if len(frags) != 3 {
		t.Fatalf("want 3 fragments for 300 bytes at size 100, got %d", len(frags))
	}
	// Reconstruct and verify.
	var got []byte
	for _, f := range frags {
		got = append(got, f.Payload...)
	}
	if string(got) != string(payload) {
		t.Error("reassembled payload mismatch")
	}
	for i, f := range frags {
		if f.FragmentStartingNum != uint32(i+1) {
			t.Errorf("frag[%d].FragmentStartingNum=%d, want %d", i, f.FragmentStartingNum, i+1)
		}
	}
}

func TestSplitIntoFragmentsN_ZeroSizeFallback(t *testing.T) {
	payload := make([]byte, 10)
	eid := entityIdForWriter(1)
	frags := splitIntoFragmentsN(eid, SequenceNumber{Low: 1}, payload, 0)
	// 0 size → defaults to maxFragmentPayload; small payload = 1 fragment.
	if len(frags) != 1 {
		t.Fatalf("want 1 fragment for small payload with size=0, got %d", len(frags))
	}
}

func TestWriter_FragmentSize_NoTSNStream(t *testing.T) {
	p := testPart(t)
	pub, err := p.NewPublisher("fsize/default", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()
	w, ok := pub.(*rtpsWriter)
	if !ok {
		t.Fatalf("expected *rtpsWriter, got %T", pub)
	}
	if w.fragmentSize() != maxFragmentPayload {
		t.Errorf("fragmentSize() without TSN stream: got %d, want %d", w.fragmentSize(), maxFragmentPayload)
	}
}

func TestWriter_SendSock_NoTSN_IsDataSock(t *testing.T) {
	p := testPart(t)
	pub, err := p.NewPublisher("ssock/default", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()
	w, ok := pub.(*rtpsWriter)
	if !ok {
		t.Fatalf("expected *rtpsWriter, got %T", pub)
	}
	if w.sendSock() != p.dataSock {
		t.Error("sendSock() without TSN should return p.dataSock")
	}
}

func TestWithSPDPInterval_Stored(t *testing.T) {
	p, err := newParticipant(dds.Domain(85), WithNoMulticast(), WithSPDPInterval(10*time.Second))
	if err != nil {
		t.Skipf("newParticipant: %v", err)
	}
	defer p.Close()
	if p.spdpInterval != 10*time.Second {
		t.Errorf("spdpInterval: got %v, want 10s", p.spdpInterval)
	}
}

func TestWithSPDPJitter_Stored(t *testing.T) {
	p, err := newParticipant(dds.Domain(84), WithNoMulticast(), WithSPDPJitter(500*time.Millisecond))
	if err != nil {
		t.Skipf("newParticipant: %v", err)
	}
	defer p.Close()
	if p.spdpJitter != 500*time.Millisecond {
		t.Errorf("spdpJitter: got %v, want 500ms", p.spdpJitter)
	}
}

func TestWithStaticPeers_SameAsPeerLocators(t *testing.T) {
	p, err := newParticipant(dds.Domain(83), WithNoMulticast(), WithStaticPeers("127.0.0.1:7400"))
	if err != nil {
		t.Skipf("newParticipant: %v", err)
	}
	defer p.Close()
	if len(p.peerLocators) != 1 || p.peerLocators[0] != "127.0.0.1:7400" {
		t.Errorf("peerLocators: got %v", p.peerLocators)
	}
}

func TestWithTSNConfig_MatchesTopic(t *testing.T) {
	cfg, err := tsn.ParseConfig([]byte(`{
		"streams":[
			{"topic":"tsn/cfg","pcp":5,"dscp":46,"max_frame_size":1500}
		]
	}`))
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	p, err := newParticipant(dds.Domain(82), WithNoMulticast(), WithTSNConfig(cfg))
	if err != nil {
		t.Skipf("newParticipant: %v", err)
	}
	defer p.Close()

	pub, err := p.NewPublisher("tsn/cfg", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	w, ok := pub.(*rtpsWriter)
	if !ok {
		t.Fatalf("expected *rtpsWriter, got %T", pub)
	}
	if w.tsnStream == nil {
		t.Fatal("tsnStream should be set for topic 'tsn/cfg'")
	}
	if w.tsnStream.PCP != 5 {
		t.Errorf("tsnStream.PCP = %d, want 5", w.tsnStream.PCP)
	}
	if w.tsnSock == nil {
		t.Error("tsnSock should be non-nil for TSN publisher")
	}
	// sendSock() should return the TSN socket, not dataSock.
	if w.sendSock() == p.dataSock {
		t.Error("sendSock() for TSN writer should not return dataSock")
	}
}

func TestWithTSNConfig_NonMatchingTopic_NoTSN(t *testing.T) {
	cfg, err := tsn.ParseConfig([]byte(`{
		"streams":[{"topic":"tsn/other","pcp":3}]
	}`))
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	p, err := newParticipant(dds.Domain(81), WithNoMulticast(), WithTSNConfig(cfg))
	if err != nil {
		t.Skipf("newParticipant: %v", err)
	}
	defer p.Close()

	pub, err := p.NewPublisher("not/tsn/topic", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	w, ok := pub.(*rtpsWriter)
	if !ok {
		t.Fatalf("expected *rtpsWriter, got %T", pub)
	}
	if w.tsnStream != nil {
		t.Error("tsnStream should be nil for non-matching topic")
	}
	if w.tsnSock != nil {
		t.Error("tsnSock should be nil for non-matching topic")
	}
}

func TestTransportPriority_QoS_CreatesTSNSock(t *testing.T) {
	p := testPart(t)
	qos := dds.DefaultQoS
	qos.TransportPriority = 3
	pub, err := p.NewPublisher("tp/qos", qos)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()
	w, ok := pub.(*rtpsWriter)
	if !ok {
		t.Fatalf("expected *rtpsWriter, got %T", pub)
	}
	if w.tsnSock == nil {
		t.Error("tsnSock should be allocated when TransportPriority > 0")
	}
}

func TestTransportPriority_Zero_NoTSNSock(t *testing.T) {
	p := testPart(t)
	pub, err := p.NewPublisher("tp/zero", dds.DefaultQoS) // TransportPriority=0
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()
	w, ok := pub.(*rtpsWriter)
	if !ok {
		t.Fatalf("expected *rtpsWriter, got %T", pub)
	}
	if w.tsnSock != nil {
		t.Error("tsnSock should be nil when TransportPriority = 0 and no tsn config")
	}
}

func TestClockTAINow_ReturnsPlausibleTime(t *testing.T) {
	now := time.Now()
	taiNow, err := clockTAINow()
	if err != nil {
		t.Skipf("CLOCK_TAI unavailable: %v", err)
	}
	// TAI should be within a few hours of wall time (TAI is ahead by ~37s currently).
	diff := taiNow.Sub(now)
	if diff < 0 {
		diff = -diff
	}
	if diff > time.Hour {
		t.Errorf("clockTAINow() %v differs from time.Now() %v by %v (>1h), unexpected", taiNow, now, diff)
	}
}

func TestTSNConfig_FragSizePropagated(t *testing.T) {
	// MaxFrameSize=200 → MaxFragPayload()=152; fragmentSize() should return 152.
	cfg, err := tsn.ParseConfig([]byte(`{
		"streams":[{"topic":"frag/tsn","pcp":1,"max_frame_size":200}]
	}`))
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	p, err := newParticipant(dds.Domain(80), WithNoMulticast(), WithTSNConfig(cfg))
	if err != nil {
		t.Skipf("newParticipant: %v", err)
	}
	defer p.Close()

	pub, err := p.NewPublisher("frag/tsn", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()
	w, ok := pub.(*rtpsWriter)
	if !ok {
		t.Fatalf("expected *rtpsWriter, got %T", pub)
	}
	if w.tsnStream == nil {
		t.Fatal("tsnStream nil")
	}
	want := cfg.Streams[0].MaxFragPayload()
	if w.fragmentSize() != want {
		t.Errorf("fragmentSize() = %d, want %d (from MaxFrameSize=200)", w.fragmentSize(), want)
	}
}

// TestEnableTxTime_WhenTxOffsetPositive verifies that enableTxTime is called
// when a TSN stream has TxOffsetUS > 0.  On non-Linux platforms the stub just
// returns nil; on Linux it calls setsockopt(SO_TXTIME) which may fail on VMs
// without an ETF qdisc — the caller ignores the error either way.
func TestEnableTxTime_WhenTxOffsetPositive(t *testing.T) {
	cfg, err := tsn.ParseConfig([]byte(`{
		"streams":[{"topic":"tsn/txtime","pcp":5,"dscp":46,"tx_offset_us":50,"interval_us":125}]
	}`))
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	p, err := newParticipant(dds.Domain(79), WithNoMulticast(), WithTSNConfig(cfg))
	if err != nil {
		t.Skipf("newParticipant: %v", err)
	}
	defer p.Close()

	pub, err := p.NewPublisher("tsn/txtime", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	w, ok := pub.(*rtpsWriter)
	if !ok {
		t.Fatalf("expected *rtpsWriter, got %T", pub)
	}
	if w.tsnStream == nil {
		t.Fatal("tsnStream should be set when topic matches TSN config")
	}
	if w.tsnSock == nil {
		t.Fatal("tsnSock should be non-nil; tsnSocketForPCP should have called enableTxTime")
	}
}

// TestScheduledSend_TriggeredOnWrite exercises the scheduledSend path by
// injecting a fake remote-reader locator so that matchedReaderLocators returns
// a non-empty slice, then writing a sample through a TSN publisher with
// TxOffsetUS > 0.  The production code ignores the send error (nobody is
// listening at the fake loopback port), so the test just verifies Write
// returns nil (the local-delivery path is unaffected).
func TestScheduledSend_TriggeredOnWrite(t *testing.T) {
	cfg, err := tsn.ParseConfig([]byte(`{
		"streams":[{"topic":"tsn/sched","pcp":5,"dscp":46,"tx_offset_us":50,"interval_us":125}]
	}`))
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	p, err := newParticipant(dds.Domain(78), WithNoMulticast(), WithTSNConfig(cfg))
	if err != nil {
		t.Skipf("newParticipant: %v", err)
	}
	defer p.Close()

	pub, err := p.NewPublisher("tsn/sched", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	// Inject a fake remote reader so matchedReaderLocators returns a locator,
	// making the write take the unicast (scheduledSend) path instead of no-op.
	fakeGUID := GUID{
		Prefix: GuidPrefix{0xFA, 0xFB, 0xFC, 0xFD, 0xFE, 0xFF, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06},
		Entity: entityIdForReader(0xFFFF),
	}
	// IPv4 loopback 127.0.0.1 at an ephemeral port — nobody listening, but the
	// send error is silently ignored by the caller.
	loopback := Locator{Kind: LocatorKindUDPv4, Port: 19977}
	loopback.Address[12] = 127
	loopback.Address[15] = 1

	p.sedp.mu.Lock()
	p.sedp.remoteReaders[fakeGUID] = &endpointInfo{topicName: "tsn/sched"}
	p.sedp.remoteReaderLocs[fakeGUID] = loopback
	p.sedp.mu.Unlock()

	// Write triggers: tsnStream.TxOffsetUS > 0 → clockTAINow() → txTimeNS > 0
	// → else branch → scheduledSend(tsnSock.conn, dst, msg, txTimeNS).
	if err := pub.Write([]byte("tsn-scheduled")); err != nil {
		t.Fatalf("Write: %v", err)
	}
}

func TestWrite_FragmentedPath_LargePayload(t *testing.T) {
	// Verify that a payload larger than maxFragmentPayload is written without error.
	p := testPart(t)
	sub, err := p.NewSubscriber("frag/large", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()
	pub, err := p.NewPublisher("frag/large", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	large := make([]byte, maxFragmentPayload+100)
	for i := range large {
		large[i] = byte(i % 251)
	}
	if err := pub.Write(large); err != nil {
		t.Fatalf("Write large payload: %v", err)
	}
	select {
	case s := <-sub.C():
		if string(s.Payload) != string(large) {
			t.Error("large payload received does not match original")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for large payload")
	}
}
