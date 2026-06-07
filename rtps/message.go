// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// RTPS message framing: Header, SubmessageHeader, and DATA submessage
// as defined in RTPS 2.3 §9.4.

package rtps

import (
	"encoding/binary"
	"fmt"
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
	submsgDATA    = byte(0x15)
	submsgINFO_TS = byte(0x09)
)

// flag bits for DATA submessage (§9.4.5.3).
const (
	flagEndianness = byte(0x01) // E: 1 = little-endian
	flagInlineQos  = byte(0x02) // Q: inline QoS present
	flagData       = byte(0x04) // D: serialised payload present
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
func parseSubmessages(body []byte, fn func(id, flags byte, body []byte) error) error {
	for len(body) >= 4 {
		id := body[0]
		flags := body[1]
		length := int(binary.LittleEndian.Uint16(body[2:4]))
		body = body[4:]
		if length > len(body) {
			return fmt.Errorf("rtps: submessage length %d exceeds remaining bytes %d", length, len(body))
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
func cdrWrapPayload(payload []byte) []byte {
	hdr := []byte{byte(cdrLE), byte(cdrLE >> 8), 0x00, 0x00}
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
	scheme := binary.LittleEndian.Uint16(b[0:2])
	if scheme != cdrLE && scheme != plCDRLE {
		return nil, false
	}
	result := make([]byte, len(b)-4)
	copy(result, b[4:])
	return result, true
}
