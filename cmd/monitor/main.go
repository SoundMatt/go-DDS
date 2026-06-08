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
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/monitor"
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

	p, err := rtps.New(domain)
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
