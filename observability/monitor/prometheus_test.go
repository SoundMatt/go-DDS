// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package monitor_test

import (
	"bufio"
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/mock"
	"github.com/SoundMatt/go-DDS/observability/monitor"
)

// newIsolatedParticipant returns a mock participant on its own broker,
// unshared with any other participant in this test binary (unlike
// newMockParticipant's plain mock.New, which — per mock.New's doc comment —
// shares one global broker across every domain-0 participant in the
// process). Prometheus tests asserting exact counter/gauge values need this
// isolation; tests that only check for a metric's presence do not.
func newIsolatedParticipant(t *testing.T) dds.Participant {
	t.Helper()
	p, err := mock.New(dds.Domain(0), mock.IsolatedBroker())
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func bodyOf(t *testing.T, resp *http.Response) string {
	t.Helper()
	var b strings.Builder
	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		b.WriteString(sc.Text())
		b.WriteByte('\n')
	}
	return b.String()
}

func TestMonitor_Metrics_ServesPrometheusText(t *testing.T) {
	p := newIsolatedParticipant(t)
	defer p.Close()
	mon, err := monitor.New(p, monitor.Options{Addr: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	defer mon.Close()

	pub, err := p.NewPublisher("monitor/prom/topic", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()
	if writeErr := pub.Write([]byte("hello")); writeErr != nil {
		t.Fatalf("Write: %v", writeErr)
	}

	ctx := context.Background()
	resp, err := get(ctx, "http://"+mon.Addr()+"/metrics")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Fatalf("expected text/plain, got %q", ct)
	}

	body := bodyOf(t, resp)
	for _, want := range []string{
		"# TYPE dds_active_topics gauge",
		"dds_active_topics 1",
		"# TYPE dds_matched_readers gauge",
		"# TYPE dds_matched_writers gauge",
		"# TYPE dds_participant_count gauge",
		"# TYPE dds_samples_published_total counter",
		"dds_samples_published_total 1",
		"# TYPE dds_samples_received_total counter",
		"# TYPE dds_samples_dropped_total counter",
		"# TYPE dds_bytes_out_total counter",
		"# TYPE dds_bytes_in_total counter",
		"# TYPE dds_cdr_encode_errors_total counter",
		"# TYPE dds_cdr_decode_errors_total counter",
		"# TYPE dds_latency_seconds histogram",
		"dds_latency_seconds_bucket{le=\"+Inf\"}",
		"dds_latency_seconds_sum",
		"dds_latency_seconds_count",
		"# TYPE dds_queue_depth histogram",
		"dds_queue_depth_bucket{le=\"+Inf\"}",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("expected /metrics body to contain %q; got:\n%s", want, body)
		}
	}
}

func TestMonitor_Metrics_NoProviders_StillServes(t *testing.T) {
	// A participant implementing none of MetricsProvider, DiscoveryMetricsProvider,
	// or TopicMetricsProvider must still produce a valid (if zeroed) /metrics body.
	type minPart struct{ dds.Participant }
	realPart := newMockParticipant(t)
	defer realPart.Close()
	stub := &minPart{Participant: realPart}

	mon, err := monitor.New(stub, monitor.Options{Addr: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	defer mon.Close()

	ctx := context.Background()
	resp, err := get(ctx, "http://"+mon.Addr()+"/metrics")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body := bodyOf(t, resp)
	if !strings.Contains(body, "dds_active_topics 0") {
		t.Errorf("expected dds_active_topics 0 with no TopicMetricsProvider; got:\n%s", body)
	}
	if !strings.Contains(body, "dds_participant_count 0") {
		t.Errorf("expected dds_participant_count 0 with no DiscoveryMetricsProvider; got:\n%s", body)
	}
	if strings.Contains(body, "dds_samples_published_total") {
		t.Errorf("expected no dds_samples_published_total series with no MetricsProvider; got:\n%s", body)
	}
}

func TestMonitor_SetMatched_ReflectedInMetrics(t *testing.T) {
	p := newMockParticipant(t)
	defer p.Close()
	mon, err := monitor.New(p, monitor.Options{Addr: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	defer mon.Close()

	mon.SetMatched(3, 5)

	ctx := context.Background()
	resp, err := get(ctx, "http://"+mon.Addr()+"/metrics")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body := bodyOf(t, resp)
	if !strings.Contains(body, "dds_matched_readers 3") {
		t.Errorf("expected dds_matched_readers 3; got:\n%s", body)
	}
	if !strings.Contains(body, "dds_matched_writers 5") {
		t.Errorf("expected dds_matched_writers 5; got:\n%s", body)
	}
}

func TestMonitor_CDRErrorCounters_Increment(t *testing.T) {
	p := newMockParticipant(t)
	defer p.Close()
	mon, err := monitor.New(p, monitor.Options{Addr: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	defer mon.Close()

	mon.IncCDREncodeError()
	mon.IncCDREncodeError()
	mon.IncCDRDecodeError()

	ctx := context.Background()
	resp, err := get(ctx, "http://"+mon.Addr()+"/metrics")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body := bodyOf(t, resp)
	if !strings.Contains(body, "dds_cdr_encode_errors_total 2") {
		t.Errorf("expected dds_cdr_encode_errors_total 2; got:\n%s", body)
	}
	if !strings.Contains(body, "dds_cdr_decode_errors_total 1") {
		t.Errorf("expected dds_cdr_decode_errors_total 1; got:\n%s", body)
	}
}

func TestMonitor_ObserveLatencyAndQueueDepth_AppearInHistogram(t *testing.T) {
	p := newMockParticipant(t)
	defer p.Close()
	mon, err := monitor.New(p, monitor.Options{Addr: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	defer mon.Close()

	mon.ObserveLatency(2 * time.Millisecond)
	mon.ObserveLatency(20 * time.Second) // exceeds every bucket -> +Inf overflow
	mon.ObserveQueueDepth(3)
	mon.ObserveQueueDepth(9999) // exceeds every bucket -> +Inf overflow

	ctx := context.Background()
	resp, err := get(ctx, "http://"+mon.Addr()+"/metrics")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body := bodyOf(t, resp)
	if !strings.Contains(body, "dds_latency_seconds_count 2") {
		t.Errorf("expected dds_latency_seconds_count 2; got:\n%s", body)
	}
	if !strings.Contains(body, `dds_latency_seconds_bucket{le="0.005"} 1`) {
		t.Errorf("expected the 2ms observation in the 0.005s bucket; got:\n%s", body)
	}
	if !strings.Contains(body, `dds_latency_seconds_bucket{le="+Inf"} 2`) {
		t.Errorf("expected +Inf bucket to include the 20s overflow observation; got:\n%s", body)
	}
	if !strings.Contains(body, "dds_queue_depth_count 2") {
		t.Errorf("expected dds_queue_depth_count 2; got:\n%s", body)
	}
	if !strings.Contains(body, `dds_queue_depth_bucket{le="4"} 1`) {
		t.Errorf("expected the depth-3 observation in the le=4 bucket; got:\n%s", body)
	}
	if !strings.Contains(body, `dds_queue_depth_bucket{le="+Inf"} 2`) {
		t.Errorf("expected +Inf bucket to include the depth-9999 overflow observation; got:\n%s", body)
	}
}

func TestMonitor_WithPrometheus_DedicatedServer(t *testing.T) {
	p := newMockParticipant(t)
	defer p.Close()
	mon, err := monitor.New(p, monitor.Options{Addr: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	defer mon.Close()

	if addr := mon.PrometheusAddr(); addr != "" {
		t.Fatalf("expected empty PrometheusAddr before WithPrometheus, got %q", addr)
	}

	mon2, err := mon.WithPrometheus("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	if mon2 != mon {
		t.Fatal("expected WithPrometheus to return the same *Monitor")
	}
	promAddr := mon.PrometheusAddr()
	if promAddr == "" {
		t.Fatal("expected non-empty PrometheusAddr after WithPrometheus")
	}
	if promAddr == mon.Addr() {
		t.Fatal("expected the dedicated Prometheus server to listen on a different address")
	}

	ctx := context.Background()
	resp, err := get(ctx, "http://"+promAddr+"/metrics")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from dedicated prometheus server, got %d", resp.StatusCode)
	}
	body := bodyOf(t, resp)
	if !strings.Contains(body, "dds_active_topics") {
		t.Errorf("expected dds_active_topics on dedicated server; got:\n%s", body)
	}

	// The dedicated server must serve only /metrics, not the full dashboard.
	resp2, err := get(ctx, "http://"+promAddr+"/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for / on dedicated prometheus server, got %d", resp2.StatusCode)
	}
}

func TestMonitor_WithPrometheus_ListenError(t *testing.T) {
	p := newMockParticipant(t)
	defer p.Close()
	mon, err := monitor.New(p, monitor.Options{Addr: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	defer mon.Close()

	ignoredRet, err := mon.WithPrometheus("127.0.0.1:99999")
	_ = ignoredRet
	if err == nil {
		t.Fatal("expected error for invalid listen address")
	}
}

func TestMonitor_Close_ShutsDownPrometheusServer(t *testing.T) {
	p := newMockParticipant(t)
	defer p.Close()
	mon, err := monitor.New(p, monitor.Options{Addr: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mon.WithPrometheus("127.0.0.1:0"); err != nil {
		t.Fatal(err)
	}
	promAddr := mon.PrometheusAddr()

	if err := mon.Close(); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	resp, reqErr := get(ctx, "http://"+promAddr+"/metrics")
	if reqErr == nil {
		resp.Body.Close()
		t.Fatal("expected error after monitor closed")
	}
}
