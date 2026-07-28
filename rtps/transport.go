// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package rtps

//fusa:req REQ-TRANS-001
//fusa:req REQ-TRANS-002
//fusa:req REQ-TRANS-003
//fusa:req REQ-TRANS-020

import (
	"fmt"
	"net"
	"time"
)

const maxUDPSize = 65535

// udpPacket is a received UDP datagram with sender address.
type udpPacket struct {
	data []byte
	from *net.UDPAddr
}

// udpSocket wraps a UDP connection with an async receive loop.
type udpSocket struct {
	conn *net.UDPConn
	port int
	recv chan udpPacket
	done chan struct{}
}

// newUnicastSocket binds to 0.0.0.0:<port> (IPv4). If that port is in use it
// tries port+1 … port+15 before giving up.
func newUnicastSocket(port int) (*udpSocket, error) {
	var conn *net.UDPConn
	usedPort := port
	for i := 0; i < 16; i++ {
		var err error
		conn, err = net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: port + i})
		if err == nil {
			usedPort = port + i
			break
		}
	}
	if conn == nil {
		return nil, fmt.Errorf("rtps: no free UDP port in range [%d, %d)", port, port+16)
	}
	return newSocket(conn, usedPort), nil
}

// newUnicastSocketV6 binds to [::]:<port> (IPv6 dual-stack). It tries up to 16
// sequential ports before giving up.
func newUnicastSocketV6(port int) (*udpSocket, error) {
	var conn *net.UDPConn
	usedPort := port
	for i := 0; i < 16; i++ {
		var err error
		conn, err = net.ListenUDP("udp6", &net.UDPAddr{IP: net.IPv6zero, Port: port + i})
		if err == nil {
			usedPort = port + i
			break
		}
	}
	if conn == nil {
		return nil, fmt.Errorf("rtps: no free UDP6 port in range [%d, %d)", port, port+16)
	}
	return newSocket(conn, usedPort), nil
}

// newMulticastReceiveSocket creates a socket that receives on the given
// multicast group and port. If the OS has no multicast-capable interface
// (common in containers and macOS VMs) — including a host that fails to
// enumerate any interface at all, e.g. net.Interfaces() under a WASI
// runtime's `wasi:sockets` (ROADMAP.md Milestone 16 "WebAssembly Target":
// GOOS=wasip1 GOARCH=wasm running under Wasmtime, Fastly Compute, or a
// similar edge/cloud-function sandbox, none of which expose interface
// enumeration) — it falls back to a plain unicast bind on the same port.
// The socket then works for intra-process delivery; SPDP peer discovery
// across network boundaries is simply disabled in that case (a dial-only
// WS/TCP/QUIC peer list — see WithWSPeers et al. — still works normally,
// since that path never depends on multicast). The returned bool reports
// whether a genuine multicast join succeeded (false whenever either
// fallback was used) — participant.go uses this to decide whether UDP
// multicast is "available" for the RTPS-over-TCP fallback (Milestone 14;
// see participant.preferTCP).
func newMulticastReceiveSocket(group net.IP, port int) (*udpSocket, bool, error) {
	return newMulticastReceiveSocketWithIface(group, port, firstMulticastInterface)
}

// newMulticastReceiveSocketWithIface is newMulticastReceiveSocket with its
// interface-lookup step injected, so a test can simulate "no interface
// could even be enumerated" (net.Interfaces() itself failing or returning
// nothing, as under a WASI runtime's `wasi:sockets`) without needing an
// actual such host.
func newMulticastReceiveSocketWithIface(group net.IP, port int, findIface func() (*net.Interface, error)) (*udpSocket, bool, error) {
	iface, ifaceErr := findIface() // nil iface is OK: OS picks
	if ifaceErr == nil {
		if conn, err := net.ListenMulticastUDP("udp4", iface, &net.UDPAddr{IP: group, Port: port}); err == nil {
			return newSocket(conn, port), true, nil
		}
	}
	// Multicast unavailable — either no multicast-capable interface exists
	// (or could even be enumerated) or the join itself failed — bind
	// unicast as a no-op receiver so the participant can still start.
	// Intra-process pub/sub is unaffected.
	conn2, err2 := net.ListenUDP("udp4", &net.UDPAddr{Port: port})
	if err2 != nil {
		if ifaceErr != nil {
			return nil, false, fmt.Errorf("rtps: multicast receive %s:%d: %w", group, port, ifaceErr)
		}
		return nil, false, fmt.Errorf("rtps: multicast receive %s:%d: %w", group, port, err2)
	}
	return newSocket(conn2, port), false, nil
}

