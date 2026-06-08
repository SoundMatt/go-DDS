// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Command sub subscribes to DDS topic "sensors/temperature" and logs samples.
// Part of the Docker Quickstart (Milestone 13).
package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/rtps"
)

func main() {
	p, err := rtps.New(dds.Domain(0))
	if err != nil {
		log.Fatalf("rtps.New: %v", err)
	}
	defer func() { _ = p.Close() }()

	sub, err := p.NewSubscriber("sensors/temperature", dds.DefaultQoS)
	if err != nil {
		log.Fatalf("NewSubscriber: %v", err)
	}
	defer func() { _ = sub.Close() }()

	log.Println("subscribing to sensors/temperature…")

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	for {
		select {
		case s, ok := <-sub.C():
			if !ok {
				return
			}
			log.Printf("received [seq=%d writer=%x]: %s", s.SequenceNumber, s.WriterGUID[:4], s.Payload)
		case <-sig:
			return
		}
	}
}
