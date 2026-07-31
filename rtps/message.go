// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// RTPS message framing: Header, SubmessageHeader, and DATA submessage
// as defined in RTPS 2.3 §9.4.

package rtps

//fusa:req REQ-RTPS-001
//fusa:req REQ-RTPS-002
//fusa:req REQ-RTPS-003
//fusa:req REQ-RTPS-004
//fusa:req REQ-RTPS-005
//fusa:req REQ-RTPS-006
//fusa:req REQ-RTPS-007
//fusa:req REQ-RTPS-008
//fusa:req REQ-RTPS-009
//fusa:req REQ-RTPS-010
//fusa:req REQ-RTPS-011

import (
	"encoding/binary"
	"fmt"
	"math/bits"
	"time"
)

// Magic bytes at the start of every RTPS message.
var rtpsMagic = [4]byte{'R', 'T', 'P', 'S'}

// go-DDS vendor ID (unregistered; 0x0127 = arbitrary).
var goVendorId = [2]byte{0x01, 0x27}

// Header is the fixed 20-byte RTPS message header (§9.4.1).
type Header struct {
	ProtocolVersion [2]byte // {major, minor}
	VendorId        [2]byte
	GuidPrefix      GuidPrefix
}

// SequenceNumber is the 8-byte writer sequence number (§9.3.2).
type SequenceNumber struct {
	High int32
	Low  uint32
}

// submessage IDs (§9.4.5).
const (
	submsgDATA      = byte(0x15)
	submsgGAP       = byte(0x08)
	submsgHEARTBEAT = byte(0x07)
	submsgACKNACK   = byte(0x06)
	submsgINFO_TS   = byte(0x09)
)

// flag bits for DATA submessage (§9.4.5.3).
const (
	flagEndianness = byte(0x01) // E: 1 = little-endian
	flagInlineQos  = byte(0x02) // Q: inline QoS present
	flagData       = byte(0x04) // D: serialised payload present
)

// flag bits for HEARTBEAT submessage (§9.4.5.5).
const (
	// hbFlagLiveliness is the spec's "L" flag: set when a HEARTBEAT is sent
	// purely to assert liveliness (Milestone 14, "QoS Enforcement — Active
	// Policy"), as opposed to advertising the writer's retransmission
	// history. Readers must not feed a liveliness-only HEARTBEAT into their
	// reliability tracker (no ACKNACK should be generated from it) — only
	// touch the writer's last-seen-alive timestamp. See
	// rtpsWriter.assertLiveliness and participant.handleHeartbeat.
	hbFlagLiveliness = byte(0x04)
)

// marshalHeader writes the 20-byte RTPS Header into buf.
func marshalHeader(h Header) []byte {
	b := make([]byte, 20)
	copy(b[0:4], rtpsMagic[:])
	copy(b[4:6], h.ProtocolVersion[:])
	copy(b[6:8], h.VendorId[:])
	copy(b[8:20], h.GuidPrefix[:])
	return b
}

// parseHeader parses the 20-byte RTPS header; returns false if magic is wrong.
func parseHeader(b []byte) (Header, bool) {
	if len(b) < 20 {
		return Header{}, false
	}
	if b[0] != 'R' || b[1] != 'T' || b[2] != 'P' || b[3] != 'S' {
		return Header{}, false
	}
	var h Header
	copy(h.ProtocolVersion[:], b[4:6])
	copy(h.VendorId[:], b[6:8])
	copy(h.GuidPrefix[:], b[8:20])
	return h, true
}

// marshalDataSubmessage builds a DATA submessage carrying serialisedPayload.
// serialisedPayload should already include the CDR encapsulation header.
func marshalDataSubmessage(writerEID, readerEID EntityId, seqNum SequenceNumber, serialisedPayload []byte) []byte {
	// DATA body layout (§9.4.5.3):
	//   extraFlags(2) + octetsToInlineQos(2) + readerId(4) + writerId(4)
	//   + seqNum.High(4) + seqNum.Low(4) + payload(variable) = 20 + len(payload)
	//
	// octetsToInlineQos = 16: distance from the end of the octetsToInlineQos
	// field (body[4]) to the start of payload (body[20]) = 16 bytes.
	body := make([]byte, 20+len(serialisedPayload))
	binary.LittleEndian.PutUint16(body[0:], 0)  // extraFlags
	binary.LittleEndian.PutUint16(body[2:], 16) // octetsToInlineQos
	copy(body[4:8], readerEID[:])
	copy(body[8:12], writerEID[:])
	binary.LittleEndian.PutUint32(body[12:], uint32(seqNum.High))
	binary.LittleEndian.PutUint32(body[16:], seqNum.Low)
	copy(body[20:], serialisedPayload)

	// Submessage header: id(1) + flags(1) + octetsToNextHeader(2).
	flags := flagEndianness | flagData
	hdr := make([]byte, 4)
	hdr[0] = submsgDATA
	hdr[1] = flags
	binary.LittleEndian.PutUint16(hdr[2:], uint16(len(body)))
	return append(hdr, body...)
}

