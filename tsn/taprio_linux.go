// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

//go:build linux

package tsn

//fusa:req REQ-TSN-007

import (
	"encoding/binary"
	"fmt"
	"math"
	"net"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

// Apply programs the TAPRIO qdisc on c.Interface by sending an RTM_NEWQDISC
// netlink message to the kernel. It replaces any existing root qdisc.
//
// Requires CAP_NET_ADMIN. Returns ErrNotSupported on non-Linux platforms (see
// taprio_stub.go).
func (c *TAPRIOConfig) Apply() error {
	if err := c.Validate(); err != nil {
		return err
	}

	iface, err := net.InterfaceByName(c.Interface)
	if err != nil {
		return fmt.Errorf("tsn: taprio: interface %q: %w", c.Interface, err)
	}
	if iface.Index > math.MaxUint32 {
		return fmt.Errorf("tsn: taprio: interface index %d overflows uint32", iface.Index)
	}

	msg, err := buildTAPRIOMsg(c, iface.Index)
	if err != nil {
		return fmt.Errorf("tsn: taprio: build message: %w", err)
	}

	fd, err := unix.Socket(unix.AF_NETLINK, unix.SOCK_RAW|unix.SOCK_CLOEXEC, unix.NETLINK_ROUTE)
	if err != nil {
		return fmt.Errorf("tsn: taprio: netlink socket: %w", err)
	}
	defer unix.Close(fd) //nolint:errcheck // netlink fd cleanup; close errors are not actionable after the request is complete

	sa := &unix.SockaddrNetlink{Family: unix.AF_NETLINK}
	if bindErr := unix.Bind(fd, sa); bindErr != nil {
		return fmt.Errorf("tsn: taprio: bind: %w", bindErr)
	}

	written, err := unix.Write(fd, msg)
	_ = written
	if err != nil {
		return fmt.Errorf("tsn: taprio: send: %w", err)
	}

	return recvACK(fd)
}

// VerifyApplied queries the kernel via RTM_GETQDISC to confirm that a TAPRIO
// qdisc is the root qdisc on c.Interface. Returns nil if "taprio" is found,
// or an error describing the mismatch. Call this after Apply to confirm the
// schedule was accepted by the kernel.
func (c *TAPRIOConfig) VerifyApplied() error {
	if c.Interface == "" {
		return fmt.Errorf("tsn: VerifyApplied: Interface must not be empty")
	}
	iface, err := net.InterfaceByName(c.Interface)
	if err != nil {
		return fmt.Errorf("tsn: VerifyApplied: interface %q: %w", c.Interface, err)
	}

	fd, err := unix.Socket(unix.AF_NETLINK, unix.SOCK_RAW|unix.SOCK_CLOEXEC, unix.NETLINK_ROUTE)
	if err != nil {
		return fmt.Errorf("tsn: VerifyApplied: netlink socket: %w", err)
	}
	defer unix.Close(fd) //nolint:errcheck // netlink fd cleanup; close errors are not actionable after the request is complete

	sa := &unix.SockaddrNetlink{Family: unix.AF_NETLINK}
	if bindErr := unix.Bind(fd, sa); bindErr != nil {
		return fmt.Errorf("tsn: VerifyApplied: bind: %w", bindErr)
	}

	req := buildGetQdiscMsg(uint32(iface.Index))
	if _, writeErr := unix.Write(fd, req); writeErr != nil {
		return fmt.Errorf("tsn: VerifyApplied: send: %w", writeErr)
	}

	return readQdiscKind(fd)
}

func buildGetQdiscMsg(ifindex uint32) []byte {
	// tcmsg: only ifindex matters for a per-interface dump
	tcmsg := make([]byte, 20)
	tcmsg[0] = unix.AF_UNSPEC
	binary.LittleEndian.PutUint32(tcmsg[4:], ifindex)

	const (
		rtmGetQdisc = 38
		nlmFRequest = 0x01
		nlmFDump    = 0x300 // NLM_F_ROOT | NLM_F_MATCH
	)
	hdr := make([]byte, 16)
	total := uint32(16 + len(tcmsg))
	binary.LittleEndian.PutUint32(hdr[0:], total)
	binary.LittleEndian.PutUint16(hdr[4:], rtmGetQdisc)
	binary.LittleEndian.PutUint16(hdr[6:], nlmFRequest|nlmFDump)
	binary.LittleEndian.PutUint32(hdr[8:], 2)  // seq
	binary.LittleEndian.PutUint32(hdr[12:], 0) // pid (kernel)
	return append(hdr, tcmsg...)
}

// readQdiscKind reads RTM_GETQDISC response messages and returns nil if any
// root qdisc is "taprio", or an error if none is found.
func readQdiscKind(fd int) error {
	buf := make([]byte, 32768)
	for {
		n, err := unix.Read(fd, buf)
		if err != nil {
			return fmt.Errorf("tsn: VerifyApplied: read: %w", err)
		}
		data := buf[:n]
		for len(data) >= 16 {
			msgLen := binary.LittleEndian.Uint32(data[0:4])
			msgType := binary.LittleEndian.Uint16(data[4:6])
			const (
				nlmsgError  = 2
				nlmsgDone   = 3
				rtmNewQdisc = 36
			)
			switch msgType {
			case nlmsgDone:
				return fmt.Errorf("tsn: VerifyApplied: no taprio qdisc found on interface")
			case nlmsgError:
				if len(data) >= 20 {
					code := int32(binary.LittleEndian.Uint32(data[16:20]))
					if code != 0 {
						return fmt.Errorf("tsn: VerifyApplied: kernel error: %w", syscall.Errno(-code))
					}
				}
				return fmt.Errorf("tsn: VerifyApplied: no taprio qdisc found on interface")
			case rtmNewQdisc:
				if int(msgLen) >= 16+20 {
					kind := extractTCAKind(data[16+20 : msgLen])
					if kind == "taprio" {
						return nil
					}
				}
			}
			advance := (msgLen + 3) &^ 3
			if advance < 16 || int(advance) >= len(data) {
				break
			}
			data = data[advance:]
		}
	}
}

// extractTCAKind walks a netlink attribute list and returns the value of TCA_KIND (type 1).
func extractTCAKind(attrs []byte) string {
	for len(attrs) >= 4 {
		attrLen := binary.LittleEndian.Uint16(attrs[0:2])
		attrType := binary.LittleEndian.Uint16(attrs[2:4]) & 0x7FFF // mask NLA_F_NESTED
		if attrLen < 4 || int(attrLen) > len(attrs) {
			break
		}
		if attrType == 1 { // TCA_KIND
			kind := string(attrs[4:attrLen])
			return strings.TrimRight(kind, "\x00")
		}
		advance := (int(attrLen) + 3) &^ 3
		if advance >= len(attrs) {
			break
		}
		attrs = attrs[advance:]
	}
	return ""
}

// buildTAPRIOMsg constructs the RTM_NEWQDISC netlink message.
func buildTAPRIOMsg(c *TAPRIOConfig, ifindex int) ([]byte, error) {
	nb := newNlBuf()

	// tcmsg: family=AF_UNSPEC, ifindex, handle=0x00010000, parent=TC_H_ROOT
	const tcHRoot = uint32(0xFFFFFFFF)
	tcmsg := make([]byte, 20)
	tcmsg[0] = unix.AF_UNSPEC                                 // family
	binary.LittleEndian.PutUint32(tcmsg[4:], uint32(ifindex)) // ifindex
	binary.LittleEndian.PutUint32(tcmsg[8:], 0x00010000)      // handle: 1:0
	binary.LittleEndian.PutUint32(tcmsg[12:], tcHRoot)        // parent: root
	nb.addRaw(tcmsg)

	// TCA_KIND = "taprio\x00"
	nb.addAttr(1 /* TCA_KIND */, append([]byte("taprio"), 0x00))

	// TCA_OPTIONS (nested TAPRIO attributes)
	opts := newNlBuf()
	opts.addAttrU32(11 /* TCA_TAPRIO_ATTR_FLAGS */, taprioFlags(c.Offload))
	opts.addAttrS32(6 /* TCA_TAPRIO_ATTR_SCHED_CLOCKID */, 11 /* CLOCK_TAI */)

	baseTime := c.BaseTime
	if baseTime == 0 {
		// Let the kernel schedule starting from now + one cycle
		baseTime = 0
	}
	opts.addAttrS64(3 /* TCA_TAPRIO_ATTR_SCHED_BASE_TIME */, baseTime)

	cycleNs := int64(c.CycleDuration())
	if cycleNs > 0 {
		opts.addAttrS64(9 /* TCA_TAPRIO_ATTR_SCHED_CYCLE_TIME */, cycleNs)
	}

	// TCA_TAPRIO_ATTR_SCHED_ENTRY_LIST (nested list)
	entryList := newNlBuf()
	for _, e := range c.Entries {
		entry := newNlBuf()
		entry.addAttrU8(3 /* TCA_TAPRIO_SCHED_ENTRY_CMD */, 0x00 /* GATE_OP */)
		entry.addAttrU32(4 /* TCA_TAPRIO_SCHED_ENTRY_GATE_MASK */, uint32(e.GateMask))
		entry.addAttrU32(5 /* TCA_TAPRIO_SCHED_ENTRY_INTERVAL */, uint32(e.Interval.Nanoseconds()))
		entryList.addNestedAttr(2 /* TCA_TAPRIO_ATTR_SCHED_SINGLE_ENTRY */, entry.bytes())
	}
	opts.addNestedAttr(2 /* TCA_TAPRIO_ATTR_SCHED_ENTRY_LIST */, entryList.bytes())

	nb.addNestedAttr(2 /* TCA_OPTIONS */, opts.bytes())

	// Wrap in nlmsghdr
	const (
		rtmNewQdisc = 36
		nlmFRequest = 0x01
		nlmFAck     = 0x04
		nlmFCreate  = 0x400
		nlmFReplace = 0x100
	)
	payload := nb.bytes()
	hdr := make([]byte, 16)
	total := uint32(16 + len(payload))
	binary.LittleEndian.PutUint32(hdr[0:], total)
	binary.LittleEndian.PutUint16(hdr[4:], rtmNewQdisc)
	binary.LittleEndian.PutUint16(hdr[6:], nlmFRequest|nlmFAck|nlmFCreate|nlmFReplace)
	binary.LittleEndian.PutUint32(hdr[8:], 1)  // seq
	binary.LittleEndian.PutUint32(hdr[12:], 0) // pid (kernel)
	return append(hdr, payload...), nil
}

func taprioFlags(offload bool) uint32 {
	if offload {
		return 0x2 // TC_TAPRIO_ATTR_FLAG_FULL_OFFLOAD
	}
	return 0x0
}

// recvACK waits for the netlink ACK (NLMSG_ERROR with error code 0).
func recvACK(fd int) error {
	buf := make([]byte, 4096)
	n, err := unix.Read(fd, buf)
	if err != nil {
		return fmt.Errorf("tsn: taprio: recv ack: %w", err)
	}
	if n < 20 {
		return fmt.Errorf("tsn: taprio: ack too short (%d bytes)", n)
	}
	// nlmsghdr.type must be NLMSG_ERROR (2)
	msgType := binary.LittleEndian.Uint16(buf[4:6])
	if msgType != 2 /* NLMSG_ERROR */ {
		return fmt.Errorf("tsn: taprio: unexpected ack type %d", msgType)
	}
	// error field is int32 at offset 16
	code := int32(binary.LittleEndian.Uint32(buf[16:20]))
	if code != 0 {
		return fmt.Errorf("tsn: taprio: kernel error: %w", syscall.Errno(-code))
	}
	return nil
}

// ── Netlink attribute builder ─────────────────────────────────────────────────

type nlBuf struct {
	b []byte
}

func newNlBuf() *nlBuf { return &nlBuf{} }

func (n *nlBuf) addRaw(b []byte) {
	n.b = append(n.b, b...)
}

func (n *nlBuf) addAttr(attrType uint16, val []byte) {
	// nlattr: len(2) + type(2) + val + padding to 4-byte boundary
	length := uint16(4 + len(val))
	hdr := make([]byte, 4)
	binary.LittleEndian.PutUint16(hdr[0:], length)
	binary.LittleEndian.PutUint16(hdr[2:], attrType)
	n.b = append(n.b, hdr...)
	n.b = append(n.b, val...)
	pad := (4 - (len(val) % 4)) % 4
	for i := 0; i < pad; i++ {
		n.b = append(n.b, 0)
	}
}

func (n *nlBuf) addAttrU8(attrType uint16, v uint8) {
	n.addAttr(attrType, []byte{v})
}

func (n *nlBuf) addAttrU32(attrType uint16, v uint32) {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, v)
	n.addAttr(attrType, b)
}

func (n *nlBuf) addAttrS32(attrType uint16, v int32) {
	n.addAttrU32(attrType, uint32(v))
}

func (n *nlBuf) addAttrS64(attrType uint16, v int64) {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, uint64(v))
	n.addAttr(attrType, b)
}

func (n *nlBuf) addNestedAttr(attrType uint16, nested []byte) {
	// NLA_F_NESTED = 0x8000
	n.addAttr(attrType|0x8000, nested)
}

func (n *nlBuf) bytes() []byte {
	return n.b
}
