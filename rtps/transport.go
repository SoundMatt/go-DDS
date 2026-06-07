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

// newUnicastSocket binds to 0.0.0.0:<port>. If that port is in use it tries
// port+1 … port+15 before giving up.
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

// newMulticastReceiveSocket creates a socket that receives on the given
// multicast group and port. net.ListenMulticastUDP handles the kernel-level
// IP_ADD_MEMBERSHIP without needing platform-specific syscalls.
func newMulticastReceiveSocket(group net.IP, port int) (*udpSocket, error) {
	iface, err := firstMulticastInterface()
	if err != nil {
		// nil interface tells the OS to pick; this works on loopback for tests.
		iface = nil
	}
	conn, err := net.ListenMulticastUDP("udp4", iface, &net.UDPAddr{IP: group, Port: port})
	if err != nil {
		return nil, fmt.Errorf("rtps: multicast receive %s:%d: %w", group, port, err)
	}
	return newSocket(conn, port), nil
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
		s.conn.SetReadDeadline(time.Now().Add(250 * time.Millisecond))
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
	s.conn.Close()
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