// newMulticastReceiveSocketV6 joins an IPv6 multicast group on the given port.
// Falls back to a plain unicast bind on the same port when no IPv6 multicast
// interface is available or none can be enumerated at all (containers, CI
// environments, WASI sandboxes — see newMulticastReceiveSocket). The
// returned bool reports whether a genuine multicast join succeeded, as with
// newMulticastReceiveSocket.
func newMulticastReceiveSocketV6(group net.IP, port int) (*udpSocket, bool, error) {
	return newMulticastReceiveSocketV6WithIface(group, port, firstIPv6MulticastInterface)
}

// newMulticastReceiveSocketV6WithIface is newMulticastReceiveSocketV6 with
// its interface-lookup step injected — see
// newMulticastReceiveSocketWithIface.
func newMulticastReceiveSocketV6WithIface(group net.IP, port int, findIface func() (*net.Interface, error)) (*udpSocket, bool, error) {
	iface, ifaceErr := findIface()
	if ifaceErr == nil {
		if conn, err := net.ListenMulticastUDP("udp6", iface, &net.UDPAddr{IP: group, Port: port}); err == nil {
			return newSocket(conn, port), true, nil
		}
	}
	conn2, err2 := net.ListenUDP("udp6", &net.UDPAddr{Port: port})
	if err2 != nil {
		if ifaceErr != nil {
			return nil, false, fmt.Errorf("rtps: IPv6 multicast receive %s:%d: %w", group, port, ifaceErr)
		}
		return nil, false, fmt.Errorf("rtps: IPv6 multicast receive %s:%d: %w", group, port, err2)
	}
	return newSocket(conn2, port), false, nil
}

// firstIPv6MulticastInterface returns the first UP, non-loopback IPv6
// multicast interface, falling back to loopback.
func firstIPv6MulticastInterface() (*net.Interface, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	for _, iface := range ifaces {
		flags := iface.Flags
		if flags&net.FlagUp != 0 && flags&net.FlagMulticast != 0 && flags&net.FlagLoopback == 0 {
			addrs, err := iface.Addrs()
			if err != nil {
				return nil, err
			}
			for _, a := range addrs {
				if ipnet, ok := a.(*net.IPNet); ok && ipnet.IP.To4() == nil && ipnet.IP.To16() != nil {
					return &iface, nil
				}
			}
		}
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp != 0 && iface.Flags&net.FlagMulticast != 0 {
			return &iface, nil
		}
	}
	return nil, fmt.Errorf("rtps: no IPv6 multicast-capable interface")
}

func newSocket(conn *net.UDPConn, port int) *udpSocket {
	s := &udpSocket{
		conn: conn,
		port: port,
		recv: make(chan udpPacket, 256),
		done: make(chan struct{}),
	}
	go s.readLoop()
	return s
}

func (s *udpSocket) readLoop() {
	buf := make([]byte, maxUDPSize)
	for {
		_ = s.conn.SetReadDeadline(time.Now().Add(250 * time.Millisecond))
		n, from, err := s.conn.ReadFromUDP(buf)
		if err != nil {
			select {
			case <-s.done:
				return
			default:
				continue
			}
		}
		data := make([]byte, n)
		copy(data, buf[:n])
		select {
		case s.recv <- udpPacket{data: data, from: from}:
		default: // slow consumer; drop
		}
	}
}

func (s *udpSocket) send(dst *net.UDPAddr, data []byte) error {
	_, err := s.conn.WriteToUDP(data, dst)
	return err
}

func (s *udpSocket) close() {
	close(s.done)
	_ = s.conn.Close()
}

// firstMulticastInterface returns the first UP, non-loopback multicast
// interface, falling back to loopback so tests run without a LAN.
func firstMulticastInterface() (*net.Interface, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	for _, iface := range ifaces {
		flags := iface.Flags
		if flags&net.FlagUp != 0 && flags&net.FlagMulticast != 0 && flags&net.FlagLoopback == 0 {
			return &iface, nil
		}
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp != 0 && iface.Flags&net.FlagMulticast != 0 {
			return &iface, nil
		}
	}
	return nil, fmt.Errorf("rtps: no multicast-capable interface")
}
