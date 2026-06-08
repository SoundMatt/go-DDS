// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Command pub publishes sensor readings on DDS topic "sensors/temperature".
// Part of the Docker Quickstart (Milestone 13).
package main

import (
	"fmt"
	"log"
	"math/rand"
	"time"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/rtps"
)

func main() {
	p, err := rtps.New(dds.Domain(0))
	if err != nil {
		log.Fatalf("rtps.New: %v", err)
	}
	defer func() { _ = p.Close() }()

	pub, err := p.NewPublisher("sensors/temperature", dds.DefaultQoS)
	if err != nil {
		log.Fatalf("NewPublisher: %v", err)
	}
	defer func() { _ = pub.Close() }()

	log.Println("publishing on sensors/temperature every second…")
	for i := 1; ; i++ {
		temp := 20.0 + rand.Float64()*10
		msg := fmt.Sprintf(`{"seq":%d,"temp":%.2f}`, i, temp)
		if err := pub.Write([]byte(msg)); err != nil {
			log.Printf("Write: %v", err)
			return
		}
		log.Printf("published: %s", msg)
		time.Sleep(time.Second)
	}
}
