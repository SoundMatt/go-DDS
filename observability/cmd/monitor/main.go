// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Command monitor starts a real-time web dashboard for a DDS domain.
//
// Usage:
//
//	monitor [flags]
//
// Environment variables:
//
//	DDS_DOMAIN     DDS domain ID (default: 0)
//	MONITOR_ADDR   HTTP listen address (default: :8080)
//	DDS_PEERS      Comma-separated static peer addresses for bridge networking
//	               (e.g. "pub:7400,sub:7400"). When set, multicast is disabled.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/observability/monitor"
	"github.com/SoundMatt/go-DDS/rtps"
)

func main() {
	domain := dds.Domain(0)
	if s := os.Getenv("DDS_DOMAIN"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil {
			log.Fatalf("DDS_DOMAIN: %v", err)
		}
		domain = dds.Domain(n)
	}

	addr := ":8080"
	if s := os.Getenv("MONITOR_ADDR"); s != "" {
		addr = s
	}

	var opts []rtps.Option
	if peers := os.Getenv("DDS_PEERS"); peers != "" {
		addrs := strings.Split(peers, ",")
		opts = append(opts, rtps.WithNoMulticast(), rtps.WithStaticPeers(addrs...))
		log.Printf("unicast mode, peers: %v", addrs)
	}

	p, err := rtps.New(domain, opts...)
	if err != nil {
		log.Fatalf("rtps.New(domain=%d): %v", domain, err)
	}
	defer func() { _ = p.Close() }()

	mon, err := monitor.New(p, monitor.Options{Addr: addr})
	if err != nil {
		log.Fatalf("monitor.New: %v", err)
	}
	defer func() { _ = mon.Close() }()

	log.Printf("go-DDS monitor listening on %s (domain %d)", addr, domain)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	log.Println("shutting down")
}
