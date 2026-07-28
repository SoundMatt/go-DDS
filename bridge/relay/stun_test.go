// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package relay

import (
	"context"
	"encoding/binary"
	"net"
	"net/netip"
	"testing"
	"time"
)

// fakeSTUNServer is a minimal, protocol-correct RFC 5389 STUN server: it
// replies to a Binding Request with a Binding Response carrying the
// request's observed source address as an XOR-MAPPED-ADDRESS attribute.
// Used so [Discover] can be tested end-to-end without reaching a real STUN
// server on the network.
type fakeSTUNServer struct {
	conn *net.UDPConn
	done chan struct{}
}

func startFakeSTUNServer(t *testing.T, respondWithLegacyMappedAddress bool) *fakeSTUNServer {
	t.Helper()
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Skipf("ListenUDP: %v", err)
	}
	s := &fakeSTUNServer{conn: conn, done: make(chan struct{})}
	go s.serve(t, respondWithLegacyMappedAddress)
	t.Cleanup(func() {
		close(s.done)
		_ = conn.Close()
	})
	return s
}

func (s *fakeSTUNServer) addr() string { return s.conn.LocalAddr().String() }

func (s *fakeSTUNServer) serve(t *testing.T, legacy bool) {
	buf := make([]byte, 1500)
	for {
		n, from, err := s.conn.ReadFromUDP(buf)
		if err != nil {
			select {
			case <-s.done:
				return
			default:
				continue
			}
		}
		req := buf[:n]
		if len(req) < 20 {
			continue
		}
		txID := req[8:20]
		resp := encodeFakeSTUNResponse(txID, from, legacy)
		_, _ = s.conn.WriteToUDP(resp, from)
	}
}

// encodeFakeSTUNResponse builds a Binding Response carrying from's address
// either as XOR-MAPPED-ADDRESS (legacy=false) or the older MAPPED-ADDRESS
// (legacy=true), exercising both of Discover's decode paths.
func encodeFakeSTUNResponse(txID []byte, from *net.UDPAddr, legacy bool) []byte {
	ip4 := from.IP.To4()
	var attrType uint16
	var addrBytes []byte
	var port uint16

	if legacy {
		attrType = stunAttrMappedAddress
		addrBytes = append([]byte(nil), ip4...)
		port = uint16(from.Port)
	} else {
		attrType = stunAttrXorMappedAddress
		var cookieBytes [4]byte
		binary.BigEndian.PutUint32(cookieBytes[:], stunMagicCookie)
		xored := make([]byte, 4)
		for i := 0; i < 4; i++ {
			xored[i] = ip4[i] ^ cookieBytes[i]
		}
		addrBytes = xored
		port = uint16(from.Port) ^ uint16(stunMagicCookie>>16)
	}

	attrVal := make([]byte, 4+len(addrBytes))
	attrVal[0] = 0x00
	attrVal[1] = stunFamilyIPv4
	binary.BigEndian.PutUint16(attrVal[2:4], port)
	copy(attrVal[4:], addrBytes)

	attr := make([]byte, 4+len(attrVal))
	binary.BigEndian.PutUint16(attr[0:2], attrType)
	binary.BigEndian.PutUint16(attr[2:4], uint16(len(attrVal)))
	copy(attr[4:], attrVal)

	msg := make([]byte, 20+len(attr))
	binary.BigEndian.PutUint16(msg[0:2], stunBindingResponse)
	binary.BigEndian.PutUint16(msg[2:4], uint16(len(attr)))
	binary.BigEndian.PutUint32(msg[4:8], stunMagicCookie)
	copy(msg[8:20], txID)
	copy(msg[20:], attr)
	return msg
}

func TestDiscover_XorMappedAddress(t *testing.T) {
	srv := startFakeSTUNServer(t, false)

	ap, err := Discover(context.Background(), srv.addr())
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if !ap.Addr().Is4() || ap.Addr().As4() != [4]byte{127, 0, 0, 1} {
		t.Errorf("Addr = %v, want 127.0.0.1", ap.Addr())
	}
	if ap.Port() == 0 {
		t.Error("Port = 0, want the client's ephemeral source port")
	}
}

