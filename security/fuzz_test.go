// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package security_test

// Fuzz tests for the security package plugins.
//
// Run the fuzzer with e.g.:
//
//	go test -fuzz=FuzzHMACRoundTrip    -fuzztime=60s ./security/...
//	go test -fuzz=FuzzAESGCMRoundTrip  -fuzztime=60s ./security/...
//	go test -fuzz=FuzzHMACOpenArbitrary   -fuzztime=60s ./security/...
//	go test -fuzz=FuzzAESGCMOpenArbitrary -fuzztime=60s ./security/...

import (
	"bytes"
	"testing"

	"github.com/SoundMatt/go-DDS/security"
)

// FuzzHMACRoundTrip checks that Seal→Open is an identity for any plaintext.
func FuzzHMACRoundTrip(f *testing.F) {
	f.Add([]byte(""))
	f.Add([]byte("hello world"))
	f.Add([]byte{0x00})
	f.Add([]byte{0xFF, 0xFE, 0x00, 0x01})
	f.Add(make([]byte, 1024))

	key := security.NewRandomKey(32)
	p := security.NewHMACPlugin(key)

	f.Fuzz(func(t *testing.T, plaintext []byte) {
		sealed, err := p.Seal(plaintext)
		if err != nil {
			t.Fatalf("Seal: %v", err)
		}
		got, err := p.Open(sealed)
		if err != nil {
			t.Fatalf("Open after Seal: %v", err)
		}
		if !bytes.Equal(got, plaintext) {
			t.Errorf("round-trip mismatch: got %q, want %q", got, plaintext)
		}
	})
}

// FuzzAESGCMRoundTrip checks that Seal→Open is an identity for any plaintext.
func FuzzAESGCMRoundTrip(f *testing.F) {
	f.Add([]byte(""))
	f.Add([]byte("hello world"))
	f.Add([]byte{0x00})
	f.Add([]byte{0xFF, 0xFE, 0x00, 0x01})
	f.Add(make([]byte, 1024))

	key := security.NewRandomKey(32)
	p, err := security.NewAESGCMPlugin(key)
	if err != nil {
		f.Fatalf("NewAESGCMPlugin: %v", err)
	}

	f.Fuzz(func(t *testing.T, plaintext []byte) {
		sealed, err := p.Seal(plaintext)
		if err != nil {
			t.Fatalf("Seal: %v", err)
		}
		got, err := p.Open(sealed)
		if err != nil {
			t.Fatalf("Open after Seal: %v", err)
		}
		if !bytes.Equal(got, plaintext) {
			t.Errorf("round-trip mismatch: got %q, want %q", got, plaintext)
		}
	})
}

// FuzzHMACOpenArbitrary feeds arbitrary bytes to HMACPlugin.Open and verifies
// it never panics (invalid inputs must return an error, not panic).
func FuzzHMACOpenArbitrary(f *testing.F) {
	f.Add([]byte(""))
	f.Add([]byte("short"))
	f.Add(make([]byte, 31))  // one byte short of a valid HMAC tag
	f.Add(make([]byte, 32))  // exactly tag length, no payload
	f.Add(make([]byte, 128)) // plausible ciphertext length

	key := security.NewRandomKey(32)
	p := security.NewHMACPlugin(key)

	f.Fuzz(func(t *testing.T, data []byte) {
		// Must not panic. May return an error.
		_, _ = p.Open(data)
	})
}

// FuzzAESGCMOpenArbitrary feeds arbitrary bytes to AESGCMPlugin.Open and
// verifies it never panics (invalid inputs must return an error, not panic).
func FuzzAESGCMOpenArbitrary(f *testing.F) {
	f.Add([]byte(""))
	f.Add([]byte("short"))
	f.Add(make([]byte, 12))  // exactly nonce length, no ciphertext+tag
	f.Add(make([]byte, 27))  // nonce + tag - 1
	f.Add(make([]byte, 28))  // nonce(12) + tag(16) = minimum valid length
	f.Add(make([]byte, 256)) // plausible ciphertext

	key := security.NewRandomKey(32)
	p, err := security.NewAESGCMPlugin(key)
	if err != nil {
		f.Fatalf("NewAESGCMPlugin: %v", err)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		// Must not panic. May return an error.
		_, _ = p.Open(data)
	})
}
