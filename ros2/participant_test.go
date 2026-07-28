// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package ros2_test

import (
	"bytes"
	"errors"
	"fmt"
	"runtime"
	"testing"
	"time"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/ros2"
)

func TestNewROS2Participant_InvalidNodeName(t *testing.T) {
	_, err := ros2.NewROS2Participant(dds.Domain(200), "1bad-name", "/")
	if !errors.Is(err, ros2.ErrInvalidName) {
		t.Errorf("err = %v, want ErrInvalidName", err)
	}
}

func TestNewROS2Participant_InvalidNamespace(t *testing.T) {
	_, err := ros2.NewROS2Participant(dds.Domain(200), "talker", "no-leading-slash")
	if !errors.Is(err, ros2.ErrInvalidName) {
		t.Errorf("err = %v, want ErrInvalidName", err)
	}
}

// TestSingleNode_SelfRegistersAndPublishesLocally exercises the
// no-network-required path: a single ros2.Participant sees itself in
// Nodes(), its own pub/sub in Topics() under the demangled ROS 2 name, and
// same-process delivery works exactly like a plain rtps.Participant's.
func TestSingleNode_SelfRegistersAndPublishesLocally(t *testing.T) {
	p, err := ros2.NewROS2Participant(dds.Domain(201), "talker", "/robot1")
	if err != nil {
		t.Skipf("NewROS2Participant: %v", err)
	}
	defer p.Close()

	if got, want := p.FullyQualifiedNodeName(), "/robot1/talker"; got != want {
		t.Errorf("FullyQualifiedNodeName() = %q, want %q", got, want)
	}

	nodes := p.Nodes()
	if len(nodes) != 1 || !nodes[0].Local || nodes[0].FullyQualifiedName != "/robot1/talker" {
		t.Fatalf("Nodes() = %+v, want exactly this node, Local=true", nodes)
	}

	const typeName = "std_msgs::msg::dds_::String_"
	sub, err := p.NewSubscriber("chatter", typeName, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()

	pub, err := p.NewPublisher("chatter", typeName, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	want := []byte("hello")
	if err := pub.Write(want); err != nil {
		t.Fatalf("Write: %v", err)
	}
	select {
	case s := <-sub.C():
		if !bytes.Equal(s.Payload, want) {
			t.Errorf("payload: got %q, want %q", s.Payload, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: same-node sample not delivered")
	}

	topics := p.Topics()
	found := false
	for _, tp := range topics {
		if tp.Name != "/robot1/chatter" {
			continue
		}
		found = true
		if tp.PublisherCount != 1 || tp.SubscriberCount != 1 {
			t.Errorf("topic %q: PublisherCount=%d SubscriberCount=%d, want 1/1", tp.Name, tp.PublisherCount, tp.SubscriberCount)
		}
		if len(tp.Types) != 1 || tp.Types[0] != typeName {
			t.Errorf("topic %q: Types=%v, want [%q]", tp.Name, tp.Types, typeName)
		}
	}
	if !found {
		t.Errorf("Topics() = %+v, missing /robot1/chatter", topics)
	}

	// The hidden ros_discovery_info topic must never surface in Topics().
	for _, tp := range topics {
		if tp.Name == ros2.DiscoveryTopicName || tp.Name == "/"+ros2.DiscoveryTopicName {
			t.Errorf("Topics() leaked the discovery topic: %+v", tp)
		}
	}
}

func TestNewPublisher_AbsoluteTopicIgnoresNamespace(t *testing.T) {
	p, err := ros2.NewROS2Participant(dds.Domain(202), "talker", "/robot1")
	if err != nil {
		t.Skipf("NewROS2Participant: %v", err)
	}
	defer p.Close()

	pub, err := p.NewPublisher("/global/topic", "std_msgs::msg::dds_::String_", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	for _, tp := range p.Topics() {
		if tp.Name == "/robot1/global/topic" {
			t.Errorf("absolute topic name was namespaced: %+v", tp)
		}
	}
	found := false
	for _, tp := range p.Topics() {
		if tp.Name == "/global/topic" {
			found = true
		}
	}
	if !found {
		t.Errorf("Topics() = %+v, missing /global/topic", p.Topics())
	}
}

// TestTwoNodes_SeeEachOtherInGraph proves the actual point of this
// sub-phase: two independent ros2.Participants (i.e. two separate
// underlying rtps.Participants, exactly like two separate ROS 2
// processes) discover each other over real RTPS SPDP/SEDP, exchange
// ros_discovery_info graph samples, and end up seeing each other's node
// in Nodes() and each other's topic in Topics() — with no bridge process
// and no shared in-memory state. Mirrors
// rtps_test.TestRTPS_TwoParticipants_SameHost's network/platform gating.
func TestTwoNodes_SeeEachOtherInGraph(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping two-node graph test in -short mode (requires network)")
	}
	if runtime.GOOS != "linux" {
		t.Skipf("UDP multicast loopback unreliable on %s; skipping cross-participant test", runtime.GOOS)
	}

	const domain = dds.Domain(210)
	const typeName = "std_msgs::msg::dds_::String_"

	talker, err := ros2.NewROS2Participant(domain, "talker", "/robot1")
	if err != nil {
		t.Skipf("NewROS2Participant(talker): %v", err)
	}
	defer talker.Close()

	listener, err := ros2.NewROS2Participant(domain, "listener", "/robot2")
	if err != nil {
		t.Skipf("NewROS2Participant(listener): %v", err)
	}
	defer listener.Close()

	pub, err := talker.NewPublisher("/chatter", typeName, dds.ReliableQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	sub, err := listener.NewSubscriber("/chatter", typeName, dds.ReliableQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()

	// Allow SPDP/SEDP discovery and ros_discovery_info exchange to settle
	// (within the 2 s SPDP announce period), matching
	// TestRTPS_TwoParticipants_SameHost's own wait.
	time.Sleep(2200 * time.Millisecond)

	want := []byte("hello from talker")
	if err := pub.Write(want); err != nil {
		t.Fatalf("Write: %v", err)
	}
	select {
	case s := <-sub.C():
		if !bytes.Equal(s.Payload, want) {
			t.Errorf("payload: got %q, want %q", s.Payload, want)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout: cross-node sample not received")
	}

	// ros_discovery_info is exchanged over the same best-effort SPDP/SEDP
	// matching path as any other topic, on a TRANSIENT_LOCAL history-1
	// writer — matching is asynchronous and its completion isn't ordered
	// against the /chatter delivery above, so poll rather than assert
	// immediately (a fixed sleep here was flaky under CI load: matching
	// can still be in flight even after a user-topic sample has already
	// been delivered).
	waitFor(t, 3*time.Second, func() bool { return hasNode(listener.Nodes(), "/robot1/talker") },
		func() string { return fmt.Sprintf("listener.Nodes() = %+v, missing /robot1/talker", listener.Nodes()) })
	waitFor(t, 3*time.Second, func() bool { return hasNode(talker.Nodes(), "/robot2/listener") },
		func() string { return fmt.Sprintf("talker.Nodes() = %+v, missing /robot2/listener", talker.Nodes()) })
	waitFor(t, 3*time.Second, func() bool { return hasTopic(listener.Topics(), "/chatter") },
		func() string { return fmt.Sprintf("listener.Topics() = %+v, missing /chatter", listener.Topics()) })
	waitFor(t, 3*time.Second, func() bool { return hasTopic(talker.Topics(), "/chatter") },
		func() string { return fmt.Sprintf("talker.Topics() = %+v, missing /chatter", talker.Topics()) })
}

// waitFor polls cond every 20ms until it returns true or timeout elapses,
// failing the test with msg() only if cond never became true.
func waitFor(t *testing.T, timeout time.Duration, cond func() bool, msg func() string) {
	t.Helper()
	deadline := time.After(timeout)
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		if cond() {
			return
		}
		select {
		case <-ticker.C:
		case <-deadline:
			t.Error(msg())
			return
		}
	}
}

func hasNode(nodes []ros2.NodeInfo, fqn string) bool {
	for _, n := range nodes {
		if n.FullyQualifiedName == fqn {
			return true
		}
	}
	return false
}

func hasTopic(topics []ros2.TopicInfo, name string) bool {
	for _, tp := range topics {
		if tp.Name == name {
			return true
		}
	}
	return false
}
