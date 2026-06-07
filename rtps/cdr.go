// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// CDR/PL_CDR encoding used by RTPS discovery submessages (§10.2, §10.3).
//
// Only the little-endian variant (CDR_LE / PL_CDR_LE) is implemented here,
// which is the de-facto standard for modern RTPS implementations.

package rtps

import (
	"encoding/binary"
)

// CDR encapsulation scheme identifiers (§10.2 Table 10.1).
const (
	cdrLE   = uint16(0x0001) // CDR little-endian
	plCDRLE = uint16(0x0003) // PL_CDR little-endian
)

// Parameter IDs for SPDP ParticipantProxy and SEDP EndpointData (§9.6.3).
const (
	pidPad                         = uint16(0x0000)
	pidSentinel                    = uint16(0x0001)
	pidUserData                    = uint16(0x002C)
	pidTopicName                   = uint16(0x0005)
	pidTypeName                    = uint16(0x0007)
	pidProtocolVersion             = uint16(0x0015)
	pidVendorId                    = uint16(0x0016)
	pidMetatrafficUnicastLocator   = uint16(0x0032)
	pidMetatrafficMulticastLocator = uint16(0x0033)
	pidDefaultUnicastLocator       = uint16(0x002f)
	pidDefaultMulticastLocator     = uint16(0x0030)
	pidParticipantLeaseDuration    = uint16(0x0002)
	pidParticipantGUID             = uint16(0x0050)
	pidEndpointGUID                = uint16(0x005A)
	pidBuiltinEndpointSet          = uint16(0x0058)
	pidReliability                 = uint16(0x001A)
	pidDurability                  = uint16(0x001D)
	// pidDiscoveryToken is a vendor-specific PID (OMG vendor-extension range
	// 0x8000–0xBFFF) used to carry the SPDP discovery authentication tag
	// produced by a DiscoveryPlugin.
	pidDiscoveryToken = uint16(0x8001)
)

// plCDREncoder builds a PL_CDR_LE encoded parameter list.
type plCDREncoder struct {
	buf []byte
}

// newPLCDREncoder returns an encoder pre-seeded with the PL_CDR_LE header.
func newPLCDREncoder() *plCDREncoder {
	e := &plCDREncoder{}
	// Encapsulation header: scheme (2 bytes) + options (2 bytes, zero).
	e.buf = append(e.buf, byte(plCDRLE), byte(plCDRLE>>8), 0x00, 0x00)
	return e
}

// addParam appends a (PID, length, value) triple, padding value to 4 bytes.
func (e *plCDREncoder) addParam(pid uint16, value []byte) {
	padded := (len(value) + 3) &^ 3
	hdr := make([]byte, 4)
	binary.LittleEndian.PutUint16(hdr[0:], pid)
	binary.LittleEndian.PutUint16(hdr[2:], uint16(padded))
	e.buf = append(e.buf, hdr...)
	e.buf = append(e.buf, value...)
	for i := len(value); i < padded; i++ {
		e.buf = append(e.buf, 0x00)
	}
}

// addUint32 appends a 4-byte little-endian uint32 parameter.
func (e *plCDREncoder) addUint32(pid uint16, v uint32) {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, v)
	e.addParam(pid, b)
}

// addLocator appends a 24-byte Locator parameter.
func (e *plCDREncoder) addLocator(pid uint16, l Locator) {
	e.addParam(pid, marshalLocator(l))
}

// addGUID appends a 16-byte GUID parameter.
func (e *plCDREncoder) addGUID(pid uint16, g GUID) {
	b := make([]byte, 16)
	copy(b[0:12], g.Prefix[:])
	copy(b[12:16], g.Entity[:])
	e.addParam(pid, b)
}

// addString appends a CDR string (uint32 length + chars + null + padding).
func (e *plCDREncoder) addString(pid uint16, s string) {
	raw := make([]byte, 4+len(s)+1)
	binary.LittleEndian.PutUint32(raw[0:], uint32(len(s)+1))
	copy(raw[4:], s)
	// null terminator already zero from make
	e.addParam(pid, raw)
}

// finish appends the PID_SENTINEL and returns the encoded bytes.
func (e *plCDREncoder) finish() []byte {
	sentinel := []byte{byte(pidSentinel), byte(pidSentinel >> 8), 0x00, 0x00}
	return append(e.buf, sentinel...)
}

// plCDRDecoder iterates over PL_CDR_LE parameter lists.
type plCDRDecoder struct {
	buf []byte
	pos int
}

// newPLCDRDecoder creates a decoder; returns (nil, false) if header is invalid.
func newPLCDRDecoder(b []byte) (*plCDRDecoder, bool) {
	if len(b) < 4 {
		return nil, false
	}
	scheme := binary.LittleEndian.Uint16(b[0:2])
	if scheme != plCDRLE {
		return nil, false
	}
	return &plCDRDecoder{buf: b, pos: 4}, true
}

type param struct {
	pid   uint16
	value []byte
}

// next returns the next parameter, advancing the cursor.
// Returns (param{}, false) at PID_SENTINEL or on malformed input.
func (d *plCDRDecoder) next() (param, bool) {
	if d.pos+4 > len(d.buf) {
		return param{}, false
	}
	pid := binary.LittleEndian.Uint16(d.buf[d.pos:])
	length := int(binary.LittleEndian.Uint16(d.buf[d.pos+2:]))
	d.pos += 4
	if pid == pidSentinel {
		return param{}, false
	}
	if pid == pidPad {
		return d.next()
	}
	if d.pos+length > len(d.buf) {
		return param{}, false
	}
	v := d.buf[d.pos : d.pos+length]
	d.pos += length
	return param{pid: pid, value: v}, true
}

// decodeString reads a CDR string from a parameter value byte slice.
func decodeString(b []byte) (string, bool) {
	if len(b) < 4 {
		return "", false
	}
	n := int(binary.LittleEndian.Uint32(b[0:4]))
	if len(b) < 4+n {
		return "", false
	}
	s := b[4 : 4+n]
	// Strip null terminator.
	if len(s) > 0 && s[len(s)-1] == 0 {
		s = s[:len(s)-1]
	}
	return string(s), true
}

// decodeGUID reads a 16-byte GUID from a parameter value.
func decodeGUID(b []byte) (GUID, bool) {
	if len(b) < 16 {
		return GUID{}, false
	}
	var g GUID
	copy(g.Prefix[:], b[0:12])
	copy(g.Entity[:], b[12:16])
	return g, true
}
