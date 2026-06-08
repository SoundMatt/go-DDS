// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Wire-format unit tests. Uses package rtps (internal) for access to
// unexported CDR/RTPS encoding functions without needing CGo or live sockets.

package rtps

//fusa:test REQ-RTPS-001
//fusa:test REQ-RTPS-002
//fusa:test REQ-RTPS-003
//fusa:test REQ-RTPS-004
//fusa:test REQ-RTPS-006
//fusa:test REQ-RTPS-007
//fusa:test REQ-RTPS-008
//fusa:test REQ-RTPS-009
//fusa:test REQ-RTPS-010
//fusa:test REQ-RTPS-011
//fusa:test REQ-CDR-001
//fusa:test REQ-CDR-002
//fusa:test REQ-CDR-003
//fusa:test REQ-GUID-001
//fusa:test REQ-GUID-002
//fusa:test REQ-LOC-001
//fusa:test REQ-LOC-002
//fusa:test REQ-LOC-003
//fusa:test REQ-FRAG-001
//fusa:test REQ-FRAG-002
//fusa:test REQ-FRAG-003
//fusa:test REQ-FRAG-004
//fusa:test REQ-FRAG-005

import (
	"bytes"
	"net"
	"testing"
)

// ── CDR string encoding ───────────────────────────────────────────────────────

func TestCDR_StringRoundTrip(t *testing.T) {
	cases := []string{"hello", "", "unicode/日本語", "a/b/c/d", "topic with spaces"}
	for _, tc := range cases {
		t.Run(tc, func(t *testing.T) {
			enc := newPLCDREncoder()
			enc.addString(pidTopicName, tc)
			raw := enc.finish()

			dec, ok := newPLCDRDecoder(raw)
			if !ok {
				t.Fatal("newPLCDRDecoder failed")
			}
			p, ok := dec.next()
			if !ok {
				t.Fatal("dec.next() returned no param")
			}
			got, ok := decodeString(p.value)
			if !ok {
				t.Fatal("decodeString failed")
			}
			if got != tc {
				t.Errorf("got %q, want %q", got, tc)
			}
		})
	}
}

// ── CDR GUID encoding ─────────────────────────────────────────────────────────

func TestCDR_GUIDRoundTrip(t *testing.T) {
	want := GUID{
		Prefix: GuidPrefix{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12},
		Entity: EntityId{0xAA, 0xBB, 0xCC, 0x03},
	}

	enc := newPLCDREncoder()
	enc.addGUID(pidEndpointGUID, want)
	raw := enc.finish()

	dec, ok := newPLCDRDecoder(raw)
	if !ok {
		t.Fatal("newPLCDRDecoder failed")
	}
	p, ok := dec.next()
	if !ok {
		t.Fatal("dec.next() returned no param")
	}
	got, ok := decodeGUID(p.value)
	if !ok {
		t.Fatal("decodeGUID failed")
	}
	if got != want {
		t.Errorf("GUID mismatch: got %v, want %v", got, want)
	}
}

// ── CDR uint32 encoding ───────────────────────────────────────────────────────

func TestCDR_Uint32RoundTrip(t *testing.T) {
	enc := newPLCDREncoder()
	enc.addUint32(pidBuiltinEndpointSet, 0xDEADBEEF)
	raw := enc.finish()

	dec, _ := newPLCDRDecoder(raw)
	p, ok := dec.next()
	if !ok {
		t.Fatal("no param")
	}
	if len(p.value) < 4 {
		t.Fatalf("value too short: %d", len(p.value))
	}
	// Re-read manually since addUint32 is little-endian.
	got := uint32(p.value[0]) | uint32(p.value[1])<<8 | uint32(p.value[2])<<16 | uint32(p.value[3])<<24
	if got != 0xDEADBEEF {
		t.Errorf("got 0x%08X, want 0xDEADBEEF", got)
	}
}

// ── Locator marshal/unmarshal ─────────────────────────────────────────────────

