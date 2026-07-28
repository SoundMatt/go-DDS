// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Tests for newMulticastReceiveSocket/newMulticastReceiveSocketV6's
// unicast-bind fallback when no multicast-capable interface can even be
// enumerated (ROADMAP.md Milestone 16, "WebAssembly Target") — discovered
// while proving a real GOOS=wasip1 GOARCH=wasm build under a WASI runtime
// (Wasmtime, with wasi:sockets enabled): net.Interfaces() returns nothing
// there, and firstMulticastInterface's resulting error was previously
// treated as fatal by newMulticastReceiveSocket even though its own doc
// comment already promised a unicast fallback for "no multicast-capable
// interface" — the same fallback that already existed for a successful
// interface lookup whose ListenMulticastUDP call itself then failed. These
// tests use *WithIface's injected interface-lookup function to simulate the
// "no interface" case directly, without needing an actual WASI host.

package rtps

//fusa:test REQ-TRANS-020

import (
	"errors"
	"net"
	"testing"
)

func TestNewMulticastReceiveSocket_NoInterfaceEnumerated_FallsBackToUnicast(t *testing.T) {
	findIface := func() (*net.Interface, error) {
		return nil, errors.New("simulated: no interfaces (e.g. WASI wasi:sockets)")
	}
	sock, mcastOK, err := newMulticastReceiveSocketWithIface(spdpMulticastAddr, 0, findIface)
	if err != nil {
		t.Fatalf("newMulticastReceiveSocketWithIface: %v", err)
	}
	defer sock.close()
	if mcastOK {
		t.Error("mcastOK: got true, want false (unicast fallback was used)")
	}
	if sock.conn == nil {
		t.Error("conn: got nil, want a bound unicast UDP connection")
	}
}

func TestNewMulticastReceiveSocketV6_NoInterfaceEnumerated_FallsBackToUnicast(t *testing.T) {
	findIface := func() (*net.Interface, error) {
		return nil, errors.New("simulated: no interfaces (e.g. WASI wasi:sockets)")
	}
	sock, mcastOK, err := newMulticastReceiveSocketV6WithIface(spdpMulticastAddrV6, 0, findIface)
	if err != nil {
		t.Skipf("newMulticastReceiveSocketV6WithIface: %v — IPv6 unicast bind unavailable", err)
	}
	defer sock.close()
	if mcastOK {
		t.Error("mcastOK: got true, want false (unicast fallback was used)")
	}
	if sock.conn == nil {
		t.Error("conn: got nil, want a bound unicast UDP connection")
	}
}

// TestNewMulticastReceiveSocket_InterfaceFoundButJoinFails_StillFallsBack
// covers the other existing fallback path (interface enumeration succeeds,
// but the corresponding multicast join fails) still behaves identically
// after the refactor that added *WithIface — a bogus, non-multicast-capable
// interface is a reliable way to force ListenMulticastUDP to fail without
// depending on the host's real network configuration.
func TestNewMulticastReceiveSocket_InterfaceFoundButJoinFails_StillFallsBack(t *testing.T) {
	findIface := func() (*net.Interface, error) {
		// No real host has thousands of interfaces, so this index is
		// reliably rejected by net.ListenMulticastUDP as nonexistent,
		// forcing the join itself (not the lookup) to fail.
		return &net.Interface{Index: 999999, Name: "bogus999999"}, nil
	}
	sock, mcastOK, err := newMulticastReceiveSocketWithIface(spdpMulticastAddr, 0, findIface)
	if err != nil {
		t.Fatalf("newMulticastReceiveSocketWithIface: %v", err)
	}
	defer sock.close()
	if mcastOK {
		t.Error("mcastOK: got true, want false (unicast fallback was used)")
	}
}