// DataSubmessage holds the parsed fields of a DATA submessage.
type DataSubmessage struct {
	ReaderEntityId EntityId
	WriterEntityId EntityId
	SeqNum         SequenceNumber
	Payload        []byte // nil when Data flag not set
}

// parseSubmessages iterates over all submessages in an RTPS message body
// (the bytes after the 20-byte Header). It calls fn for each submessage,
// passing (submessageId, flags, body). Returns an error on malformed input.
//
// Per RTPS 2.3 §9.4.5.1.1, bit 0 of the flags octet is the E (endianness)
// flag: it governs the byte order of every multi-octet field within that
// submessage — including octetsToNextHeader — independent of any other
// submessage in the same message. octetsToNextHeader is therefore decoded
// per-submessage according to its own E flag, not unconditionally as
// little-endian.
//
// This package's submessage body decoders (parseDataSubmessage, parseInfoTS,
// parseGAP, parseHeartbeat, parseAckNack, parseDataFrag, the PL_CDR
// parameter-list decoder, etc.) only implement little-endian field decoding,
// matching the little-endian-only CDR/PL_CDR support documented in cdr.go.
// A submessage that declares big-endian (E=0) cannot be safely handed to
// those decoders — doing so would silently misinterpret its fields rather
// than reject them — so such a submessage is rejected here rather than
// parsed. This enforces (and makes explicit) an all-little-endian-peer
// restriction, per go-DDS's documented LE-only CDR support.
func parseSubmessages(body []byte, fn func(id, flags byte, body []byte) error) error {
	for len(body) >= 4 {
		id := body[0]
		flags := body[1]
		little := flags&flagEndianness != 0
		var length int
		if little {
			length = int(binary.LittleEndian.Uint16(body[2:4]))
		} else {
			length = int(binary.BigEndian.Uint16(body[2:4]))
		}
		body = body[4:]
		if length > len(body) {
			return fmt.Errorf("rtps: submessage length %d exceeds remaining bytes %d", length, len(body))
		}
		if !little {
			// Big-endian submessage body: framing (this length field) is
			// endianness-correct above, but the body decoders below this
			// point are little-endian only — see the doc comment. Skip this
			// submessage rather than misparse its fields.
			body = body[length:]
			continue
		}
		if err := fn(id, flags, body[:length]); err != nil {
			return err
		}
		body = body[length:]
	}
	return nil
}

// parseDataSubmessage extracts a DataSubmessage from a DATA submessage body.
func parseDataSubmessage(flags byte, body []byte) (DataSubmessage, bool) {
	// body = extraFlags(2) + octetsToInlineQos(2) + readerEID(4) + writerEID(4) + seqNum(8) + ...
	if len(body) < 20 {
		return DataSubmessage{}, false
	}
	var ds DataSubmessage
	copy(ds.ReaderEntityId[:], body[4:8])
	copy(ds.WriterEntityId[:], body[8:12])
	highRaw := binary.LittleEndian.Uint32(body[12:16])
	ds.SeqNum = SequenceNumber{
		High: int32(highRaw),
		Low:  binary.LittleEndian.Uint32(body[16:20]),
	}
	if flags&flagData != 0 && len(body) > 20 {
		payload := make([]byte, len(body)-20)
		copy(payload, body[20:])
		ds.Payload = payload
	}
	return ds, true
}

// ── HEARTBEAT submessage (§9.4.5.5) ──────────────────────────────────────────

// Heartbeat holds the parsed fields of a HEARTBEAT submessage.
type Heartbeat struct {
	ReaderEntityId EntityId
	WriterEntityId EntityId
	FirstSN        SequenceNumber // lowest SN still in the writer's history
	LastSN         SequenceNumber // highest SN sent so far
	Count          int32          // monotonically increasing per writer
	// Liveliness marks this HEARTBEAT as a pure liveliness assertion (the "L"
	// flag, §9.4.5.5) rather than a retransmission-history advertisement. The
	// zero value (false) reproduces the pre-Milestone-14 wire format exactly.
	Liveliness bool
}

