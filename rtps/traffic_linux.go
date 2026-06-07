// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

//go:build linux

package rtps

import (
	"encoding/binary"
	"net"
	"syscall"
	"time"
	"unsafe"
)

// Linux socket / cmsg constants not exported by the syscall package.
const (
	// clockTAI is CLOCK_TAI (11) — the International Atomic Time clock.
	// Used as the reference for SO_TXTIME / ETF qdisc scheduling.
	clockTAI = 11

	// soTxTime is SO_TXTIME (61), enabling launchtime packet scheduling.
	// Requires Linux ≥ 4.19 and an ETF or taprio qdisc on the egress interface.
	soTxTime = 61

	// scmTxTime is SCM_TXTIME (61), the cmsg type that carries the
	// scheduled transmit timestamp (uint64 nanoseconds since TAI epoch).
	scmTxTime = 61
)

// sockTxTime is the argument to setsockopt(SO_TXTIME).
// Mirrors struct sock_txtime from <linux/net_tstamp.h>.
type sockTxTime struct {
	Clockid int32
	Flags   uint32
}

// withFd extracts the raw file descriptor from conn and calls fn with it.
func withFd(conn *net.UDPConn, fn func(fd int) error) error {
	rawConn, err := conn.SyscallConn()
	if err != nil {
		return err
	}
	var fnErr error
	rawConn.Control(func(fd uintptr) {
		fnErr = fn(int(fd))
	})
	return fnErr
}

// setSockPriority sets SO_PRIORITY on conn. The Linux kernel maps priority
// values to traffic classes via tc / qdisc rules; values 0–7 directly
// correspond to VLAN PCP when the egress interface is configured for it.
func setSockPriority(conn *net.UDPConn, priority int) error {
	return withFd(conn, func(fd int) error {
		return syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_PRIORITY, priority)
	})
}

// setSockTOS sets the IP ToS byte to encode the DSCP value.
// dscp is the 6-bit DSCP code point (0–63); it is shifted left by 2 to
// produce the full 8-bit ToS byte (ECN bits remain 0).
func setSockTOS(conn *net.UDPConn, dscp uint8) error {
	return withFd(conn, func(fd int) error {
		return syscall.SetsockoptInt(fd, syscall.IPPROTO_IP, syscall.IP_TOS, int(dscp<<2))
	})
}

// enableTxTime enables SO_TXTIME on conn using CLOCK_TAI.
// Requires Linux ≥ 4.19 and an ETF or taprio qdisc on the egress NIC.
// Returns a non-nil error (silently ignored by the caller) on older kernels.
func enableTxTime(conn *net.UDPConn) error {
	cfg := sockTxTime{Clockid: clockTAI, Flags: 0}
	return withFd(conn, func(fd int) error {
		_, _, errno := syscall.RawSyscall6(
			syscall.SYS_SETSOCKOPT,
			uintptr(fd),
			syscall.SOL_SOCKET,
			uintptr(soTxTime),
			uintptr(unsafe.Pointer(&cfg)),
			unsafe.Sizeof(cfg),
			0,
		)
		if errno != 0 {
			return errno
		}
		return nil
	})
}

// clockTAINow returns the current CLOCK_TAI time.
// CLOCK_TAI runs at the same rate as UTC but is not adjusted for leap seconds,
// making it suitable as the reference clock for TSN scheduled transmit.
// Falls back to time.Now() if the kernel clock is unavailable.
func clockTAINow() (time.Time, error) {
	var ts syscall.Timespec
	// syscall.ClockGettime is not available on all Linux build configurations;
	// use a raw syscall instead for portability across Go toolchain versions.
	_, _, errno := syscall.RawSyscall(syscall.SYS_CLOCK_GETTIME,
		uintptr(clockTAI), uintptr(unsafe.Pointer(&ts)), 0)
	if errno != 0 {
		return time.Now(), errno
	}
	return time.Unix(ts.Sec, ts.Nsec), nil
}

// buildTxTimeCmsg builds a cmsg carrying a SCM_TXTIME control message.
// txTimeNS is the scheduled transmit time in nanoseconds since the TAI epoch.
func buildTxTimeCmsg(txTimeNS uint64) []byte {
	cmsg := make([]byte, syscall.CmsgSpace(8))
	h := (*syscall.Cmsghdr)(unsafe.Pointer(&cmsg[0]))
	h.Level = syscall.SOL_SOCKET
	h.Type = scmTxTime
	h.SetLen(syscall.CmsgLen(8))
	// SCM_TXTIME expects the timestamp in host native byte order.
	// Use NativeEndian (not LittleEndian) so the code is correct on both
	// little-endian (amd64, arm64) and big-endian (s390x, ppc64) Linux targets.
	binary.NativeEndian.PutUint64(cmsg[syscall.SizeofCmsghdr:], txTimeNS)
	return cmsg
}

// scheduledSend transmits data to dst at the scheduled TAI time txTimeNS via
// sendmsg with a SCM_TXTIME control message. The ETF or taprio qdisc on the
// egress NIC holds the packet in a time-sorted queue until the scheduled time.
//
// If SO_TXTIME is not enabled on conn (e.g. on a non-TSN interface), this
// falls back to a plain WriteToUDP.
func scheduledSend(conn *net.UDPConn, dst *net.UDPAddr, data []byte, txTimeNS uint64) error {
	if txTimeNS == 0 {
		_, err := conn.WriteToUDP(data, dst)
		return err
	}
	ip4 := dst.IP.To4()
	if ip4 == nil {
		// IPv6 path: fall back to ordinary send (SCM_TXTIME with IPv6 needs
		// a slightly different cmsg layout; not implemented in v0.5).
		_, err := conn.WriteToUDP(data, dst)
		return err
	}
	sa := &syscall.SockaddrInet4{Port: dst.Port}
	copy(sa.Addr[:], ip4)
	cmsg := buildTxTimeCmsg(txTimeNS)
	return withFd(conn, func(fd int) error {
		return syscall.Sendmsg(fd, data, cmsg, sa, 0)
	})
}