func TestLocator_MarshalUnmarshal(t *testing.T) {
	cases := []Locator{
		{Kind: LocatorKindUDPv4, Port: 7410, Address: [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 127, 0, 0, 1}},
		{Kind: LocatorKindUDPv4, Port: 32161, Address: [16]byte{}},
		{Kind: LocatorKindInvalid, Port: 0, Address: [16]byte{}},
	}
	for _, want := range cases {
		b := marshalLocator(want)
		if len(b) != 24 {
			t.Errorf("expected 24 bytes, got %d", len(b))
		}
		got, ok := unmarshalLocator(b)
		if !ok {
			t.Fatal("unmarshalLocator failed")
		}
		if got != want {
			t.Errorf("locator mismatch: got %+v, want %+v", got, want)
		}
	}
}

func TestLocator_FromUDP(t *testing.T) {
	addr := &net.UDPAddr{IP: net.ParseIP("192.168.1.1"), Port: 7411}
	l := locatorFromUDP(addr, 7411)
	if l.Kind != LocatorKindUDPv4 {
		t.Errorf("kind: got %d", l.Kind)
	}
	if l.Port != 7411 {
		t.Errorf("port: got %d", l.Port)
	}
	if l.Address[12] != 192 || l.Address[13] != 168 || l.Address[14] != 1 || l.Address[15] != 1 {
		t.Errorf("address: got %v", l.Address[12:])
	}
}

func TestLocator_UDPAddr_Roundtrip(t *testing.T) {
	l := Locator{Kind: LocatorKindUDPv4, Port: 9999,
		Address: [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 10, 0, 0, 1}}
	addr := l.udpAddr()
	if addr == nil {
		t.Fatal("udpAddr returned nil")
	}
	if addr.Port != 9999 {
		t.Errorf("port: got %d", addr.Port)
	}
	if !addr.IP.Equal(net.ParseIP("10.0.0.1")) {
		t.Errorf("IP: got %v", addr.IP)
	}
}

// ── RTPS Header ───────────────────────────────────────────────────────────────

func TestRTPS_HeaderMarshalParse(t *testing.T) {
	prefix := GuidPrefix{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}
	want := Header{
		ProtocolVersion: [2]byte{2, 3},
		VendorId:        goVendorId,
		GuidPrefix:      prefix,
	}
	b := marshalHeader(want)
	if len(b) != 20 {
		t.Fatalf("expected 20 bytes, got %d", len(b))
	}

	got, ok := parseHeader(b)
	if !ok {
		t.Fatal("parseHeader failed")
	}
	if got != want {
		t.Errorf("header mismatch:\n  got  %+v\n  want %+v", got, want)
	}
}

func TestRTPS_Header_InvalidMagic(t *testing.T) {
	b := make([]byte, 20)
	b[0] = 'X' // wrong magic
	_, ok := parseHeader(b)
	if ok {
		t.Error("parseHeader should fail on bad magic")
	}
}

func TestRTPS_Header_TooShort(t *testing.T) {
	_, ok := parseHeader(make([]byte, 15))
	if ok {
		t.Error("parseHeader should fail on short input")
	}
}

// ── DATA submessage ───────────────────────────────────────────────────────────

func TestRTPS_DataSubmessageRoundTrip(t *testing.T) {
	payloads := [][]byte{
		[]byte("hello"),
		[]byte(`{"action":"get","path":"Vehicle.Speed"}`),
		make([]byte, 1024),
		{},
	}
	for _, want := range payloads {
		wrapped := cdrWrapPayload(want)
		submsg := marshalDataSubmessage(
			entityIdForWriter(1),
			EntityIdUnknown,
			SequenceNumber{High: 0, Low: 42},
			wrapped,
		)
		if len(submsg) < 4 {
			t.Fatal("submessage too short")
		}
		id := submsg[0]
		flags := submsg[1]
		if id != submsgDATA {
			t.Fatalf("submessage ID: got 0x%02X, want 0x%02X", id, submsgDATA)
		}
		body := submsg[4:]
		ds, ok := parseDataSubmessage(flags, body)
		if !ok {
			t.Fatal("parseDataSubmessage failed")
		}
		if ds.SeqNum.Low != 42 {
			t.Errorf("seqNum: got %d, want 42", ds.SeqNum.Low)
		}
		got, ok := cdrUnwrapPayload(ds.Payload)
		if !ok {
			t.Fatal("cdrUnwrapPayload failed")
		}
		if !bytes.Equal(got, want) {
			t.Errorf("payload: got %q, want %q", got, want)
		}
	}
}

