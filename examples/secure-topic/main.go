// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// secure-topic demonstrates end-to-end payload protection using the security
// package. A SecureCodec wraps any dds.Codec with a security.Plugin so that
// published payloads are sealed (encrypted/signed) before hitting the wire and
// opened (decrypted/verified) on receipt.
//
// Three scenarios are shown:
//  1. AES-256-GCM — full confidentiality + integrity + authenticity
//  2. HMAC-SHA-256 — integrity + authenticity, no confidentiality
//  3. Tamper detection — a subscriber with the wrong key receives an error
//
// Run with:
//
//	go run .
package main

import (
	"fmt"
	"log"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/mock"
	"github.com/SoundMatt/go-DDS/security"
)

// SecureCodec wraps a dds.Codec[T] with a security.Plugin.
// Marshal seals the encoded bytes; Unmarshal opens then decodes.
type SecureCodec[T any] struct {
	inner  dds.Codec[T]
	plugin security.Plugin
}

func (s SecureCodec[T]) Marshal(v T) ([]byte, error) {
	plain, err := s.inner.Marshal(v)
	if err != nil {
		return nil, err
	}
	return s.plugin.Seal(plain)
}

func (s SecureCodec[T]) Unmarshal(data []byte) (T, error) {
	plain, err := s.plugin.Open(data)
	if err != nil {
		var zero T
		return zero, fmt.Errorf("secure-topic: open failed: %w", err)
	}
	return s.inner.Unmarshal(plain)
}

type TelemetryEvent struct {
	NodeID  string
	Payload string
}

func main() {
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		log.Fatal(err)
	}
	defer p.Close()

	fmt.Println("── Scenario 1: AES-256-GCM (confidentiality + integrity) ──")
	demoAESGCM(p)

	fmt.Println("\n── Scenario 2: HMAC-SHA-256 (integrity only) ──")
	demoHMAC(p)

	fmt.Println("\n── Scenario 3: tamper detection (wrong key) ──")
	demoTamper(p)
}

func demoAESGCM(p dds.Participant) {
	key := security.NewRandomKey(32)
	plugin, err := security.NewAESGCMPlugin(key)
	if err != nil {
		log.Fatal(err)
	}
	codec := SecureCodec[TelemetryEvent]{inner: dds.JSONCodec[TelemetryEvent]{}, plugin: plugin}

	pub, err := p.NewPublisher("secure/aesgcm", dds.DefaultQoS)
	if err != nil {
		log.Fatal(err)
	}
	defer pub.Close()
	tpub := dds.NewTypedPublisher(pub, codec)

	sub, err := p.NewSubscriber("secure/aesgcm", dds.DefaultQoS)
	if err != nil {
		log.Fatal(err)
	}
	defer sub.Close()
	tsub := dds.NewTypedSubscriber(sub, codec)
	defer tsub.Close()

	event := TelemetryEvent{NodeID: "node-42", Payload: "classified telemetry"}
	if err := tpub.Write(event); err != nil {
		log.Fatal(err)
	}

	s := <-tsub.C()
	fmt.Printf("  received: node=%s payload=%q\n", s.Value.NodeID, s.Value.Payload)
	fmt.Println("  wire bytes are AES-GCM ciphertext — unreadable without the key")
}

func demoHMAC(p dds.Participant) {
	key := security.NewRandomKey(32)
	plugin := security.NewHMACPlugin(key)
	codec := SecureCodec[TelemetryEvent]{inner: dds.JSONCodec[TelemetryEvent]{}, plugin: plugin}

	pub, err := p.NewPublisher("secure/hmac", dds.DefaultQoS)
	if err != nil {
		log.Fatal(err)
	}
	defer pub.Close()
	tpub := dds.NewTypedPublisher(pub, codec)

	sub, err := p.NewSubscriber("secure/hmac", dds.DefaultQoS)
	if err != nil {
		log.Fatal(err)
	}
	defer sub.Close()
	tsub := dds.NewTypedSubscriber(sub, codec)
	defer tsub.Close()

	event := TelemetryEvent{NodeID: "node-7", Payload: "status=ok"}
	if err := tpub.Write(event); err != nil {
		log.Fatal(err)
	}

	s := <-tsub.C()
	fmt.Printf("  received: node=%s payload=%q\n", s.Value.NodeID, s.Value.Payload)
	fmt.Println("  wire bytes are plaintext JSON + 32-byte HMAC tag appended")
}

func demoTamper(p dds.Participant) {
	rightKey := security.NewRandomKey(32)
	wrongKey := security.NewRandomKey(32)

	pubPlugin, err := security.NewAESGCMPlugin(rightKey)
	if err != nil {
		log.Fatal(err)
	}
	subPlugin, err := security.NewAESGCMPlugin(wrongKey)
	if err != nil {
		log.Fatal(err)
	}

	pubCodec := SecureCodec[TelemetryEvent]{inner: dds.JSONCodec[TelemetryEvent]{}, plugin: pubPlugin}

	pub, err := p.NewPublisher("secure/tamper", dds.DefaultQoS)
	if err != nil {
		log.Fatal(err)
	}
	defer pub.Close()
	tpub := dds.NewTypedPublisher(pub, pubCodec)

	// Subscribe with the wrong key — decode errors are silently dropped by
	// TypedSubscriber, so we use a raw subscriber to observe the failure directly.
	rawSub, err := p.NewSubscriber("secure/tamper", dds.DefaultQoS)
	if err != nil {
		log.Fatal(err)
	}
	defer rawSub.Close()

	event := TelemetryEvent{NodeID: "node-99", Payload: "secret"}
	if err := tpub.Write(event); err != nil {
		log.Fatal(err)
	}

	raw := <-rawSub.C()
	_, openErr := subPlugin.Open(raw.Payload)
	if openErr != nil {
		fmt.Printf("  tamper detected: %v\n", openErr)
		fmt.Println("  payload rejected — wrong key cannot decrypt or verify")
	}
}
