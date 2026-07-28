// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// CDR/PL_CDR encoding used by RTPS discovery submessages (§10.2, §10.3).
//
// Only the little-endian variant (CDR_LE / PL_CDR_LE) is implemented here,
// which is the de-facto standard for modern RTPS implementations.

package rtps

//fusa:req REQ-CDR-001
//fusa:req REQ-CDR-002
//fusa:req REQ-CDR-003

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
	// pidEndpointToken is a vendor-specific PID used to carry the SEDP endpoint
	// authentication tag produced by an EndpointPlugin.
	pidEndpointToken = uint16(0x8002)
	// pidTCPLocator is a vendor-specific PID (Milestone 14) carrying the
	// sending participant's RTPS-over-TCP listen locator (LocatorKindTCPv4),
	// when the RTPS-over-TCP transport is enabled via WithTCPAddr. Peers that
	// don't understand it skip it like any other unrecognised PL_CDR parameter.
	pidTCPLocator = uint16(0x8003)

	// The following vendor-specific PIDs (Milestone 14, "QoS Enforcement —
	// Active Policy") carry the subset of QoS state that must cross the wire
	// for a remote peer to actively enforce Partition, Ownership, and
	// Liveliness. They live in the vendor-extension range for the same reason
	// pidTCPLocator does: unlike pidReliability/pidDurability above (defined
	// but not yet wired to any encoder — reserved for a future PR), these are
	// go-DDS-specific wire representations, not a claim of exact OMG PID
	// values. Peers that don't understand a PID skip it like any other
	// unrecognised PL_CDR parameter, so this is fully backward compatible.
	//
	// pidPartition carries one Partition QoS name per occurrence (a SEDP
	// EndpointData may contain zero or more of these); present on both
	// publication and subscription announcements.
	pidPartition = uint16(0x8004)
	// pidOwnership is present (with a non-zero value) only when the writer's
	// Ownership QoS is ExclusiveOwnership. Absence means SharedOwnership.
	pidOwnership = uint16(0x8005)
	// pidOwnershipStrength carries the writer's OwnershipStrength as a
	// little-endian uint32. Only meaningful alongside pidOwnership.
	pidOwnershipStrength = uint16(0x8006)
	// pidLivelinessLeaseDuration carries a writer's QoS.LivelinessLeaseDuration
	// as a little-endian int64 count of nanoseconds (time.Duration's native
	// representation). Present only on publication announcements, and only
	// when the writer set a non-zero lease.
	pidLivelinessLeaseDuration = uint16(0x8007)
	// pidRelayID is a vendor-specific PID (Milestone 15, "NAT Traversal /
	// Cloud Gateway") carrying the sending participant's relay registration
	// ID (see WithRelayAddr / transport_relay.go), when the RTPS-over-Relay
	// transport is enabled. It is a plain CDR string, not a Locator: unlike
	// pidTCPLocator, a relay peer generally has no directly reachable
	// network address at all (that is the point of the relay), so there is
	// no host:port to encode — only an opaque ID the relay server uses to
	// address this participant. Peers that don't understand it skip it like
	// any other unrecognised PL_CDR parameter.
	pidRelayID = uint16(0x8008)
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

// addInt64 appends an 8-byte little-endian int64 parameter (used to carry a
// time.Duration's nanosecond count — see pidLivelinessLeaseDuration).
func (e *plCDREncoder) addInt64(pid uint16, v int64) {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, uint64(v))
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

// addBytes appends an arbitrary byte-slice parameter.
func (e *plCDREncoder) addBytes(pid uint16, v []byte) {
	e.addParam(pid, v)
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

// decodeUint32 reads a 4-byte little-endian uint32 from a parameter value.
func decodeUint32(b []byte) (uint32, bool) {
	if len(b) < 4 {
		return 0, false
	}
	return binary.LittleEndian.Uint32(b), true
}

// decodeInt64 reads an 8-byte little-endian int64 from a parameter value.
func decodeInt64(b []byte) (int64, bool) {
	if len(b) < 8 {
		return 0, false
	}
	return int64(binary.LittleEndian.Uint64(b)), true
}