// ── CDR payload wrap/unwrap ───────────────────────────────────────────────────

func TestCDR_WrapUnwrap(t *testing.T) {
	cases := [][]byte{
		{},
		{0xAB},
		make([]byte, 65531),
		[]byte(`{"v":1}`),
	}
	for _, want := range cases {
		wrapped := cdrWrapPayload(want)
		if len(wrapped) < 4 {
			t.Fatal("wrapped too short")
		}
		got, ok := cdrUnwrapPayload(wrapped)
		if !ok {
			t.Fatal("cdrUnwrapPayload failed")
		}
		if !bytes.Equal(got, want) {
			t.Errorf("payload mismatch: got %q, want %q", got, want)
		}
	}
}

func TestCDR_UnwrapInvalidScheme(t *testing.T) {
	b := []byte{0xFF, 0xFF, 0x00, 0x00, 0x01, 0x02}
	_, ok := cdrUnwrapPayload(b)
	if ok {
		t.Error("expected cdrUnwrapPayload to fail on unknown scheme")
	}
}

// ── GUID entity ID helpers ────────────────────────────────────────────────────

func TestEntityID_WriterKind(t *testing.T) {
	eid := entityIdForWriter(7)
	if eid[3] != 0x03 {
		t.Errorf("writer kind byte: got 0x%02X, want 0x03", eid[3])
	}
}

func TestEntityID_ReaderKind(t *testing.T) {
	eid := entityIdForReader(7)
	if eid[3] != 0x04 {
		t.Errorf("reader kind byte: got 0x%02X, want 0x04", eid[3])
	}
}

// ── Port formula ──────────────────────────────────────────────────────────────

func TestPortFormula(t *testing.T) {
	// Domain 0: meta-multicast=7400, meta-unicast=7410, data-unicast=7411
	if metaMulticastPort(0) != 7400 {
		t.Errorf("metaMulticast(0) = %d", metaMulticastPort(0))
	}
	if metaUnicastPort(0, 0) != 7410 {
		t.Errorf("metaUnicast(0,0) = %d", metaUnicastPort(0, 0))
	}
	if userUnicastPort(0, 0) != 7411 {
		t.Errorf("userUnicast(0,0) = %d", userUnicastPort(0, 0))
	}
	// Domain 99: meta-multicast=32150, meta-unicast=32160, data-unicast=32161
	if metaMulticastPort(99) != 32150 {
		t.Errorf("metaMulticast(99) = %d", metaMulticastPort(99))
	}
	if metaUnicastPort(99, 0) != 32160 {
		t.Errorf("metaUnicast(99,0) = %d", metaUnicastPort(99, 0))
	}
	if userUnicastPort(99, 0) != 32161 {
		t.Errorf("userUnicast(99,0) = %d", userUnicastPort(99, 0))
	}
}

// ── Full RTPS message round-trip ──────────────────────────────────────────────

func TestRTPS_FullMessageRoundTrip(t *testing.T) {
	prefix := newGuidPrefix()
	want := []byte(`{"action":"get","path":"Vehicle.Speed"}`)

	wrapped := cdrWrapPayload(want)
	submsg := marshalDataSubmessage(entityIdForWriter(1), EntityIdUnknown,
		SequenceNumber{High: 0, Low: 1}, wrapped)
	msg := wrapInRTPSMessage(prefix, submsg)

	hdr, ok := parseHeader(msg)
	if !ok {
		t.Fatal("parseHeader failed")
	}
	if hdr.GuidPrefix != prefix {
		t.Error("GuidPrefix mismatch")
	}

	var got []byte
	_ = parseSubmessages(msg[20:], func(id, flags byte, body []byte) error {
		if id != submsgDATA {
			return nil
		}
		ds, ok := parseDataSubmessage(flags, body)
		if !ok || ds.Payload == nil {
			return nil
		}
		got, _ = cdrUnwrapPayload(ds.Payload)
		return nil
	})
	if !bytes.Equal(got, want) {
		t.Errorf("round-trip: got %q, want %q", got, want)
	}
}

