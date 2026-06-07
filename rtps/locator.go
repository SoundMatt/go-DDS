// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package rtps

import (
	"encoding/binary"
	"net"
)

// Locator_t: 24-byte transport endpoint address (RTPS 2.3 §9.3.2).
type Locator struct {
	Kind    int32
	Port    uint32
	Address [16]byte
}

const (
	LocatorKindInvalid = -1
	LocatorKindUDPv4   = 1
	LocatorKindUDPv6   = 2
)

// locatorFromUDP builds a Locator from a net.UDPAddr.
// IPv4 addresses are encoded in bytes 12–15 of the 16-byte address field.
func locatorFromUDP(addr *net.UDPAddr, port int) Locator {
	l := Locator{Kind: LocatorKindUDPv4, Port: uint32(port)}
	ip4 := addr.IP.To4()
	if ip4 == nil {
		ip4 = net.IPv4zero.To4()
	}
	copy(l.Address[12:], ip4)
	return l
}

// udpAddr converts a Locator to a *net.UDPAddr. Returns nil for non-UDPv4.
func (l Locator) udpAddr() *net.UDPAddr {
	if l.Kind != LocatorKindUDPv4 {
		return nil
	}
	return &net.UDPAddr{
		IP:   net.IP(l.Address[12:16]),
		Port: int(l.Port),
	}
}

// marshalLocator serialises a Locator into 24 little-endian bytes.
func marshalLocator(l Locator) []byte {
	b := make([]byte, 24)
	binary.LittleEndian.PutUint32(b[0:], uint32(l.Kind))
	binary.LittleEndian.PutUint32(b[4:], l.Port)
	copy(b[8:], l.Address[:])
	return b
}

// unmarshalLocator parses 24 little-endian bytes into a Locator.
func unmarshalLocator(b []byte) (Locator, bool) {
	if len(b) < 24 {
		return Locator{}, false
	}
	var l Locator
	l.Kind = int32(binary.LittleEndian.Uint32(b[0:]))
	l.Port = binary.LittleEndian.Uint32(b[4:])
	copy(l.Address[:], b[8:24])
	return l, true
}

// spdpMulticastAddr is the standard RTPS discovery multicast group.
var spdpMulticastAddr = net.ParseIP("239.255.0.1")

// portBase constants from the RTPS port mapping formula (§9.6.1).
const (
	portBase          = 7400
	domainGain        = 250
	participantGain   = 2
)

// metaMulticastPort returns the SPDP multicast port for a domain.
func metaMulticastPort(domain int) int {
	return portBase + domainGain*domain
}

// metaUnicastPort returns the participant's metadata unicast port.
func metaUnicastPort(domain, participantIdx int) int {
	return portBase + domainGain*domain + 10 + participantGain*participantIdx
}

// userUnicastPort returns the participant's user-data unicast port.
func userUnicastPort(domain, participantIdx int) int {
	return portBase + domainGain*domain + 11 + participantGain*participantIdx
}
