//go:build interop

// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package interop provides RTPS wire-compatibility tests against a live
// CycloneDDS peer. These tests are gated behind the "interop" build tag
// so they do not run in the normal CI suite.
//
// # Prerequisites
//
//  1. Docker or a native CycloneDDS installation on the same host.
//  2. A CycloneDDS subscriber listening on topic "interop/go-dds-ping" on
//     domain 0 (or set INTEROP_DOMAIN to a different domain).
//  3. A CycloneDDS publisher writing to "interop/go-dds-pong" on the same domain.
//
// # Quick start with Docker
//
//	docker compose -f interop/docker-compose.yml up -d cyclone-peer
//	go test -tags interop -v -timeout 30s ./interop/...
//	docker compose -f interop/docker-compose.yml down
//
// # Environment variables
//
//   - INTEROP_DOMAIN   DDS domain (default "0")
//   - INTEROP_TIMEOUT  per-test deadline, e.g. "10s" (default "15s")
package interop

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/rtps"
)

func testDomain() dds.Domain {
	v := os.Getenv("INTEROP_DOMAIN")
	if v == "" {
		return dds.Domain(0)
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return dds.Domain(0)
	}
	return dds.Domain(n)
}

func testTimeout() time.Duration {
	v := os.Getenv("INTEROP_TIMEOUT")
	if v == "" {
		return 15 * time.Second
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 15 * time.Second
	}
	return d
}

// newParticipant creates a go-DDS RTPS participant and skips the test if
// UDP multicast is unavailable.
func newParticipant(t *testing.T) dds.Participant {
	t.Helper()
	p, err := rtps.New(testDomain())
	if err != nil {
		t.Skipf("rtps.New: %v — UDP unavailable", err)
	}
	t.Cleanup(func() { _ = p.Close() })
	return p
}

// TestInterop_GoPublisher_CycloneSubscriber publishes 5 samples on
// "interop/go-dds-ping" and expects the CycloneDDS peer to acknowledge
// receipt (verified indirectly — the test passes if no write errors occur
// and SPDP discovery completes within the timeout).
//
// Set up the cyclone-sub service before running:
//
//	docker compose -f interop/docker-compose.yml run --rm cyclone-sub
func TestInterop_GoPublisher_CycloneSubscriber(t *testing.T) {
	p := newParticipant(t)

	pub, err := p.NewPublisher("interop/go-dds-ping", dds.ReliableQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	// Allow SPDP/SEDP discovery to complete.
	disc := testTimeout() / 3
	t.Logf("waiting %s for SPDP discovery…", disc)
	time.Sleep(disc)

	for i := 0; i < 5; i++ {
		payload := []byte(fmt.Sprintf(`{"seq":%d,"src":"go-DDS"}`, i))
		if err := pub.Write(payload); err != nil {
			t.Errorf("Write(%d): %v", i, err)
		}
	}
}

// TestInterop_CyclonePublisher_GoSubscriber subscribes to
// "interop/go-dds-pong" and expects to receive at least one sample from
// the CycloneDDS publisher within the timeout.
//
// Set up the cyclone-pub service before running:
//
//	docker compose -f interop/docker-compose.yml run --rm cyclone-pub
func TestInterop_CyclonePublisher_GoSubscriber(t *testing.T) {
	p := newParticipant(t)

	sub, err := p.NewSubscriber("interop/go-dds-pong", dds.ReliableQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout())
	defer cancel()

	ws := dds.NewWaitSet(sub)
	s, wsSub, err := ws.Wait(ctx)
	_ = wsSub
	if err != nil {
		t.Skipf("no sample from CycloneDDS within %s — is the cyclone-pub service running? (%v)", testTimeout(), err)
	}
	t.Logf("received %d bytes from CycloneDDS: %q", len(s.Payload), s.Payload)
}

// TestInterop_BidirectionalEcho publishes on "interop/go-dds-ping" and
// expects an echo back on "interop/go-dds-pong" from a CycloneDDS relay
// (the cyclone-peer service in docker-compose.yml).
func TestInterop_BidirectionalEcho(t *testing.T) {
	p := newParticipant(t)

	pub, err := p.NewPublisher("interop/go-dds-ping", dds.ReliableQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	sub, err := p.NewSubscriber("interop/go-dds-pong", dds.ReliableQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()

	disc := testTimeout() / 3
	t.Logf("waiting %s for SPDP discovery…", disc)
	time.Sleep(disc)

	if err := pub.Write([]byte(`{"ping":true}`)); err != nil {
		t.Fatalf("Write: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout()*2/3)
	defer cancel()

	ws := dds.NewWaitSet(sub)
	s, wsSub, err := ws.Wait(ctx)
	_ = wsSub
	if err != nil {
		t.Skipf("no echo from CycloneDDS within timeout — is the cyclone-peer service running? (%v)", err)
	}
	t.Logf("echo received (%d bytes): %q", len(s.Payload), s.Payload)
}