// ── HEARTBEAT submessage ──────────────────────────────────────────────────────

func TestRTPS_HeartbeatRoundTrip(t *testing.T) {
	want := Heartbeat{
		ReaderEntityId: EntityIdUnknown,
		WriterEntityId: entityIdForWriter(3),
		FirstSN:        SequenceNumber{High: 0, Low: 1},
		LastSN:         SequenceNumber{High: 0, Low: 7},
		Count:          42,
	}
	msg := marshalHeartbeat(want)
	if msg[0] != submsgHEARTBEAT {
		t.Fatalf("submsgID: got 0x%02X, want 0x%02X", msg[0], submsgHEARTBEAT)
	}
	body := msg[4:]
	got, ok := parseHeartbeat(body)
	if !ok {
		t.Fatal("parseHeartbeat failed")
	}
	if got != want {
		t.Errorf("Heartbeat mismatch:\n  got  %+v\n  want %+v", got, want)
	}
}

func TestRTPS_Heartbeat_TooShort(t *testing.T) {
	_, ok := parseHeartbeat(make([]byte, 10))
	if ok {
		t.Error("parseHeartbeat should fail on short body")
	}
}

// ── ACKNACK submessage ────────────────────────────────────────────────────────

func TestRTPS_AckNackRoundTrip(t *testing.T) {
	want := AckNack{
		ReaderEntityId: entityIdForReader(2),
		WriterEntityId: entityIdForWriter(1),
		Base:           SequenceNumber{High: 0, Low: 5},
		Bitmap:         0b1010, // missing sequence numbers 5 and 7
		Count:          1,
	}
	msg := marshalAckNack(want)
	if msg[0] != submsgACKNACK {
		t.Fatalf("submsgID: got 0x%02X, want 0x%02X", msg[0], submsgACKNACK)
	}
	body := msg[4:]
	got, ok := parseAckNack(body)
	if !ok {
		t.Fatal("parseAckNack failed")
	}
	if got != want {
		t.Errorf("AckNack mismatch:\n  got  %+v\n  want %+v", got, want)
	}
}

func TestRTPS_AckNack_TooShort(t *testing.T) {
	_, ok := parseAckNack(make([]byte, 10))
	if ok {
		t.Error("parseAckNack should fail on short body")
	}
}

// ── sendHistory ───────────────────────────────────────────────────────────────

func TestSendHistory_StoreGet(t *testing.T) {
	h := newSendHistory()
	msg := []byte("test-message")
	h.store(1, msg)
	got := h.get(1)
	if string(got) != string(msg) {
		t.Errorf("get(1): got %q, want %q", got, msg)
	}
	if h.get(99) != nil {
		t.Error("get(99) should return nil for unknown seqLo")
	}
}

func TestSendHistory_FirstLast(t *testing.T) {
	h := newSendHistory()
	_, _, ok := h.firstLast()
	if ok {
		t.Error("empty history should return ok=false")
	}
	h.store(3, []byte("a"))
	h.store(1, []byte("b"))
	h.store(5, []byte("c"))
	first, last, ok := h.firstLast()
	if !ok {
		t.Fatal("non-empty history returned ok=false")
	}
	if first != 1 || last != 5 {
		t.Errorf("firstLast: got (%d, %d), want (1, 5)", first, last)
	}
}

func TestSendHistory_PayloadIsolation(t *testing.T) {
	h := newSendHistory()
	msg := []byte("original")
	h.store(1, msg)
	msg[0] = 'X' // mutate after store
	got := h.get(1)
	if got[0] == 'X' {
		t.Error("sendHistory.store should copy the message")
	}
}

