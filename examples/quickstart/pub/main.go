// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Command pub publishes sensor readings on DDS topic "sensors/temperature".
// Part of the Docker Quickstart (Milestone 13).
//
// Environment variables:
//
//	DDS_DOMAIN   DDS domain ID (default: 0)
//	DDS_PEERS    Comma-separated static peer addresses for bridge networking
//	             (e.g. "monitor:7400,sub:7400"). When set, multicast is
//	             disabled so this works on Docker bridge networks.
package main

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"strings"
	"time"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/rtps"
)

func main() {
	var opts []rtps.Option
	if peers := os.Getenv("DDS_PEERS"); peers != "" {
		addrs := strings.Split(peers, ",")
		opts = append(opts, rtps.WithNoMulticast(), rtps.WithStaticPeers(addrs...))
		log.Printf("unicast mode, peers: %v", addrs)
	}

	p, err := rtps.New(dds.Domain(0), opts...)
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
