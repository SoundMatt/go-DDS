// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Wire-format unit tests. Uses package rtps (internal) for access to
// unexported CDR/RTPS encoding functions without needing CGo or live sockets.

package rtps

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