// ── recvTracker ───────────────────────────────────────────────────────────────

func TestRecvTracker_Sequential(t *testing.T) {
	rt := &recvTracker{}
	for _, sn := range []uint32{1, 2, 3, 4, 5} {
		_, _, needAck := rt.receive(sn)
		if needAck {
			t.Errorf("sn=%d: unexpected ACKNACK", sn)
		}
	}
}

func TestRecvTracker_Gap(t *testing.T) {
	rt := &recvTracker{}
	rt.receive(1)                          // expected becomes 2
	base, bitmap, needAck := rt.receive(4) // gap: 2 and 3 missing
	if !needAck {
		t.Fatal("expected needAck for gap")
	}
	if base != 2 {
		t.Errorf("base: got %d, want 2", base)
	}
	// bits 0 and 1 set (sn 2 and 3 missing)
	if bitmap&0b11 != 0b11 {
		t.Errorf("bitmap: got 0b%b, want low 2 bits set", bitmap)
	}
}

func TestRecvTracker_Duplicate(t *testing.T) {
	rt := &recvTracker{}
	rt.receive(1)
	rt.receive(2)
	_, _, needAck := rt.receive(1) // duplicate
	if needAck {
		t.Error("duplicate sample should not trigger ACKNACK")
	}
}

// ── GUID / EntityId / GuidPrefix String methods ───────────────────────────────

func TestGUID_String(t *testing.T) {
	p := GuidPrefix{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}
	e := EntityId{0xAA, 0xBB, 0xCC, 0x03}
	g := GUID{Prefix: p, Entity: e}

	if got := p.String(); got != "0102030405060708090a0b0c" {
		t.Errorf("GuidPrefix.String: got %q", got)
	}
	if got := e.String(); got != "aabbcc03" {
		t.Errorf("EntityId.String: got %q", got)
	}
	want := "0102030405060708090a0b0c/aabbcc03"
	if got := g.String(); got != want {
		t.Errorf("GUID.String: got %q, want %q", got, want)
	}
}

// ── Locator edge cases ────────────────────────────────────────────────────────

func TestLocator_UDPAddr_Invalid(t *testing.T) {
	l := Locator{Kind: LocatorKindInvalid, Port: 7410}
	if addr := l.udpAddr(); addr != nil {
		t.Errorf("udpAddr for invalid locator should return nil, got %v", addr)
	}
}

func TestLocator_IPv6_RoundTrip(t *testing.T) {
	ip6 := net.ParseIP("2001:db8::1")
	addr := &net.UDPAddr{IP: ip6, Port: 7411}
	l := locatorFromUDPv6(addr, 7411)

	if l.Kind != LocatorKindUDPv6 {
		t.Errorf("kind: got %d, want LocatorKindUDPv6 (%d)", l.Kind, LocatorKindUDPv6)
	}
	if l.Port != 7411 {
		t.Errorf("port: got %d, want 7411", l.Port)
	}
	got := l.udpAddr()
	if got == nil {
		t.Fatal("udpAddr returned nil for UDPv6 locator")
	}
	if !got.IP.Equal(ip6) {
		t.Errorf("IP: got %v, want %v", got.IP, ip6)
	}
	if got.Port != 7411 {
		t.Errorf("port: got %d, want 7411", got.Port)
	}
}

func TestLocator_FromUDP_IPv4inIPv6(t *testing.T) {
	// net.ParseIP("192.168.1.1") returns a 16-byte IPv4-in-IPv6 form,
	// but .To4() is non-nil — should produce a UDPv4 locator.
	addr := &net.UDPAddr{IP: net.ParseIP("192.168.1.1"), Port: 9000}
	l := locatorFromUDP(addr, 9000)
	if l.Kind != LocatorKindUDPv4 {
		t.Errorf("expected UDPv4 locator for IPv4 address, got kind %d", l.Kind)
	}
}

