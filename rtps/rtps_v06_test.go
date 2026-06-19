// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// v0.6 tests: WithHeartbeatPeriod, WithConfig, per-topic metrics,
// DiscoveryMetrics, TopicMetrics, Health.

package rtps_test

//fusa:test REQ-REL-005
//fusa:test REQ-CONF-001
//fusa:test REQ-CONF-002
//fusa:test REQ-DISC-012
//fusa:test REQ-SEOOC-008

import (
	"strings"
	"testing"
	"time"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/config"
	"github.com/SoundMatt/go-DDS/rtps"
)

// ── WithHeartbeatPeriod ───────────────────────────────────────────────────────

func TestWithHeartbeatPeriod_ReliableWriter(t *testing.T) {
	// Verify that a reliable writer created with a custom heartbeat period does
	// not panic and still delivers samples correctly.
	p, err := rtps.New(testDomain,
		rtps.WithHeartbeatPeriod(50*time.Millisecond),
		rtps.WithNoMulticast(),
	)
	if err != nil {
		t.Skipf("rtps.New: %v — UDP unavailable", err)
	}
	defer p.Close()

	sub, err := p.NewSubscriber("v06/hbperiod", dds.ReliableQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()

	pub, err := p.NewPublisher("v06/hbperiod", dds.ReliableQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	if err := pub.Write([]byte("hb-period-test")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	select {
	case s := <-sub.C():
		if string(s.Payload) != "hb-period-test" {
			t.Errorf("payload: got %q", s.Payload)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for sample with custom heartbeat period")
	}
}

// ── WithConfig ────────────────────────────────────────────────────────────────

func TestWithConfig_AppliesOptions(t *testing.T) {
	cfg, err := config.ParseConfig(strings.NewReader(`{
		"domain": 0,
		"heartbeat_period": "150ms",
		"spdp_interval": "5s",
		"spdp_jitter": "0s",
		"no_multicast": true,
		"log_level": "info"
	}`))
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}

	p, newErr := rtps.New(testDomain, rtps.WithConfig(cfg))
	if newErr != nil {
		t.Skipf("rtps.New: %v — UDP unavailable", newErr)
	}
	defer p.Close()

	// With no_multicast=true the participant should still be usable.
	pub, pubErr := p.NewPublisher("v06/config", dds.DefaultQoS)
	if pubErr != nil {
		t.Fatalf("NewPublisher: %v", pubErr)
	}
	defer pub.Close()

	if err := pub.Write([]byte("configured")); err != nil {
		t.Fatalf("Write: %v", err)
	}
}

func TestWithConfig_PeerLocators(t *testing.T) {
	cfg, err := config.ParseConfig(strings.NewReader(`{
		"domain": 0,
		"no_multicast": true,
		"peer_locators": ["127.0.0.1:7410"]
	}`))
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	p, newErr := rtps.New(testDomain, rtps.WithConfig(cfg))
	if newErr != nil {
		t.Skipf("rtps.New: %v — UDP unavailable", newErr)
	}
	defer p.Close()
	// Just verify it was created without error.
}

// ── TopicMetrics ──────────────────────────────────────────────────────────────

func TestTopicMetrics_WritesTracked(t *testing.T) {
	p, err := rtps.New(testDomain, rtps.WithNoMulticast())
	if err != nil {
		t.Skipf("rtps.New: %v", err)
	}
	defer p.Close()

	tp, ok := p.(dds.TopicMetricsProvider)
	if !ok {
		t.Fatal("rtps participant must implement TopicMetricsProvider")
	}

	pub, err := p.NewPublisher("v06/topicmetrics/writes", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	const n = 3
	for i := 0; i < n; i++ {
		if err := pub.Write([]byte("x")); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}

	metrics := tp.TopicMetrics()
	var found *dds.TopicMetrics
	for i := range metrics {
		if metrics[i].Topic == "v06/topicmetrics/writes" {
			found = &metrics[i]
			break
		}
	}
	if found == nil {
		t.Fatal("expected topic 'v06/topicmetrics/writes' in TopicMetrics")
	}
	if found.WriteCount != n {
		t.Errorf("WriteCount: got %d, want %d", found.WriteCount, n)
	}
	if found.BytesWritten == 0 {
		t.Error("BytesWritten must be > 0")
	}
}

func TestTopicMetrics_DeliversTracked(t *testing.T) {
	p, err := rtps.New(testDomain, rtps.WithNoMulticast())
	if err != nil {
		t.Skipf("rtps.New: %v", err)
	}
	defer p.Close()

	tp, ok := p.(dds.TopicMetricsProvider)
	if !ok {
		t.Fatal("rtps participant must implement TopicMetricsProvider")
	}

	sub, err := p.NewSubscriber("v06/topicmetrics/delivers", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()

	pub, err := p.NewPublisher("v06/topicmetrics/delivers", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	if err := pub.Write([]byte("deliver-test")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	select {
	case <-sub.C():
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for sample")
	}

	metrics := tp.TopicMetrics()
	var found *dds.TopicMetrics
	for i := range metrics {
		if metrics[i].Topic == "v06/topicmetrics/delivers" {
			found = &metrics[i]
			break
		}
	}
	if found == nil {
		t.Fatal("expected topic in TopicMetrics")
	}
	if found.DeliverCount == 0 {
		t.Error("DeliverCount must be > 0 after delivery")
	}
}

func TestTopicMetrics_EmptyBeforeWrite(t *testing.T) {
	p, err := rtps.New(testDomain, rtps.WithNoMulticast())
	if err != nil {
		t.Skipf("rtps.New: %v", err)
	}
	defer p.Close()

	tp, ok := p.(dds.TopicMetricsProvider)
	if !ok {
		t.Fatal("rtps participant must implement TopicMetricsProvider")
	}
	// No writes yet.
	metrics := tp.TopicMetrics()
	for _, m := range metrics {
		if m.Topic == "v06/notused" {
			t.Error("unexpected topic in metrics before any writes")
		}
	}
}

// ── DiscoveryMetrics ──────────────────────────────────────────────────────────

func TestDiscoveryMetrics_AnnouncesCountUp(t *testing.T) {
	p, err := rtps.New(testDomain)
	if err != nil {
		t.Skipf("rtps.New: %v", err)
	}
	defer p.Close()

	dp, ok := p.(dds.DiscoveryMetricsProvider)
	if !ok {
		t.Fatal("rtps participant must implement DiscoveryMetricsProvider")
	}

	// Wait briefly for at least one SPDP announcement to be sent.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		dm := dp.DiscoveryMetrics()
		if dm.AnnouncesSent > 0 {
			return // success
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Error("AnnouncesSent must be > 0 after participant starts")
}

func TestDiscoveryMetrics_Fields(t *testing.T) {
	p, err := rtps.New(testDomain, rtps.WithNoMulticast())
	if err != nil {
		t.Skipf("rtps.New: %v", err)
	}
	defer p.Close()

	dp, ok := p.(dds.DiscoveryMetricsProvider)
	if !ok {
		t.Fatal("rtps participant must implement DiscoveryMetricsProvider")
	}
	dm := dp.DiscoveryMetrics()
	// Struct fields must be accessible; zero values are fine for a fresh participant
	// with no multicast.
	_ = dm.AnnouncesSent
	_ = dm.AnnouncesReceived
	_ = dm.PeersKnown
	_ = dm.PeerEvictions
	_ = dm.EndpointMatches
}

// ── Health ────────────────────────────────────────────────────────────────────

func TestHealth_OK_WhenOpen(t *testing.T) {
	p, err := rtps.New(testDomain, rtps.WithNoMulticast())
	if err != nil {
		t.Skipf("rtps.New: %v", err)
	}
	defer p.Close()

	hp, ok := p.(dds.HealthProvider)
	if !ok {
		t.Fatal("rtps participant must implement HealthProvider")
	}
	h := hp.Health()
	if h.Status != dds.HealthOK {
		t.Errorf("expected HealthOK, got %v", h.Status)
	}
}

func TestHealth_Down_AfterClose(t *testing.T) {
	p, err := rtps.New(testDomain, rtps.WithNoMulticast())
	if err != nil {
		t.Skipf("rtps.New: %v", err)
	}
	if closeErr := p.Close(); closeErr != nil {
		t.Fatalf("Close: %v", closeErr)
	}

	hp, ok := p.(dds.HealthProvider)
	if !ok {
		t.Fatal("rtps participant must implement HealthProvider")
	}
	h := hp.Health()
	if h.Status != dds.HealthDown {
		t.Errorf("expected HealthDown after close, got %v", h.Status)
	}
}

func TestHealth_Details_ContainWritersReaders(t *testing.T) {
	p, err := rtps.New(testDomain, rtps.WithNoMulticast())
	if err != nil {
		t.Skipf("rtps.New: %v", err)
	}
	defer p.Close()

	pub, err := p.NewPublisher("v06/health/details", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	hp, ok := p.(dds.HealthProvider)
	if !ok {
		t.Fatal("rtps participant must implement HealthProvider")
	}
	h := hp.Health()
	if h.Details == "" {
		t.Fatal("Health.Details must not be empty for an open participant")
	}
	if !strings.Contains(h.Details, "writers") {
		t.Error("Health.Details must mention 'writers'")
	}
	if !strings.Contains(h.Details, "readers") {
		t.Error("Health.Details must mention 'readers'")
	}
}

// ── HealthStatus.String ───────────────────────────────────────────────────────

func TestHealthStatus_String(t *testing.T) {
	cases := []struct {
		s    dds.HealthStatus
		want string
	}{
		{dds.HealthOK, "ok"},
		{dds.HealthDegraded, "degraded"},
		{dds.HealthDown, "down"},
	}
	for _, tc := range cases {
		if got := tc.s.String(); got != tc.want {
			t.Errorf("HealthStatus(%d).String() = %q, want %q", tc.s, got, tc.want)
		}
	}
}
