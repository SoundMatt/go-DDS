// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package monitor

// Prometheus text-exposition support (ROADMAP.md, Milestone 15 "Cloud-Native
// Runtime", "Prometheus Metrics"). Hand-rolled rather than depending on
// github.com/prometheus/client_golang: the text format is a handful of lines
// per metric and the whole point of this sub-phase is a small, dependency-
// free `/metrics` endpoint consistent with the "Pure Go first" guiding
// principle.
//
// Gauges (dds_active_topics, dds_participant_count) and counters
// (dds_samples_{published,received,dropped}_total, dds_bytes_{out,in}_total)
// are derived live from the same MetricsProvider / DiscoveryMetricsProvider /
// TopicMetricsProvider interfaces the SSE dashboard already uses, so they
// need no extra wiring and are automatically "compatible with existing SSE
// dashboard" (same source of truth, two exposition formats).
//
// dds_matched_readers, dds_matched_writers, dds_cdr_encode_errors_total,
// dds_cdr_decode_errors_total, dds_latency_seconds, and dds_queue_depth have
// no equivalent existing provider interface anywhere in the codebase (no
// backend currently tracks matched-endpoint counts or CDR error counts, and
// no package records end-to-end latency or queue-depth samples). Rather than
// invasively adding that instrumentation to rtps/mock/shmem/cdr in this
// pass, they are exposed as Monitor hook methods (SetMatched, IncCDR*,
// Observe*) that default to zero until application code — or a future
// instrumentation pass in those packages — calls them, mirroring the
// existing post-New() registration pattern used by RegisterSafetyMetrics
// and RegisterTSNHealth.

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// defaultLatencyBuckets are the dds_latency_seconds histogram bucket upper
// bounds, in seconds — the same default bucket set client_golang ships,
// suitable for sub-millisecond-to-multi-second DDS end-to-end latency.
var defaultLatencyBuckets = []float64{
	.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10,
}

// defaultQueueDepthBuckets are the dds_queue_depth histogram bucket upper
// bounds, in queued-sample count.
var defaultQueueDepthBuckets = []float64{
	0, 1, 2, 4, 8, 16, 32, 64, 128, 256, 512, 1024,
}

// promHistogram is a minimal, dependency-free Prometheus-style cumulative
// histogram: each Observe increments exactly one internal bucket counter in
// O(log n); export time (cumulative) walks the (small, fixed) bucket list
// once to produce the running totals the Prometheus text format expects.
type promHistogram struct {
	mu     sync.Mutex
	bounds []float64 // ascending upper bounds, exclusive of +Inf
	hits   []uint64  // len(bounds)+1; hits[len(bounds)] is the +Inf overflow bucket
	sum    float64
	count  uint64
}

func newPromHistogram(bounds []float64) *promHistogram {
	return &promHistogram{
		bounds: bounds,
		hits:   make([]uint64, len(bounds)+1),
	}
}

// observe records one sample. Values greater than every configured bound
// fall into the implicit +Inf bucket.
func (h *promHistogram) observe(v float64) {
	idx := sort.SearchFloat64s(h.bounds, v)
	h.mu.Lock()
	h.hits[idx]++
	h.sum += v
	h.count++
	h.mu.Unlock()
}

// cumulative returns, for each configured bound, the count of observations
// less than or equal to that bound (the semantics of a Prometheus histogram
// `_bucket{le="..."}` series), plus the overall sum and count.
func (h *promHistogram) cumulative() (bounds []float64, counts []uint64, sum float64, count uint64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	bounds = append([]float64(nil), h.bounds...)
	counts = make([]uint64, len(h.bounds))
	var running uint64
	for i := range h.bounds {
		running += h.hits[i]
		counts[i] = running
	}
	return bounds, counts, h.sum, h.count
}

// promMetrics holds the subset of Prometheus series with no existing
// provider interface to derive them from. See the file doc comment above.
type promMetrics struct {
	matchedReaders  atomic.Int64
	matchedWriters  atomic.Int64
	cdrEncodeErrors atomic.Uint64
	cdrDecodeErrors atomic.Uint64
	latency         *promHistogram
	queueDepth      *promHistogram
}

func newPromMetrics() *promMetrics {
	return &promMetrics{
		latency:    newPromHistogram(defaultLatencyBuckets),
		queueDepth: newPromHistogram(defaultQueueDepthBuckets),
	}
}

// SetMatched sets the current count of matched (locally known) readers and
// writers reported by the dds_matched_readers / dds_matched_writers
// Prometheus gauges. Both default to 0 until first called; intended to be
// called periodically by application code that tracks endpoint matching.
func (m *Monitor) SetMatched(readers, writers int) {
	m.prom.matchedReaders.Store(int64(readers))
	m.prom.matchedWriters.Store(int64(writers))
}

// IncCDREncodeError increments the dds_cdr_encode_errors_total Prometheus
// counter by one. Intended to be called from codec Marshal error paths.
func (m *Monitor) IncCDREncodeError() {
	m.prom.cdrEncodeErrors.Add(1)
}

// IncCDRDecodeError increments the dds_cdr_decode_errors_total Prometheus
// counter by one. Intended to be called from codec Unmarshal error paths.
func (m *Monitor) IncCDRDecodeError() {
	m.prom.cdrDecodeErrors.Add(1)
}

// ObserveLatency records one end-to-end sample latency observation (e.g.
// publish timestamp to deliver timestamp) into the dds_latency_seconds
// Prometheus histogram. Prometheus/Grafana compute p50/p95/p99 from the
// exposed buckets via histogram_quantile(); go-DDS does not compute
// percentiles itself.
func (m *Monitor) ObserveLatency(d time.Duration) {
	m.prom.latency.observe(d.Seconds())
}

