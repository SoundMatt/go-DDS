// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Command auto-transport demonstrates the auto transport selector.
//
// auto.NewParticipant picks the best transport automatically:
//   - Same host: shared-memory (zero network overhead)
//   - Cross-host or shmem unavailable: RTPS/UDP fallback
//
// No transport-specific import is needed by the caller.
//
// Run:
//
//	go run ./examples/auto-transport
package main

import (
	"log"
	"time"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/auto"
)

func main() {
	// TransportAuto: shmem if available, RTPS otherwise.
	p, err := auto.NewParticipant(dds.Domain(0))
	if err != nil {
		log.Fatalf("auto.NewParticipant: %v", err)
	}
	defer func() { _ = p.Close() }()

	log.Printf("participant on domain %d", p.Domain())

	sub, err := p.NewSubscriber("vehicle/speed", dds.DefaultQoS)
	if err != nil {
		log.Fatalf("NewSubscriber: %v", err)
	}
	defer func() { _ = sub.Close() }()

	pub, err := p.NewPublisher("vehicle/speed", dds.DefaultQoS)
	if err != nil {
		log.Fatalf("NewPublisher: %v", err)
	}
	defer func() { _ = pub.Close() }()

	if writeErr := pub.Write([]byte(`{"speed_kmh":60}`)); writeErr != nil {
		log.Fatalf("Write: %v", writeErr)
	}

	select {
	case s := <-sub.C():
		log.Printf("received: %s", s.Payload)
	case <-time.After(2 * time.Second):
		log.Fatal("timeout waiting for sample")
	}

	// Force RTPS regardless of transport availability.
	p2, err := auto.NewParticipant(dds.Domain(1), auto.WithTransport(auto.TransportRTPS))
	if err != nil {
		log.Fatalf("auto.NewParticipant (rtps): %v", err)
	}
	defer func() { _ = p2.Close() }()
	log.Printf("RTPS participant on domain %d", p2.Domain())
}