// marshalHeartbeat builds a HEARTBEAT submessage.
// Body layout: readerEID(4) + writerEID(4) + firstSN(8) + lastSN(8) + count(4) = 28 bytes.
func marshalHeartbeat(hb Heartbeat) []byte {
	body := make([]byte, 28)
	copy(body[0:4], hb.ReaderEntityId[:])
	copy(body[4:8], hb.WriterEntityId[:])
	binary.LittleEndian.PutUint32(body[8:], uint32(hb.FirstSN.High))
	binary.LittleEndian.PutUint32(body[12:], hb.FirstSN.Low)
	binary.LittleEndian.PutUint32(body[16:], uint32(hb.LastSN.High))
	binary.LittleEndian.PutUint32(body[20:], hb.LastSN.Low)
	binary.LittleEndian.PutUint32(body[24:], uint32(hb.Count))
	hdr := make([]byte, 4)
	hdr[0] = submsgHEARTBEAT
	hdr[1] = flagEndianness
	if hb.Liveliness {
		hdr[1] |= hbFlagLiveliness
	}
	binary.LittleEndian.PutUint16(hdr[2:], uint16(len(body)))
	return append(hdr, body...)
}

// parseHeartbeat extracts a Heartbeat from a HEARTBEAT submessage body.
func parseHeartbeat(body []byte) (Heartbeat, bool) {
	if len(body) < 28 {
		return Heartbeat{}, false
	}
	var hb Heartbeat
	copy(hb.ReaderEntityId[:], body[0:4])
	copy(hb.WriterEntityId[:], body[4:8])
	hb.FirstSN = SequenceNumber{
		High: int32(binary.LittleEndian.Uint32(body[8:])),
		Low:  binary.LittleEndian.Uint32(body[12:]),
	}
	hb.LastSN = SequenceNumber{
		High: int32(binary.LittleEndian.Uint32(body[16:])),
		Low:  binary.LittleEndian.Uint32(body[20:]),
	}
	hb.Count = int32(binary.LittleEndian.Uint32(body[24:]))
	return hb, true
}

// ── ACKNACK submessage (§9.4.5.1) ────────────────────────────────────────────

// AckNack holds the parsed fields of an ACKNACK submessage. Internally the
// bitmap is a single LSB-first 32-bit word (bit N → Base+N); the wire encoding
// is a variable-length, MSB-first SequenceNumberSet handled by
// marshalAckNack/parseAckNack (RTPS 2.3 §9.4.2.6).
type AckNack struct {
	ReaderEntityId EntityId
	WriterEntityId EntityId
	Base           SequenceNumber // first missing sequence number
	Bitmap         uint32         // bit N set → Base+N is missing
	Count          int32
}

// marshalAckNack builds an ACKNACK submessage with a spec-conformant
// SequenceNumberSet (RTPS 2.3 §9.4.2.6): the bitmap is MSB-first (delta N is
// bit 31-N of word N/32) and numBits is the minimal count covering the set
// deltas. A caught-up reader (Bitmap==0) emits numBits=0 and no bitmap words
// (positive ACK), a 24-byte body.
// Body layout: readerEID(4) + writerEID(4) + base(8) + numBits(4) + M*bitmap(4) + count(4).
func marshalAckNack(an AckNack) []byte {
	// numBits = position of the highest set delta + 1 (0 when no deltas set).
	numBits := uint32(bits.Len32(an.Bitmap))
	numWords := (numBits + 31) / 32
	body := make([]byte, 20+numWords*4+4)
	copy(body[0:4], an.ReaderEntityId[:])
	copy(body[4:8], an.WriterEntityId[:])
	binary.LittleEndian.PutUint32(body[8:], uint32(an.Base.High))
	binary.LittleEndian.PutUint32(body[12:], an.Base.Low)
	binary.LittleEndian.PutUint32(body[16:], numBits)
	if numWords > 0 {
		// Serialize delta N at bit 31-N of the first (only) word: MSB-first.
		binary.LittleEndian.PutUint32(body[20:], bits.Reverse32(an.Bitmap))
	}
	binary.LittleEndian.PutUint32(body[20+numWords*4:], uint32(an.Count))
	hdr := make([]byte, 4)
	hdr[0] = submsgACKNACK
	hdr[1] = flagEndianness
	binary.LittleEndian.PutUint16(hdr[2:], uint16(len(body)))
	return append(hdr, body...)
}

