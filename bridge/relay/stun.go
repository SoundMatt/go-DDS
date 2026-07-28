// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package relay

// STUN (RFC 5389) Binding Request/Response client, used for cloud↔edge peer
// discovery: a participant behind NAT sends a Binding Request to a STUN
// server and reads back its own server-reflexive (public) transport address
// from the response's XOR-MAPPED-ADDRESS attribute. That address can then be
// exchanged with a peer (e.g. via rtps.WithPeerLocators) to attempt a direct
// UDP connection, before falling back to the [Server] relay when direct
// connectivity isn't possible — the standard STUN-then-TURN pattern.
//
// Only the minimal subset of RFC 5389 needed for this is implemented: a
// Binding Request with no attributes, and parsing XOR-MAPPED-ADDRESS (falling
// back to the older, non-XOR MAPPED-ADDRESS for STUN servers that only send
// that) from the response. STUN's optional message-integrity/fingerprint
// attributes and long-term credential mechanism are not implemented — this
// client only needs address discovery, not a authenticated STUN session.

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"time"
)

// stunMagicCookie is the fixed RFC 5389 magic cookie, present in every STUN
// message header and used to XOR-obfuscate XOR-MAPPED-ADDRESS.
const stunMagicCookie uint32 = 0x2112A442

// STUN message types (RFC 5389 §6).
const (
	stunBindingRequest  uint16 = 0x0001
	stunBindingResponse uint16 = 0x0101
	stunBindingError    uint16 = 0x0111
)

// STUN attribute types (RFC 5389 §18.2).
const (
	stunAttrMappedAddress    uint16 = 0x0001
	stunAttrXorMappedAddress uint16 = 0x0020
)

// STUN address family tags (RFC 5389 §15.1).
const (
	stunFamilyIPv4 byte = 0x01
	stunFamilyIPv6 byte = 0x02
)

// ErrSTUNNoAddress is returned when a STUN Binding Response contained
// neither a MAPPED-ADDRESS nor an XOR-MAPPED-ADDRESS attribute.
var ErrSTUNNoAddress = errors.New("relay: STUN response carried no mapped address")

// ErrSTUNBindingError is returned when the server replies with a Binding
// Error Response (message type 0x0111) instead of a Binding Response.
var ErrSTUNBindingError = errors.New("relay: STUN server returned a binding error")

// defaultSTUNTimeout bounds how long Discover waits for a response before
// giving up, when ctx carries no earlier deadline.
const defaultSTUNTimeout = 5 * time.Second

// Discover sends a STUN (RFC 5389) Binding Request to stunServer ("host:port"
// over UDP) and returns this host's server-reflexive (NAT-mapped, public)
// address as observed by the STUN server. Returns an error if the server is
// unreachable, the response is malformed, times out, or carries no mapped
// address.
func Discover(ctx context.Context, stunServer string) (netip.AddrPort, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultSTUNTimeout)
		defer cancel()
	}

	raddr, err := net.ResolveUDPAddr("udp", stunServer)
	if err != nil {
		return netip.AddrPort{}, fmt.Errorf("relay: resolve STUN server %s: %w", stunServer, err)
	}
	conn, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		return netip.AddrPort{}, fmt.Errorf("relay: dial STUN server %s: %w", stunServer, err)
	}
	defer func() { _ = conn.Close() }()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}

	txID := make([]byte, 12)
	if _, rerr := rand.Read(txID); rerr != nil {
		return netip.AddrPort{}, fmt.Errorf("relay: generate STUN transaction ID: %w", rerr)
	}
	req := encodeSTUNBindingRequest(txID)
	if _, werr := conn.Write(req); werr != nil {
		return netip.AddrPort{}, fmt.Errorf("relay: send STUN request: %w", werr)
	}

	buf := make([]byte, 1500)
	n, err := conn.Read(buf)
	if err != nil {
		return netip.AddrPort{}, fmt.Errorf("relay: read STUN response: %w", err)
	}
	return decodeSTUNBindingResponse(buf[:n], txID)
}

// encodeSTUNBindingRequest builds a 20-byte STUN Binding Request header with
// no attributes: message type, zero length, magic cookie, and the given
// 12-byte transaction ID (RFC 5389 §6).
func encodeSTUNBindingRequest(txID []byte) []byte {
	b := make([]byte, 20)
	binary.BigEndian.PutUint16(b[0:2], stunBindingRequest)
	binary.BigEndian.PutUint16(b[2:4], 0) // message length: no attributes
	binary.BigEndian.PutUint32(b[4:8], stunMagicCookie)
	copy(b[8:20], txID)
	return b
}

