// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package rtps

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
// (common in containers and macOS VMs) it falls back to a plain unicast bind
// on the same port. The socket then works for intra-process delivery; SPDP
// peer discovery across network boundaries is simply disabled in that case.
func newMulticastReceiveSocket(group net.IP, port int) (*udpSocket, error) {
	iface, _ := firstMulticastInterface() // nil iface is OK: OS picks
	conn, err := net.ListenMulticastUDP("udp4", iface, &net.UDPAddr{IP: group, Port: port})
	if err == nil {
		return newSocket(conn, port), nil
	}
	// Multicast unavailable — bind unicast as a no-op receiver so the
	// participant can start. Intra-process pub/sub is unaffected.
	conn2, err2 := net.ListenUDP("udp4", &net.UDPAddr{Port: port})
	if err2 != nil {
		return nil, fmt.Errorf("rtps: multicast receive %s:%d: %w", group, port, err)
	}
	return newSocket(conn2, port), nil
}

// newMulticastReceiveSocketV6 joins an IPv6 multicast group on the given port.
// Falls back to a plain unicast bind on the same port when no IPv6 multicast
// interface is available (containers, CI environments).
func newMulticastReceiveSocketV6(group net.IP, port int) (*udpSocket, error) {
	iface, _ := firstIPv6MulticastInterface()
	conn, err := net.ListenMulticastUDP("udp6", iface, &net.UDPAddr{IP: group, Port: port})
	if err == nil {
		return newSocket(conn, port), nil
	}
	conn2, err2 := net.ListenUDP("udp6", &net.UDPAddr{Port: port})
	if err2 != nil {
		return nil, fmt.Errorf("rtps: IPv6 multicast receive %s:%d: %w", group, port, err)
	}
	return newSocket(conn2, port), nil
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
			addrs, _ := iface.Addrs()
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