// parseAckNack extracts an AckNack from an ACKNACK submessage body. It accepts
// a variable-length SequenceNumberSet: numBits=0 (positive ACK, 24-byte body)
// and numBits>32 (multi-word set) are both handled. Only the first 32 deltas
// are retained internally; the bitmap is de-serialized MSB-first.
func parseAckNack(body []byte) (AckNack, bool) {
	if len(body) < 20 {
		return AckNack{}, false
	}
	numBits := binary.LittleEndian.Uint32(body[16:20])
	if numBits > 256 { // §9.4.2.6 caps a SequenceNumberSet at 256 bits.
		return AckNack{}, false
	}
	numWords := (numBits + 31) / 32
	if len(body) < int(20+numWords*4+4) {
		return AckNack{}, false
	}
	var an AckNack
	copy(an.ReaderEntityId[:], body[0:4])
	copy(an.WriterEntityId[:], body[4:8])
	an.Base = SequenceNumber{
		High: int32(binary.LittleEndian.Uint32(body[8:])),
		Low:  binary.LittleEndian.Uint32(body[12:]),
	}
	if numWords > 0 {
		// De-serialize the first word MSB-first back to LSB-first deltas.
		an.Bitmap = bits.Reverse32(binary.LittleEndian.Uint32(body[20:]))
	}
	an.Count = int32(binary.LittleEndian.Uint32(body[20+numWords*4:]))
	return an, true
}

// ── GAP submessage (§9.4.5.4) ────────────────────────────────────────────────

// Gap indicates a contiguous range of sequence numbers that are permanently
// unavailable from this writer (evicted from history). Receiving a GAP tells
// the reader to advance its expected-SN pointer past the covered range.
type Gap struct {
	ReaderEntityId EntityId
	WriterEntityId EntityId
	GapStart       SequenceNumber // first irrelevant SN (inclusive)
	GapEnd         SequenceNumber // last irrelevant SN (inclusive)
}

// marshalGAP builds a GAP submessage covering [g.GapStart, g.GapEnd] inclusive.
// Body layout: readerEID(4) + writerEID(4) + gapStart(8) + gapList(8+4) = 28 bytes.
// The gapList bitmapBase is set to GapEnd.Low+1 with numBits=0 (no extra bitmap),
// so the contiguous gap [gapStart, gapList.base-1] = [gapStart, gapEnd] is declared.
func marshalGAP(g Gap) []byte {
	body := make([]byte, 28)
	copy(body[0:4], g.ReaderEntityId[:])
	copy(body[4:8], g.WriterEntityId[:])
	binary.LittleEndian.PutUint32(body[8:], uint32(g.GapStart.High))
	binary.LittleEndian.PutUint32(body[12:], g.GapStart.Low)
	// gapList.bitmapBase = first SN *after* the gap = GapEnd + 1, computed
	// as a full 64-bit sequence number so a carry out of the Low word
	// propagates into High (when GapEnd.Low == 0xFFFFFFFF, Low+1 alone
	// wraps to 0 without incrementing High, mis-declaring the covered gap
	// range across the 32-bit boundary).
	base := u64ToSN(snToU64(g.GapEnd) + 1)
	binary.LittleEndian.PutUint32(body[16:], uint32(base.High))
	binary.LittleEndian.PutUint32(body[20:], base.Low)
	binary.LittleEndian.PutUint32(body[24:], 0) // numBits = 0, no bitmap words
	hdr := make([]byte, 4)
	hdr[0] = submsgGAP
	hdr[1] = flagEndianness
	binary.LittleEndian.PutUint16(hdr[2:], uint16(len(body)))
	return append(hdr, body...)
}

// ── INFO_TS submessage (§9.4.5.8) ────────────────────────────────────────────

// marshalInfoTS builds an INFO_TS submessage encoding t as an NTP64 timestamp.
// The timestamp is: seconds since 1 Jan 1900 (NTP epoch) in 32 bits + 32-bit
// fractional second (2^32 ticks per second).
//
// The resulting submessage should be prepended to DATA so that readers can
// associate the source timestamp with the immediately following DATA.
//
// RTPS 2.3 §9.3.2's Time_t is genuinely a 32-bit-seconds wire format (this
// is not a go-DDS design choice; it is what conformant peers expect on the
// wire), so the NTP era rolls over every 2^32 seconds (~136 years) —
// starting with 1 Jan 1900 + 2^32s = 7 Feb 2036. The uint32 conversion below
// wraps modulo 2^32 at that boundary, which is the spec-correct wire
// encoding for post-2036 timestamps, not a bug in itself: era disambiguation
// on receipt is a decoder-side concern, handled in parseInfoTS below.
func marshalInfoTS(t time.Time) []byte {
	// NTP epoch is 1 Jan 1900; Unix epoch is 1 Jan 1970. Delta = 70 years.
	const ntpDelta = 2208988800
	secs := uint32(t.Unix() + ntpDelta)
	frac := uint32(uint64(t.Nanosecond()) * (1 << 32) / 1e9)
	body := make([]byte, 8)
	binary.LittleEndian.PutUint32(body[0:], secs)
	binary.LittleEndian.PutUint32(body[4:], frac)
	hdr := make([]byte, 4)
	hdr[0] = submsgINFO_TS
	hdr[1] = flagEndianness
	binary.LittleEndian.PutUint16(hdr[2:], 8)
	return append(hdr, body...)
}