// ObserveQueueDepth records one queue-depth sample into the dds_queue_depth
// Prometheus histogram ("queue depth over time" — ROADMAP.md).
func (m *Monitor) ObserveQueueDepth(depth int) {
	m.prom.queueDepth.observe(float64(depth))
}

// WithPrometheus starts a dedicated HTTP server on addr serving only
// GET /metrics, in Prometheus text exposition format. It returns m for
// chaining (e.g. `mon, err := monitor.New(p, opts); mon, err =
// mon.WithPrometheus(":9090")`).
//
// GET /metrics is always additionally available on the main monitor server
// (Options.Addr) — WithPrometheus is only needed when a deployment wants
// metrics scraped from a separate port, the common Kubernetes convention of
// keeping the scrape endpoint off the application's primary port. May be
// called after New returns; Close on the Monitor also shuts this server
// down.
func (m *Monitor) WithPrometheus(addr string) (*Monitor, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("monitor: prometheus listen %s: %w", addr, err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/metrics", m.handleMetrics)
	srv := &http.Server{Handler: mux}

	m.promMu.Lock()
	m.promServer = srv
	m.promLn = ln
	m.promMu.Unlock()

	go srv.Serve(ln) //nolint:errcheck // background goroutine; error surfaced via server.Shutdown in Close
	return m, nil
}

// PrometheusAddr returns the address the dedicated server started by
// WithPrometheus is listening on, or "" if WithPrometheus was never called.
func (m *Monitor) PrometheusAddr() string {
	m.promMu.RLock()
	defer m.promMu.RUnlock()
	if m.promLn == nil {
		return ""
	}
	return m.promLn.Addr().String()
}

// closePrometheus shuts down the dedicated Prometheus server, if one was
// started via WithPrometheus. Safe to call unconditionally from Close.
func (m *Monitor) closePrometheus() {
	m.promMu.RLock()
	srv := m.promServer
	m.promMu.RUnlock()
	if srv != nil {
		_ = srv.Shutdown(context.Background())
	}
}

// handleMetrics serves GET /metrics in Prometheus text exposition format:
// https://prometheus.io/docs/instrumenting/exposition_formats/
func (m *Monitor) handleMetrics(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

	var b strings.Builder

	activeTopics := 0.0
	if m.tp != nil {
		activeTopics = float64(len(m.tp.TopicMetrics()))
	}
	writeGauge(&b, "dds_active_topics", "Number of topics currently observed by the participant.", activeTopics)
	writeGauge(&b, "dds_matched_readers", "Number of matched (locally known) readers.", float64(m.prom.matchedReaders.Load()))
	writeGauge(&b, "dds_matched_writers", "Number of matched (locally known) writers.", float64(m.prom.matchedWriters.Load()))

	participants := 0.0
	if m.dp != nil {
		participants = float64(m.dp.DiscoveryMetrics().PeersKnown) + 1 // +1 for self
	}
	writeGauge(&b, "dds_participant_count", "Number of participants in the domain, including self.", participants)

	if m.mp != nil {
		mt := m.mp.Metrics()
		writeCounter(&b, "dds_samples_published_total", "Cumulative number of samples published.", float64(mt.WriteCount))
		writeCounter(&b, "dds_samples_received_total", "Cumulative number of samples received.", float64(mt.DeliverCount))
		writeCounter(&b, "dds_samples_dropped_total", "Cumulative number of samples dropped.", float64(mt.DropCount))
		writeCounter(&b, "dds_bytes_out_total", "Cumulative bytes written (published).", float64(mt.BytesWritten))
		writeCounter(&b, "dds_bytes_in_total", "Cumulative bytes received (delivered).", float64(mt.BytesDelivered))
	}

	writeCounter(&b, "dds_cdr_encode_errors_total", "Cumulative CDR encode errors.", float64(m.prom.cdrEncodeErrors.Load()))
	writeCounter(&b, "dds_cdr_decode_errors_total", "Cumulative CDR decode errors.", float64(m.prom.cdrDecodeErrors.Load()))

	writeHistogram(&b, "dds_latency_seconds", "End-to-end sample latency in seconds.", m.prom.latency)
	writeHistogram(&b, "dds_queue_depth", "Queue depth sampled over time.", m.prom.queueDepth)

	writeN, writeErr := w.Write([]byte(b.String()))
	_ = writeN
	if writeErr != nil {
		return
	}
}

func writeGauge(b *strings.Builder, name, help string, v float64) {
	fmt.Fprintf(b, "# HELP %s %s\n# TYPE %s gauge\n%s %s\n", name, help, name, name, formatFloat(v))
}

func writeCounter(b *strings.Builder, name, help string, v float64) {
	fmt.Fprintf(b, "# HELP %s %s\n# TYPE %s counter\n%s %s\n", name, help, name, name, formatFloat(v))
}

func writeHistogram(b *strings.Builder, name, help string, h *promHistogram) {
	fmt.Fprintf(b, "# HELP %s %s\n# TYPE %s histogram\n", name, help, name)
	bounds, counts, sum, count := h.cumulative()
	for i, bound := range bounds {
		fmt.Fprintf(b, "%s_bucket{le=%q} %d\n", name, formatFloat(bound), counts[i])
	}
	fmt.Fprintf(b, "%s_bucket{le=\"+Inf\"} %d\n", name, count)
	fmt.Fprintf(b, "%s_sum %s\n", name, formatFloat(sum))
	fmt.Fprintf(b, "%s_count %d\n", name, count)
}

func formatFloat(v float64) string {
	return strconv.FormatFloat(v, 'g', -1, 64)
}
