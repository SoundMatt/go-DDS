// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Command server is the native (non-Wasm) half of the wasm-subscriber demo
// (ROADMAP.md Milestone 16, "WebAssembly Target"): an ordinary go-DDS
// participant that binds the RTPS-over-WebSocket transport's listener
// (rtps.WithWSAddr) and publishes an incrementing sample once a second, so
// ../index.html has a real peer to discover and receive data from. This is
// exactly the "embedded devices" side of Milestone 16's success criterion
// ("A browser tab and a cloud function can join a DDS domain alongside
// embedded devices without a protocol bridge"): any other WS-capable
// go-DDS participant works equally well; this one exists purely to make
// the demo self-contained.
//
// Run (from the examples/ module directory):
//
//	go run ./wasm-subscriber/server
//
// then build and serve wasm-subscriber/ per its README.md.
package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"time"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/rtps"
)

func main() {
	addr := flag.String("addr", ":7800", "RTPS-over-WebSocket listen address (host:port)")
	domain := flag.Int("domain", 0, "DDS domain ID")
	topic := flag.String("topic", "vehicle/speed", "topic to publish samples on")
	flag.Parse()

	p, err := rtps.New(dds.Domain(*domain), rtps.WithWSAddr(*addr))
	if err != nil {
		log.Fatalf("rtps.New: %v", err)
	}
	defer func() { _ = p.Close() }()

	pub, err := p.NewPublisher(*topic, dds.DefaultQoS)
	if err != nil {
		log.Fatalf("NewPublisher: %v", err)
	}
	defer func() { _ = pub.Close() }()

	log.Printf("go-DDS RTPS-over-WebSocket server listening on %s, domain %d, topic %q", *addr, *domain, *topic)
	log.Printf("build and open ../index.html?ws=127.0.0.1%s&domain=%d&topic=%s to watch samples arrive in a browser tab (see README.md)",
		hostPortSuffix(*addr), *domain, *topic)

	var seq int
	for range time.Tick(time.Second) {
		seq++
		payload := fmt.Sprintf(`{"seq":%d,"speed_kmh":%d}`, seq, 40+seq%20)
		if err := pub.Write([]byte(payload)); err != nil {
			log.Printf("Write: %v", err)
		}
	}
}

// hostPortSuffix returns ":<port>" for a listen address such as ":7800" or
// "0.0.0.0:7800", for building the printed index.html?ws=... hint above —
// a listen address's own host part (often empty, or a wildcard) is never
// the right host for a client to dial, only its port is reusable as-is.
func hostPortSuffix(addr string) string {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	return ":" + port
}