func TestDiscover_LegacyMappedAddress(t *testing.T) {
	srv := startFakeSTUNServer(t, true)

	ap, err := Discover(context.Background(), srv.addr())
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if ap.Addr().As4() != [4]byte{127, 0, 0, 1} {
		t.Errorf("Addr = %v, want 127.0.0.1", ap.Addr())
	}
}

func TestDiscover_Timeout(t *testing.T) {
	// A UDP socket with nobody listening: the request is sent but no
	// response ever arrives, so Discover must return once its deadline
	// elapses rather than hanging forever.
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Skipf("ListenUDP: %v", err)
	}
	addr := conn.LocalAddr().String()
	_ = conn.Close() // nobody home

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := Discover(ctx, addr); err == nil {
		t.Fatal("expected an error from an unreachable STUN server")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("Discover took %v, want it to respect the context deadline", elapsed)
	}
}

func TestDiscover_UnresolvableServer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := Discover(ctx, "not a host:port"); err == nil {
		t.Fatal("expected a resolve error")
	}
}

func TestDecodeSTUNBindingResponse_TransactionIDMismatch(t *testing.T) {
	txID := make([]byte, 12)
	other := make([]byte, 12)
	other[0] = 0xFF
	resp := encodeFakeSTUNResponse(other, &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345}, false)
	if _, err := decodeSTUNBindingResponse(resp, txID); err == nil {
		t.Fatal("expected a transaction ID mismatch error")
	}
}

func TestDecodeSTUNBindingResponse_BindingError(t *testing.T) {
	txID := make([]byte, 12)
	msg := make([]byte, 20)
	binary.BigEndian.PutUint16(msg[0:2], stunBindingError)
	binary.BigEndian.PutUint32(msg[4:8], stunMagicCookie)
	copy(msg[8:20], txID)
	if _, err := decodeSTUNBindingResponse(msg, txID); err != ErrSTUNBindingError {
		t.Fatalf("err = %v, want ErrSTUNBindingError", err)
	}
}

func TestDecodeSTUNBindingResponse_NoAddress(t *testing.T) {
	txID := make([]byte, 12)
	msg := make([]byte, 20)
	binary.BigEndian.PutUint16(msg[0:2], stunBindingResponse)
	binary.BigEndian.PutUint32(msg[4:8], stunMagicCookie)
	copy(msg[8:20], txID)
	if _, err := decodeSTUNBindingResponse(msg, txID); err != ErrSTUNNoAddress {
		t.Fatalf("err = %v, want ErrSTUNNoAddress", err)
	}
}

func TestDecodeSTUNBindingResponse_TooShort(t *testing.T) {
	if _, err := decodeSTUNBindingResponse([]byte{1, 2, 3}, make([]byte, 12)); err == nil {
		t.Fatal("expected an error for a too-short response")
	}
}

func TestDecodeXorMappedAddress_IPv6(t *testing.T) {
	txID := make([]byte, 12)
	for i := range txID {
		txID[i] = byte(i)
	}
	addr := netip.MustParseAddr("2001:db8::1")
	raw := addr.As16()

	var cookieBytes [4]byte
	binary.BigEndian.PutUint32(cookieBytes[:], stunMagicCookie)
	xorKey := append(append([]byte(nil), cookieBytes[:]...), txID...)
	xored := make([]byte, 16)
	for i := 0; i < 16; i++ {
		xored[i] = raw[i] ^ xorKey[i]
	}

	v := make([]byte, 4+16)
	v[1] = stunFamilyIPv6
	binary.BigEndian.PutUint16(v[2:4], 4242^uint16(stunMagicCookie>>16))
	copy(v[4:], xored)

	ap, ok := decodeXorMappedAddress(v, txID)
	if !ok {
		t.Fatal("decodeXorMappedAddress: ok = false")
	}
	if ap.Addr() != addr {
		t.Errorf("Addr = %v, want %v", ap.Addr(), addr)
	}
	if ap.Port() != 4242 {
		t.Errorf("Port = %d, want 4242", ap.Port())
	}
}

func TestDecodeMappedAddress_Malformed(t *testing.T) {
	if _, ok := decodeMappedAddress([]byte{0x00}); ok {
		t.Error("expected ok = false for a too-short value")
	}
	if _, ok := decodeMappedAddress([]byte{0x00, 0x03, 0x00, 0x00}); ok {
		t.Error("expected ok = false for an unknown address family")
	}
}