// decodeSTUNBindingResponse parses a STUN message, verifies it is a Binding
// Response matching txID, and extracts the mapped address, preferring
// XOR-MAPPED-ADDRESS (RFC 5389) over the legacy MAPPED-ADDRESS (RFC 3489)
// when both are present.
func decodeSTUNBindingResponse(b []byte, txID []byte) (netip.AddrPort, error) {
	if len(b) < 20 {
		return netip.AddrPort{}, fmt.Errorf("relay: STUN response too short: %d bytes", len(b))
	}
	msgType := binary.BigEndian.Uint16(b[0:2])
	msgLen := binary.BigEndian.Uint16(b[2:4])
	cookie := binary.BigEndian.Uint32(b[4:8])
	respTxID := b[8:20]
	if cookie != stunMagicCookie {
		return netip.AddrPort{}, fmt.Errorf("relay: STUN response: bad magic cookie %#x", cookie)
	}
	for i := range txID {
		if respTxID[i] != txID[i] {
			return netip.AddrPort{}, errors.New("relay: STUN response: transaction ID mismatch")
		}
	}
	if msgType == stunBindingError {
		return netip.AddrPort{}, ErrSTUNBindingError
	}
	if msgType != stunBindingResponse {
		return netip.AddrPort{}, fmt.Errorf("relay: STUN response: unexpected message type %#x", msgType)
	}
	if 20+int(msgLen) > len(b) {
		return netip.AddrPort{}, fmt.Errorf("relay: STUN response: message length %d exceeds packet", msgLen)
	}

	attrs := b[20 : 20+int(msgLen)]
	var mapped, xorMapped *netip.AddrPort
	pos := 0
	for pos+4 <= len(attrs) {
		attrType := binary.BigEndian.Uint16(attrs[pos : pos+2])
		attrLen := int(binary.BigEndian.Uint16(attrs[pos+2 : pos+4]))
		pos += 4
		if pos+attrLen > len(attrs) {
			break // truncated attribute; stop parsing what we have
		}
		val := attrs[pos : pos+attrLen]
		switch attrType {
		case stunAttrXorMappedAddress:
			if ap, ok := decodeXorMappedAddress(val, txID); ok {
				xorMapped = &ap
			}
		case stunAttrMappedAddress:
			if ap, ok := decodeMappedAddress(val); ok {
				mapped = &ap
			}
		}
		// Attributes are padded to a 4-byte boundary (RFC 5389 §15).
		pos += (attrLen + 3) &^ 3
	}

	if xorMapped != nil {
		return *xorMapped, nil
	}
	if mapped != nil {
		return *mapped, nil
	}
	return netip.AddrPort{}, ErrSTUNNoAddress
}

// decodeMappedAddress parses a (non-XOR) MAPPED-ADDRESS attribute value.
func decodeMappedAddress(v []byte) (netip.AddrPort, bool) {
	if len(v) < 4 {
		return netip.AddrPort{}, false
	}
	family := v[1]
	port := binary.BigEndian.Uint16(v[2:4])
	switch family {
	case stunFamilyIPv4:
		if len(v) < 8 {
			return netip.AddrPort{}, false
		}
		addr := netip.AddrFrom4([4]byte(v[4:8]))
		return netip.AddrPortFrom(addr, port), true
	case stunFamilyIPv6:
		if len(v) < 20 {
			return netip.AddrPort{}, false
		}
		addr := netip.AddrFrom16([16]byte(v[4:20]))
		return netip.AddrPortFrom(addr, port), true
	default:
		return netip.AddrPort{}, false
	}
}

// decodeXorMappedAddress parses an XOR-MAPPED-ADDRESS attribute value
// (RFC 5389 §15.2): the port is XORed with the high 16 bits of the magic
// cookie, and the address is XORed with the magic cookie (IPv4) or the magic
// cookie followed by the transaction ID (IPv6).
func decodeXorMappedAddress(v []byte, txID []byte) (netip.AddrPort, bool) {
	if len(v) < 4 {
		return netip.AddrPort{}, false
	}
	family := v[1]
	xport := binary.BigEndian.Uint16(v[2:4])
	port := xport ^ uint16(stunMagicCookie>>16)

	var cookieBytes [4]byte
	binary.BigEndian.PutUint32(cookieBytes[:], stunMagicCookie)

	switch family {
	case stunFamilyIPv4:
		if len(v) < 8 {
			return netip.AddrPort{}, false
		}
		var raw [4]byte
		for i := 0; i < 4; i++ {
			raw[i] = v[4+i] ^ cookieBytes[i]
		}
		return netip.AddrPortFrom(netip.AddrFrom4(raw), port), true
	case stunFamilyIPv6:
		if len(v) < 20 {
			return netip.AddrPort{}, false
		}
		xorKey := append(append([]byte(nil), cookieBytes[:]...), txID...) // 16 bytes
		var raw [16]byte
		for i := 0; i < 16; i++ {
			raw[i] = v[4+i] ^ xorKey[i]
		}
		return netip.AddrPortFrom(netip.AddrFrom16(raw), port), true
	default:
		return netip.AddrPort{}, false
	}
}