// ── recvTracker.nextAckCount ──────────────────────────────────────────────────

func TestRecvTracker_NextAckCount(t *testing.T) {
	rt := &recvTracker{}
	first := rt.nextAckCount()
	second := rt.nextAckCount()
	if second != first+1 {
		t.Errorf("nextAckCount: %d then %d — expected monotone increment", first, second)
	}
}

// ── rtpsReader internal helpers ───────────────────────────────────────────────

func TestRtpsReader_AddSourceAndAccept(t *testing.T) {
	p := &participant{guidPrefix: newGuidPrefix()}
	r := &rtpsReader{p: p, topic: "t", eid: entityIdForReader(1)}

	// No explicit sources: accepts only from same participant prefix.
	sameGUID := GUID{Prefix: p.guidPrefix, Entity: entityIdForWriter(1)}
	if !r.acceptsSource(sameGUID) {
		t.Error("acceptsSource should accept same-participant GUID when no sources registered")
	}
	foreign := GUID{Prefix: newGuidPrefix(), Entity: entityIdForWriter(2)}
	if r.acceptsSource(foreign) {
		t.Error("acceptsSource should reject unregistered foreign GUID")
	}

	// After addSourceGUID, foreign GUID should be accepted.
	r.addSourceGUID(foreign)
	if !r.acceptsSource(foreign) {
		t.Error("acceptsSource should accept explicitly registered GUID")
	}
}

func TestRtpsReader_TrackerFor_Stable(t *testing.T) {
	p := &participant{guidPrefix: newGuidPrefix()}
	r := &rtpsReader{p: p, topic: "t", eid: entityIdForReader(1)}

	g := GUID{Prefix: newGuidPrefix(), Entity: entityIdForWriter(1)}
	t1 := r.trackerFor(g)
	if t1 == nil {
		t.Fatal("trackerFor returned nil on first call")
	}
	t2 := r.trackerFor(g)
	if t1 != t2 {
		t.Error("trackerFor should return the same tracker on repeated calls")
	}
}

// ── participant internal helpers ──────────────────────────────────────────────

func TestParticipant_ReaderByEID(t *testing.T) {
	p := &participant{
		guidPrefix: newGuidPrefix(),
		readers:    make(map[EntityId]*rtpsReader),
	}
	eid := entityIdForReader(1)
	r := &rtpsReader{p: p, topic: "t", eid: eid}
	p.readers[eid] = r

	called := false
	p.readerByEID(eid, func(found *rtpsReader) {
		called = true
		if found != r {
			t.Error("readerByEID: wrong reader returned")
		}
	})
	if !called {
		t.Error("readerByEID: fn not called for known eid")
	}

	// Unknown eid must not call fn.
	p.readerByEID(entityIdForReader(99), func(*rtpsReader) {
		t.Error("readerByEID: fn called for unknown eid")
	})
}

func TestParticipant_AddWriterLocator(t *testing.T) {
	p := &participant{
		guidPrefix:     newGuidPrefix(),
		writerLocators: make(map[GUID]Locator),
	}
	g := GUID{Prefix: newGuidPrefix(), Entity: entityIdForWriter(1)}
	l := Locator{Kind: LocatorKindUDPv4, Port: 9999}
	p.addWriterLocator(g, l)
	if got, ok := p.writerLocators[g]; !ok || got != l {
		t.Error("addWriterLocator: locator not stored correctly")
	}
}

// ── GUID prefix generation ────────────────────────────────────────────────────

func TestNewGuidPrefix_Unique(t *testing.T) {
	a := newGuidPrefix()
	b := newGuidPrefix()
	if a == b {
		t.Error("two newGuidPrefix() calls returned the same prefix")
	}
}

func TestNewGuidPrefix_PIDBytes(t *testing.T) {
	p := newGuidPrefix()
	// Bytes 8–11 should encode the current PID — just check they're not all zero.
	if p[8] == 0 && p[9] == 0 && p[10] == 0 && p[11] == 0 {
		t.Error("PID bytes in GuidPrefix are all zero")
	}
}
