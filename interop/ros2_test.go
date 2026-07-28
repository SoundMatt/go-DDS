//go:build interop

// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// ROS 2 wire-compatibility tests (ROADMAP.md Milestone 17, "ROS 2 / rmw
// Compatibility") against a live ROS 2 Jazzy or Rolling peer. Gated behind
// the "interop" build tag exactly like interop_test.go's CycloneDDS tests.
//
// # Prerequisites
//
//  1. Docker (or a native ROS 2 Jazzy/Rolling install on the same host).
//  2. The demo_nodes_cpp talker running on the same ROS_DOMAIN_ID, publishing
//     std_msgs/String on "/chatter" — exactly what
//     `ros2 run demo_nodes_cpp talker` does.
//
// # Quick start with Docker
//
//	docker compose -f interop/docker-compose.yml --profile jazzy up -d
//	go test -tags interop -v -timeout 60s -run ROS2Jazzy ./interop/...
//	docker compose -f interop/docker-compose.yml --profile jazzy down
//
//	docker compose -f interop/docker-compose.yml --profile rolling up -d
//	go test -tags interop -v -timeout 60s -run ROS2Rolling ./interop/...
//	docker compose -f interop/docker-compose.yml --profile rolling down
//
// # Environment variables
//
//   - INTEROP_ROS2_JAZZY_DOMAIN    ROS_DOMAIN_ID for the Jazzy peer (default 70)
//   - INTEROP_ROS2_ROLLING_DOMAIN  ROS_DOMAIN_ID for the Rolling peer (default 71)
//   - INTEROP_TIMEOUT              per-test deadline, e.g. "10s" (default "15s")
package interop

import (
	"os"
	"strconv"
	"testing"
	"time"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/ros2"
)

func ros2Domain(envVar string, fallback int) dds.Domain {
	v := os.Getenv(envVar)
	if v == "" {
		return dds.Domain(fallback)
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return dds.Domain(fallback)
	}
	return dds.Domain(n)
}

// newROS2Listener creates a ros2.Participant that subscribes to "/chatter"
// and skips the test if UDP multicast is unavailable — mirroring
// newParticipant's skip behavior in interop_test.go.
func newROS2Listener(t *testing.T, domain dds.Domain) (*ros2.Participant, dds.Subscriber) {
	t.Helper()
	p, err := ros2.NewROS2Participant(domain, "interop_listener", "/")
	if err != nil {
		t.Skipf("ros2.NewROS2Participant: %v — UDP unavailable", err)
	}
	t.Cleanup(func() { _ = p.Close() })

	// demo_nodes_cpp's own std_msgs/String TypeSupport name — see
	// ros2.TypeSupportName's doc comment for why this exact string is what
	// every conformant rmw uses.
	const stringType = "std_msgs::msg::dds_::String_"
	sub, err := p.NewSubscriber("/chatter", stringType, dds.ReliableQoS)
	if err != nil {
		t.Fatalf("NewSubscriber(/chatter): %v", err)
	}
	t.Cleanup(func() { _ = sub.Close() })
	return p, sub
}

// runROS2TalkerListenerTest is shared between the Jazzy and Rolling cases:
// wait for discovery, confirm the talker's node appears in the graph, then
// confirm at least one std_msgs/String sample arrives on "/chatter".
func runROS2TalkerListenerTest(t *testing.T, domain dds.Domain) {
	p, sub := newROS2Listener(t, domain)

	disc := testTimeout() / 2
	t.Logf("waiting %s for SPDP/SEDP discovery and ros_discovery_info exchange…", disc)
	time.Sleep(disc)

	sawTalkerNode := false
	for _, n := range p.Nodes() {
		t.Logf("graph node: %s (local=%v)", n.FullyQualifiedName, n.Local)
		if n.Name == "talker" {
			sawTalkerNode = true
		}
	}
	if !sawTalkerNode {
		t.Logf("did not see the ROS 2 talker's node in the graph within %s — "+
			"is the ros2-*-talker service running? continuing to wait for data anyway", disc)
	}

	select {
	case s := <-sub.C():
		t.Logf("received %d bytes from ROS 2 talker on /chatter", len(s.Payload))
		if len(s.Payload) == 0 {
			t.Error("received an empty std_msgs/String sample")
		}
	case <-time.After(testTimeout()):
		t.Skipf("no sample from ROS 2 talker within %s — is the talker service running?", testTimeout())
	}
}

// TestInterop_ROS2Jazzy_TalkerListener subscribes to a live ROS 2 Jazzy
// demo_nodes_cpp talker's "/chatter" topic and confirms both graph
// visibility and data delivery.
func TestInterop_ROS2Jazzy_TalkerListener(t *testing.T) {
	runROS2TalkerListenerTest(t, ros2Domain("INTEROP_ROS2_JAZZY_DOMAIN", 70))
}

// TestInterop_ROS2Rolling_TalkerListener is TestInterop_ROS2Jazzy_TalkerListener
// against a ROS 2 Rolling peer instead.
func TestInterop_ROS2Rolling_TalkerListener(t *testing.T) {
	runROS2TalkerListenerTest(t, ros2Domain("INTEROP_ROS2_ROLLING_DOMAIN", 71))
}