// parseInfoTS extracts a time.Time from an INFO_TS submessage body.
// Returns (zero, false) on malformed input.
func parseInfoTS(body []byte) (time.Time, bool) {
	if len(body) < 8 {
		return time.Time{}, false
	}
	secs := binary.LittleEndian.Uint32(body[0:])
	frac := binary.LittleEndian.Uint32(body[4:])
	ns := int64(uint64(frac) * 1e9 >> 32)
	return ntpEraDisambiguate(secs, ns, time.Now()), true
}

// ntpEraDisambiguate reconstructs a full timestamp from a bare 32-bit NTP
// seconds-since-1900 field (RTPS 2.3 §9.3.2 Time_t) that carries no explicit
// era number, by choosing the era — a multiple of 2^32 seconds, the NTP
// rollover period (era 0 ends, era 1 begins, at 7 Feb 2036) — whose decoded
// value lands nearest to ref.
//
// This is the standard NTP era-disambiguation technique (RFC 5905 §7.2):
// it recovers the sender's intended instant correctly as long as the
// sender's clock and ref are within half an era (~68 years) of each other,
// which holds for any real deployment. Without it, any secs value produced
// by marshalInfoTS's spec-mandated 2^32 wraparound after 2036-02-07 decodes
// back to a bogus 1900-era date instead of the correct post-2036 date.
func ntpEraDisambiguate(secs uint32, ns int64, ref time.Time) time.Time {
	const ntpDelta = 2208988800 // NTP epoch (1900) to Unix epoch (1970), seconds.
	const era = int64(1) << 32  // NTP seconds field rollover period.

	refNTPSec := ref.Unix() + ntpDelta
	diff := refNTPSec - int64(secs)
	// eraCount = round(diff / era), computed without float64 to avoid any
	// precision concern at these magnitudes; era/2 is exact since era is a
	// power of two.
	var eraCount int64
	if diff >= 0 {
		eraCount = (diff + era/2) / era
	} else {
		eraCount = -((-diff + era/2) / era)
	}
	ntpSec := int64(secs) + eraCount*era
	unixSec := ntpSec - ntpDelta
	return time.Unix(unixSec, ns).UTC()
}

// wrapInRTPSMessage wraps submessage bytes in an RTPS Header.
func wrapInRTPSMessage(prefix GuidPrefix, submessages []byte) []byte {
	h := Header{
		ProtocolVersion: [2]byte{2, 3},
		VendorId:        goVendorId,
		GuidPrefix:      prefix,
	}
	return append(marshalHeader(h), submessages...)
}

// cdrWrapPayload prepends the CDR_LE encapsulation header to a raw payload.
//
// Per RTPS 2.3 / DDS-XTypes §10.2.1 Table 10.1, the 2-octet
// representation_identifier is `typedef octet Identifier[2]` — a fixed
// octet array transmitted in the same wire order (first octet 0x00, second
// octet the scheme selector) regardless of which endianness the scheme
// itself selects for the payload that follows. It is NOT a little-endian
// uint16 encoding of the scheme constant.
func cdrWrapPayload(payload []byte) []byte {
	hdr := make([]byte, 4)
	binary.BigEndian.PutUint16(hdr[0:2], cdrLE) // wire bytes {0x00, 0x01}
	result := make([]byte, 4+len(payload))
	copy(result, hdr)
	copy(result[4:], payload)
	return result
}

// cdrUnwrapPayload strips the 4-byte CDR encapsulation header.
// Returns (payload, true) on success.
func cdrUnwrapPayload(b []byte) ([]byte, bool) {
	if len(b) < 4 {
		return nil, false
	}
	// The representation_identifier is a fixed-order 2-octet array (§10.2.1),
	// not an endianness-dependent uint16 — decode with BigEndian regardless
	// of the scheme it names.
	scheme := binary.BigEndian.Uint16(b[0:2])
	if scheme != cdrLE && scheme != plCDRLE {
		return nil, false
	}
	result := make([]byte, len(b)-4)
	copy(result, b[4:])
	return result, true
}
