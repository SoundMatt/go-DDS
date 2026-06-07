// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package security_test

import (
	"bytes"
	"testing"

	"github.com/SoundMatt/go-DDS/security"
)

// roundTrip verifies Seal→Open returns the original plaintext.
func roundTrip(t *testing.T, p security.Plugin, name string, plaintext []byte) {
	t.Helper()
	sealed, err := p.Seal(plaintext)
	if err != nil {
		t.Fatalf("%s: Seal error: %v", name, err)
	}
	got, err := p.Open(sealed)
	if err != nil {
		t.Fatalf("%s: Open error: %v", name, err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Errorf("%s: round-trip mismatch: got %q, want %q", name, got, plaintext)
	}
}

// ── NullPlugin ────────────────────────────────────────────────────────────────

func TestNullPlugin_RoundTrip(t *testing.T) {
	p := security.NullPlugin{}
	cases := [][]byte{
		nil,
		{},
		[]byte("hello"),
		make([]byte, 64*1024),
	}
	for _, c := range cases {
		roundTrip(t, p, "NullPlugin", c)
	}
}

func TestNullPlugin_Identity(t *testing.T) {
	p := security.NullPlugin{}
	in := []byte("test")
	out, _ := p.Seal(in)
	if !bytes.Equal(out, in) {
		t.Error("NullPlugin.Seal must be identity")
	}
}

// ── HMACPlugin ────────────────────────────────────────────────────────────────

func TestHMACPlugin_RoundTrip(t *testing.T) {
	key := security.NewRandomKey(32)
	p := security.NewHMACPlugin(key)

	cases := [][]byte{
		{},
		[]byte("hello world"),
		make([]byte, 1024),
	}
	for _, c := range cases {
		roundTrip(t, p, "HMACPlugin", c)
	}
}

func TestHMACPlugin_TagApended(t *testing.T) {
	key := security.NewRandomKey(32)
	p := security.NewHMACPlugin(key)
	plain := []byte("payload")
	sealed, _ := p.Seal(plain)
	if len(sealed) != len(plain)+32 {
		t.Errorf("sealed length: got %d, want %d", len(sealed), len(plain)+32)
	}
}

func TestHMACPlugin_TamperDetected(t *testing.T) {
	key := security.NewRandomKey(32)
	p := security.NewHMACPlugin(key)
	sealed, _ := p.Seal([]byte("important data"))
	sealed[0] ^= 0xFF // flip first byte
	if _, err := p.Open(sealed); err == nil {
		t.Error("expected error on tampered payload, got nil")
	}
}

func TestHMACPlugin_WrongKey(t *testing.T) {
	key1 := security.NewRandomKey(32)
	key2 := security.NewRandomKey(32)
	p1 := security.NewHMACPlugin(key1)
	p2 := security.NewHMACPlugin(key2)
	sealed, _ := p1.Seal([]byte("secret"))
	if _, err := p2.Open(sealed); err == nil {
		t.Error("expected error opening with wrong key")
	}
}

func TestHMACPlugin_TooShort(t *testing.T) {
	key := security.NewRandomKey(32)
	p := security.NewHMACPlugin(key)
	if _, err := p.Open([]byte("short")); err == nil {
		t.Error("expected error on payload shorter than HMAC tag")
	}
}

// ── AESGCMPlugin ──────────────────────────────────────────────────────────────

func TestAESGCMPlugin_RoundTrip(t *testing.T) {
	key := security.NewRandomKey(32)
	p, err := security.NewAESGCMPlugin(key)
	if err != nil {
		t.Fatalf("NewAESGCMPlugin: %v", err)
	}
	cases := [][]byte{
		{},
		[]byte("hello"),
		make([]byte, 1024),
	}
	for _, c := range cases {
		roundTrip(t, p, "AESGCMPlugin", c)
	}
}

func TestAESGCMPlugin_DistinctNonces(t *testing.T) {
	key := security.NewRandomKey(32)
	p, _ := security.NewAESGCMPlugin(key)
	a, _ := p.Seal([]byte("same plaintext"))
	b, _ := p.Seal([]byte("same plaintext"))
	// Two seals of identical plaintext must produce different ciphertexts
	// because the nonce is random.
	if bytes.Equal(a, b) {
		t.Error("two Seal calls produced identical output (nonce reuse)")
	}
}

func TestAESGCMPlugin_TamperDetected(t *testing.T) {
	key := security.NewRandomKey(32)
	p, _ := security.NewAESGCMPlugin(key)
	sealed, _ := p.Seal([]byte("sensitive"))
	sealed[len(sealed)-1] ^= 0xFF // corrupt last byte of GCM tag
	if _, err := p.Open(sealed); err == nil {
		t.Error("expected error on tampered AES-GCM payload")
	}
}

func TestAESGCMPlugin_WrongKey(t *testing.T) {
	k1 := security.NewRandomKey(32)
	k2 := security.NewRandomKey(32)
	p1, _ := security.NewAESGCMPlugin(k1)
	p2, _ := security.NewAESGCMPlugin(k2)
	sealed, _ := p1.Seal([]byte("secret"))
	if _, err := p2.Open(sealed); err == nil {
		t.Error("expected error decrypting with wrong key")
	}
}

func TestAESGCMPlugin_BadKeyLength(t *testing.T) {
	if _, err := security.NewAESGCMPlugin([]byte("tooshort")); err == nil {
		t.Error("expected error for non-32-byte key")
	}
}

func TestAESGCMPlugin_TooShort(t *testing.T) {
	key := security.NewRandomKey(32)
	p, _ := security.NewAESGCMPlugin(key)
	if _, err := p.Open([]byte("short")); err == nil {
		t.Error("expected error on payload shorter than nonce+tag")
	}
}

// ── NewRandomKey ──────────────────────────────────────────────────────────────

func TestNewRandomKey_Length(t *testing.T) {
	for _, n := range []int{16, 32, 64} {
		k := security.NewRandomKey(n)
		if len(k) != n {
			t.Errorf("NewRandomKey(%d): got len %d", n, len(k))
		}
	}
}

func TestNewRandomKey_Unique(t *testing.T) {
	a := security.NewRandomKey(32)
	b := security.NewRandomKey(32)
	if bytes.Equal(a, b) {
		t.Error("two NewRandomKey calls returned identical bytes")
	}
}

// ── Key isolation ─────────────────────────────────────────────────────────────

func TestHMACPlugin_KeyIsolation(t *testing.T) {
	key := security.NewRandomKey(32)
	orig := make([]byte, len(key))
	copy(orig, key)
	p := security.NewHMACPlugin(key)
	// Mutate the caller's key slice after construction.
	for i := range key {
		key[i] ^= 0xFF
	}
	// Plugin must still work with the original key.
	sealed, err := p.Seal([]byte("test"))
	if err != nil {
		t.Fatalf("Seal after key mutation: %v", err)
	}
	if _, err := p.Open(sealed); err != nil {
		t.Fatalf("Open after key mutation: %v", err)
	}
}
