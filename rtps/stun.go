// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package rtps

// Minimal STUN (RFC 5389) Binding Request/Response client backing
// WithSTUNServer. Implements independently of bridge/relay.Discover — same
// wire protocol, same "no cross-module dependency" precedent as the rest of
// this file's transport code (see transport_relay.go's package doc
// comment) — so this root module never imports the bridge submodule.
//
// Only Binding Request/Response and XOR-MAPPED-ADDRESS/MAPPED-ADDRESS
// parsing are implemented; see bridge/relay/stun.go for the fuller
// documentation of what RFC 5389 subset this covers and why.

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

const stunMagicCookie uint32 = 0x2112A442

const (
	stunBindingRequest  uint16 = 0x0001
	stunBindingResponse uint16 = 0x0101
	stunBindingError    uint16 = 0x0111
)

const (
	stunAttrMappedAddress    uint16 = 0x0001
	stunAttrXorMappedAddress uint16 = 0x0020
)

const (
	stunFamilyIPv4 byte = 0x01
	stunFamilyIPv6 byte = 0x02
)

var errSTUNNoAddress = errors.New("rtps: STUN response carried no mapped address")

const stunDiscoverTimeout = 5 * time.Second

// stunDiscover sends a STUN Binding Request to stunServer ("host:port" over
// UDP) and returns this host's server-reflexive (public) address as
// observed by the STUN server.
func stunDiscover(ctx context.Context, stunServer string) (netip.AddrPort, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, stunDiscoverTimeout)
		defer cancel()
	}

	raddr, err := net.ResolveUDPAddr("udp", stunServer)
	if err != nil {
		return netip.AddrPort{}, fmt.Errorf("rtps: resolve STUN server %s: %w", stunServer, err)
	}
	conn, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		return netip.AddrPort{}, fmt.Errorf("rtps: dial STUN server %s: %w", stunServer, err)
	}
	defer func() { _ = conn.Close() }()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}

	txID := make([]byte, 12)
	if _, rerr := rand.Read(txID); rerr != nil {
		return netip.AddrPort{}, fmt.Errorf("rtps: generate STUN transaction ID: %w", rerr)
	}
	req := make([]byte, 20)
	binary.BigEndian.PutUint16(req[0:2], stunBindingRequest)
	binary.BigEndian.PutUint32(req[4:8], stunMagicCookie)
	copy(req[8:20], txID)
	if _, werr := conn.Write(req); werr != nil {
		return netip.AddrPort{}, fmt.Errorf("rtps: send STUN request: %w", werr)
	}

	buf := make([]byte, 1500)
	n, err := conn.Read(buf)
	if err != nil {
		return netip.AddrPort{}, fmt.Errorf("rtps: read STUN response: %w", err)
	}
	return decodeSTUNResponse(buf[:n], txID)
}

func decodeSTUNResponse(b []byte, txID []byte) (netip.AddrPort, error) {
	if len(b) < 20 {
		return netip.AddrPort{}, fmt.Errorf("rtps: STUN response too short: %d bytes", len(b))
	}
	msgType := binary.BigEndian.Uint16(b[0:2])
	msgLen := binary.BigEndian.Uint16(b[2:4])
	cookie := binary.BigEndian.Uint32(b[4:8])
	if cookie != stunMagicCookie {
		return netip.AddrPort{}, fmt.Errorf("rtps: STUN response: bad magic cookie %#x", cookie)
	}
	respTxID := b[8:20]
	for i := range txID {
		if respTxID[i] != txID[i] {
			return netip.AddrPort{}, errors.New("rtps: STUN response: transaction ID mismatch")
		}
	}
	if msgType == stunBindingError {
		return netip.AddrPort{}, errors.New("rtps: STUN server returned a binding error")
	}
	if msgType != stunBindingResponse {
		return netip.AddrPort{}, fmt.Errorf("rtps: STUN response: unexpected message type %#x", msgType)
	}
	if 20+int(msgLen) > len(b) {
		return netip.AddrPort{}, fmt.Errorf("rtps: STUN response: message length %d exceeds packet", msgLen)
	}

	attrs := b[20 : 20+int(msgLen)]
	var mapped, xorMapped *netip.AddrPort
	pos := 0
	for pos+4 <= len(attrs) {
		attrType := binary.BigEndian.Uint16(attrs[pos : pos+2])
		attrLen := int(binary.BigEndian.Uint16(attrs[pos+2 : pos+4]))
		pos += 4
		if pos+attrLen > len(attrs) {
			break
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
		pos += (attrLen + 3) &^ 3
	}

	if xorMapped != nil {
		return *xorMapped, nil
	}
	if mapped != nil {
		return *mapped, nil
	}
	return netip.AddrPort{}, errSTUNNoAddress
}

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
		return netip.AddrPortFrom(netip.AddrFrom4([4]byte(v[4:8])), port), true
	case stunFamilyIPv6:
		if len(v) < 20 {
			return netip.AddrPort{}, false
		}
		return netip.AddrPortFrom(netip.AddrFrom16([16]byte(v[4:20])), port), true
	default:
		return netip.AddrPort{}, false
	}
}

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
		xorKey := append(append([]byte(nil), cookieBytes[:]...), txID...)
		var raw [16]byte
		for i := 0; i < 16; i++ {
			raw[i] = v[4+i] ^ xorKey[i]
		}
		return netip.AddrPortFrom(netip.AddrFrom16(raw), port), true
	default:
		return netip.AddrPort{}, false
	}
}
