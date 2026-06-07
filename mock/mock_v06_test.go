// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// v0.6 tests for mock package: DiscoveryMetrics, TopicMetrics, Health.

package mock_test

import (
	"fmt"
	"testing"
	"time"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/mock"
)

// uniqueTopic returns a topic name that won't collide with topics in parallel tests.
func uniqueTopic(name string) string {
	return fmt.Sprintf("v06-mock/%s/%d", name, time.Now().UnixNano())
}

// ── DiscoveryMetrics ──────────────────────────────────────────────────────────

func TestMockDiscoveryMetrics_ReturnsZero(t *testing.T) {
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		t.Fatalf("mock.New: %v", err)
	}
	defer p.Close()

	dp, ok := p.(dds.DiscoveryMetricsProvider)
	if !ok {
		t.Fatal("mock participant must implement DiscoveryMetricsProvider")
	}
	dm := dp.DiscoveryMetrics()
	// Mock has no real discovery; all fields must be zero.
	if dm.AnnouncesSent != 0 || dm.AnnouncesReceived != 0 ||
		dm.PeersKnown != 0 || dm.PeerEvictions != 0 || dm.EndpointMatches != 0 {
		t.Errorf("expected all-zero DiscoveryMetrics for mock, got %+v", dm)
	}
}

// ── TopicMetrics ──────────────────────────────────────────────────────────────

func TestMockTopicMetrics_WriteCountIncremented(t *testing.T) {
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		t.Fatalf("mock.New: %v", err)
	}
	defer p.Close()

	tp, ok := p.(dds.TopicMetricsProvider)
	if !ok {
		t.Fatal("mock participant must implement TopicMetricsProvider")
	}

	topic := uniqueTopic("writes")
	pub, err := p.NewPublisher(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	const n = 4
	for i := 0; i < n; i++ {
		if err := pub.Write([]byte("payload")); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}

	var found *dds.TopicMetrics
	for _, m := range tp.TopicMetrics() {
		if m.Topic == topic {
			cp := m
			found = &cp
			break
		}
	}
	if found == nil {
		t.Fatalf("topic %q not found in TopicMetrics", topic)
	}
	if found.WriteCount != n {
		t.Errorf("WriteCount: got %d, want %d", found.WriteCount, n)
	}
	if found.BytesWritten == 0 {
		t.Error("BytesWritten must be > 0")
	}
}

func TestMockTopicMetrics_DeliverCountIncremented(t *testing.T) {
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		t.Fatalf("mock.New: %v", err)
	}
	defer p.Close()

	tp, ok := p.(dds.TopicMetricsProvider)
	if !ok {
		t.Fatal("mock participant must implement TopicMetricsProvider")
	}

	topic := uniqueTopic("delivers")
	sub, err := p.NewSubscriber(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()

	pub, err := p.NewPublisher(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	payload := []byte("deliver-payload")
	if err := pub.Write(payload); err != nil {
		t.Fatalf("Write: %v", err)
	}
	select {
	case <-sub.C():
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for sample")
	}

	var found *dds.TopicMetrics
	for _, m := range tp.TopicMetrics() {
		if m.Topic == topic {
			cp := m
			found = &cp
			break
		}
	}
	if found == nil {
		t.Fatalf("topic %q not found in TopicMetrics", topic)
	}
	if found.DeliverCount == 0 {
		t.Error("DeliverCount must be > 0 after delivery")
	}
	if found.BytesDelivered == 0 {
		t.Error("BytesDelivered must be > 0")
	}
}

func TestMockTopicMetrics_DropCountIncremented(t *testing.T) {
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		t.Fatalf("mock.New: %v", err)
	}
	defer p.Close()

	tp, ok := p.(dds.TopicMetricsProvider)
	if !ok {
		t.Fatal("mock participant must implement TopicMetricsProvider")
	}

	topic := uniqueTopic("drops")
	// Depth=1 so the second write causes a drop under DropNewest.
	sub, err := p.NewSubscriber(topic, dds.DefaultQoS,
		dds.WithChannelDepth(1),
		dds.WithBackPressure(dds.DropNewest),
	)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()

	pub, err := p.NewPublisher(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	// Write twice. The second write should be dropped (channel depth=1 and first
	// write fills it).
	_ = pub.Write([]byte("first"))
	_ = pub.Write([]byte("second")) // should drop

	// Give the broker a moment (it's synchronous in mock, so no wait needed,
	// but drain the channel to avoid leaks).
	select {
	case <-sub.C():
	default:
	}

	var found *dds.TopicMetrics
	for _, m := range tp.TopicMetrics() {
		if m.Topic == topic {
			cp := m
			found = &cp
			break
		}
	}
	if found == nil {
		t.Fatalf("topic %q not found in TopicMetrics", topic)
	}
	if found.DropCount == 0 {
		t.Error("DropCount must be > 0 after a drop")
	}
}

func TestMockTopicMetrics_MultipleTopic(t *testing.T) {
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		t.Fatalf("mock.New: %v", err)
	}
	defer p.Close()

	tp, ok := p.(dds.TopicMetricsProvider)
	if !ok {
		t.Fatal("mock participant must implement TopicMetricsProvider")
	}

	topicA := uniqueTopic("multi-a")
	topicB := uniqueTopic("multi-b")

	pubA, _ := p.NewPublisher(topicA, dds.DefaultQoS)
	pubB, _ := p.NewPublisher(topicB, dds.DefaultQoS)
	defer pubA.Close()
	defer pubB.Close()

	_ = pubA.Write([]byte("a"))
	_ = pubB.Write([]byte("b"))
	_ = pubB.Write([]byte("b2"))

	counts := map[string]uint64{}
	for _, m := range tp.TopicMetrics() {
		counts[m.Topic] = m.WriteCount
	}
	if counts[topicA] != 1 {
		t.Errorf("topicA WriteCount: got %d, want 1", counts[topicA])
	}
	if counts[topicB] != 2 {
		t.Errorf("topicB WriteCount: got %d, want 2", counts[topicB])
	}
}

// ── Health ────────────────────────────────────────────────────────────────────

func TestMockHealth_OK_WhenOpen(t *testing.T) {
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		t.Fatalf("mock.New: %v", err)
	}
	defer p.Close()

	hp, ok := p.(dds.HealthProvider)
	if !ok {
		t.Fatal("mock participant must implement HealthProvider")
	}
	h := hp.Health()
	if h.Status != dds.HealthOK {
		t.Errorf("expected HealthOK, got %v", h.Status)
	}
}

func TestMockHealth_Down_AfterClose(t *testing.T) {
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		t.Fatalf("mock.New: %v", err)
	}
	if err := p.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	hp, ok := p.(dds.HealthProvider)
	if !ok {
		t.Fatal("mock participant must implement HealthProvider")
	}
	h := hp.Health()
	if h.Status != dds.HealthDown {
		t.Errorf("expected HealthDown after close, got %v", h.Status)
	}
}

func TestMockHealth_StatusString(t *testing.T) {
	if dds.HealthOK.String() != "ok" {
		t.Errorf("HealthOK.String() = %q", dds.HealthOK.String())
	}
	if dds.HealthDegraded.String() != "degraded" {
		t.Errorf("HealthDegraded.String() = %q", dds.HealthDegraded.String())
	}
	if dds.HealthDown.String() != "down" {
		t.Errorf("HealthDown.String() = %q", dds.HealthDown.String())
	}
}
