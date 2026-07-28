// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package rtps

//fusa:req REQ-LOC-001
//fusa:req REQ-LOC-002
//fusa:req REQ-LOC-003

import (
	"encoding/binary"
	"net"
	"strconv"
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
	// LocatorKindTCPv4 identifies a go-DDS TCP/TLS unicast locator
	// (Milestone 14, RTPS-over-TCP). This is a go-DDS vendor extension: it is
	// only ever carried inside the vendor-specific pidTCPLocator parameter, so
	// a peer that doesn't recognise it simply skips the parameter — PL_CDR
	// parameter lists are self-describing (§9.4.2.11) and require no changes
	// on peers that don't support TCP.
	LocatorKindTCPv4 = 4
	// LocatorKindQUICv4 identifies a go-DDS RTPS-over-QUIC unicast locator
	// (Milestone 16, ROADMAP.md "QUIC Transport"). Like LocatorKindTCPv4, this
	// is a go-DDS vendor extension only ever carried inside the
	// vendor-specific pidQUICLocator parameter.
	LocatorKindQUICv4 = 5
)

// locatorFromUDP builds a Locator from a net.UDPAddr.
// IPv4 addresses are encoded in bytes 12–15 of the 16-byte address field.
// IPv6 addresses occupy the full 16-byte field.
func locatorFromUDP(addr *net.UDPAddr, port int) Locator {
	if ip6 := addr.IP.To16(); ip6 != nil && addr.IP.To4() == nil {
		return locatorFromUDPv6(addr, port)
	}
	l := Locator{Kind: LocatorKindUDPv4, Port: uint32(port)}
	ip4 := addr.IP.To4()
	if ip4 == nil {
		ip4 = net.IPv4zero.To4()
	}
	copy(l.Address[12:], ip4)
	return l
}

// locatorFromUDPv6 builds a UDPv6 Locator. The full 16-byte IPv6 address
// is stored directly in the Address field (RTPS 2.3 §9.3.2).
func locatorFromUDPv6(addr *net.UDPAddr, port int) Locator {
	l := Locator{Kind: LocatorKindUDPv6, Port: uint32(port)}
	ip6 := addr.IP.To16()
	if ip6 == nil {
		ip6 = net.IPv6zero
	}
	copy(l.Address[:], ip6)
	return l
}

// udpAddr converts a Locator to a *net.UDPAddr.
// Returns nil for locators with an unsupported or invalid kind.
func (l Locator) udpAddr() *net.UDPAddr {
	switch l.Kind {
	case LocatorKindUDPv4:
		return &net.UDPAddr{
			IP:   net.IP(append([]byte(nil), l.Address[12:16]...)),
			Port: int(l.Port),
		}
	case LocatorKindUDPv6:
		return &net.UDPAddr{
			IP:   net.IP(append([]byte(nil), l.Address[:]...)),
			Port: int(l.Port),
		}
	default:
		return nil
	}
}

// locatorFromTCP builds a TCPv4 Locator from an IP and port. Like
// locatorFromUDP, a zero/unset ip encodes as 0.0.0.0; the receiving peer
// fills in the sender's observed address in that case (see
// parseParticipantData).
func locatorFromTCP(ip net.IP, port int) Locator {
	l := Locator{Kind: LocatorKindTCPv4, Port: uint32(port)}
	ip4 := ip.To4()
	if ip4 == nil {
		ip4 = net.IPv4zero.To4()
	}
	copy(l.Address[12:], ip4)
	return l
}

// tcpHostPort converts a TCPv4 Locator to a "host:port" string suitable for
// tcpSocket.send. Returns ("", false) for locators of any other kind.
func (l Locator) tcpHostPort() (string, bool) {
	if l.Kind != LocatorKindTCPv4 {
		return "", false
	}
	ip := net.IP(append([]byte(nil), l.Address[12:16]...))
	return net.JoinHostPort(ip.String(), strconv.Itoa(int(l.Port))), true
}

// locatorFromQUIC builds a QUICv4 Locator from an IP and port, the QUIC
// analogue of locatorFromTCP.
func locatorFromQUIC(ip net.IP, port int) Locator {
	l := Locator{Kind: LocatorKindQUICv4, Port: uint32(port)}
	ip4 := ip.To4()
	if ip4 == nil {
		ip4 = net.IPv4zero.To4()
	}
	copy(l.Address[12:], ip4)
	return l
}

// quicHostPort converts a QUICv4 Locator to a "host:port" string suitable
// for quicSocket.send. Returns ("", false) for locators of any other kind.
func (l Locator) quicHostPort() (string, bool) {
	if l.Kind != LocatorKindQUICv4 {
		return "", false
	}
	ip := net.IP(append([]byte(nil), l.Address[12:16]...))
	return net.JoinHostPort(ip.String(), strconv.Itoa(int(l.Port))), true
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

// spdpMulticastAddr is the standard RTPS IPv4 discovery multicast group.
var spdpMulticastAddr = net.ParseIP("239.255.0.1")

// spdpMulticastAddrV6 is the RTPS IPv6 discovery multicast group (site-local).
var spdpMulticastAddrV6 = net.ParseIP("FF03::1")

// userDataMulticastAddr is the go-DDS user-data multicast group (domain-scoped).
// One multicast packet from a writer reaches all subscribers in the same domain
// without N separate unicast sends.
var userDataMulticastAddr = net.ParseIP("239.255.0.2")

// userMulticastPort returns the user-data multicast receive port for a domain.
// This is port_base + domain_gain*domain + 1, i.e., the standard RTPS
// user-data multicast port (§9.6.1, Pb + DG*d + d_port_user_multicast).
func userMulticastPort(domain int) int {
	return portBase + domainGain*domain + 1
}

// portBase constants from the RTPS port mapping formula (§9.6.1).
const (
	portBase        = 7400
	domainGain      = 250
	participantGain = 2
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
