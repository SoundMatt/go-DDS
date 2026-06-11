// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package monitor provides a real-time web dashboard for a DDS participant.
// It embeds a single-page HTML UI (Server-Sent Events) and exposes an HTTP
// server that streams DDS samples and participant metrics to any browser.
//
// Usage:
//
//	p, _ := rtps.New(dds.Domain(0))
//	mon, _ := monitor.New(p, monitor.Options{Addr: ":8080"})
//	defer mon.Close()
//	// browse http://localhost:8080
package monitor

//fusa:req REQ-MON-001
//fusa:req REQ-MON-002
//fusa:req REQ-MON-003
//fusa:req REQ-MON-004
//fusa:req REQ-MON-005
//fusa:req REQ-MON-006
//fusa:req REQ-MON-007

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	_ "embed"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/safety"
	"github.com/SoundMatt/go-DDS/tsn"
)

//go:embed static/index.html
var indexHTML []byte

// Options configures the monitor HTTP server.
type Options struct {
	// Addr is the TCP listen address. Default: ":8080".
	Addr string
	// MetricsInterval controls how often metrics snapshots are pushed to the
	// browser. Default: 2s.
	MetricsInterval time.Duration
}

func (o Options) addr() string {
	if o.Addr == "" {
		return ":8080"
	}
	return o.Addr
}

func (o Options) metricsInterval() time.Duration {
	if o.MetricsInterval <= 0 {
		return 2 * time.Second
	}
	return o.MetricsInterval
}

// Monitor wraps a dds.Participant and serves a real-time web dashboard.
type Monitor struct {
	p       dds.Participant
	mp      dds.MetricsProvider          // non-nil when p implements MetricsProvider
	dp      dds.DiscoveryMetricsProvider // non-nil when p implements DiscoveryMetricsProvider
	tp      dds.TopicMetricsProvider     // non-nil when p implements TopicMetricsProvider
	hp      dds.HealthProvider           // non-nil when p implements HealthProvider
	opts    Options
	server  *http.Server
	ln      net.Listener
	cancel  context.CancelFunc
	ctx     context.Context
	mu      sync.RWMutex
	clients map[chan string]struct{}

	safetyMu        sync.RWMutex
	safetyProviders []safety.SafetyMetricsProvider

	tsnMu       sync.RWMutex
	tsnTrackers map[string]*tsn.HealthTracker
}

// New creates a Monitor wrapping p and starts the HTTP server.
// The caller must call Close() to release the HTTP listener.
func New(p dds.Participant, opts Options) (*Monitor, error) {
	ln, err := net.Listen("tcp", opts.addr())
	if err != nil {
		return nil, fmt.Errorf("monitor: listen %s: %w", opts.addr(), err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	m := &Monitor{
		p:       p,
		opts:    opts,
		ln:      ln,
		cancel:  cancel,
		ctx:     ctx,
		clients: make(map[chan string]struct{}),
	}
	if mp, ok := p.(dds.MetricsProvider); ok {
		m.mp = mp
	}
	if dp, ok := p.(dds.DiscoveryMetricsProvider); ok {
		m.dp = dp
	}
	if tp, ok := p.(dds.TopicMetricsProvider); ok {
		m.tp = tp
	}
	if hp, ok := p.(dds.HealthProvider); ok {
		m.hp = hp
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", m.handleIndex)
	mux.HandleFunc("/events", m.handleSSE)
	mux.HandleFunc("/health", m.handleHealth)
	mux.HandleFunc("/api/topics", m.handleAPITopics)
	mux.HandleFunc("/api/diagnostics", m.handleAPIDiagnostics)
	mux.HandleFunc("/api/tsn", m.handleAPITSN)
	m.server = &http.Server{Handler: mux}

	go m.server.Serve(ln) //nolint:errcheck // background goroutine; error surfaced via server.Shutdown in Close
	go m.metricsLoop()
	return m, nil
}

// Addr returns the address the HTTP server is listening on (useful when Addr
// was ":0" to let the OS pick a port).
func (m *Monitor) Addr() string { return m.ln.Addr().String() }

// Publish emits a sample event to all connected browser clients. Call this
// whenever a sample is written or received that should appear in the dashboard.
func (m *Monitor) Publish(s dds.Sample) {
	type sampleEvent struct {
		Topic   string `json:"topic"`
		Payload string `json:"payload"`
	}
	b, jsonErr := json.Marshal(sampleEvent{Topic: s.Topic, Payload: string(s.Payload)})
	if jsonErr != nil {
		return
	}
	m.broadcast("sample", string(b))
}

// PublishSafetyEvent broadcasts a safety violation event to all connected SSE clients.
func (m *Monitor) PublishSafetyEvent(e safety.SafetyEvent) {
	type safetyEv struct {
		Kind    string `json:"kind"`
		Topic   string `json:"topic"`
		Counter uint32 `json:"counter"`
		Message string `json:"message"`
	}
	b, jsonErr := json.Marshal(safetyEv{
		Kind:    e.Kind.String(),
		Topic:   e.Topic,
		Counter: e.Counter,
		Message: e.Message,
	})
	if jsonErr != nil {
		return
	}
	m.broadcast("safety", string(b))
}

// WatchSafety starts a goroutine that reads from events and calls
// PublishSafetyEvent for each one. It stops when the monitor is closed or
// the channel is closed. Intended to be wired to E2ESubscriber.SafetyEvents().
func (m *Monitor) WatchSafety(events <-chan safety.SafetyEvent) {
	go func() {
		for {
			select {
			case e, ok := <-events:
				if !ok {
					return
				}
				m.PublishSafetyEvent(e)
			case <-m.ctx.Done():
				return
			}
		}
	}()
}

// RegisterSafetyMetrics adds p to the set of safety metrics providers whose
// snapshots are periodically broadcast as "safety_metrics" SSE events.
// May be called after New returns.
func (m *Monitor) RegisterSafetyMetrics(p safety.SafetyMetricsProvider) {
	m.safetyMu.Lock()
	m.safetyProviders = append(m.safetyProviders, p)
	m.safetyMu.Unlock()
}

// RegisterTSNHealth registers a HealthTracker under name. Its health snapshot
// is periodically broadcast as a "tsn_health" SSE event and served at /api/tsn.
// May be called after New returns.
func (m *Monitor) RegisterTSNHealth(name string, ht *tsn.HealthTracker) {
	m.tsnMu.Lock()
	if m.tsnTrackers == nil {
		m.tsnTrackers = make(map[string]*tsn.HealthTracker)
	}
	m.tsnTrackers[name] = ht
	m.tsnMu.Unlock()
}

// Close stops the HTTP server and all background goroutines.
func (m *Monitor) Close() error {
	m.cancel()
	return m.server.Shutdown(context.Background())
}

// ── Internal ──────────────────────────────────────────────────────────────────

func (m *Monitor) handleIndex(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	writeN, writeErr := w.Write(indexHTML)
	_ = writeN
	if writeErr != nil {
		return
	}
}

func (m *Monitor) handleSSE(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	ch := make(chan string, 64)
	m.mu.Lock()
	m.clients[ch] = struct{}{}
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		delete(m.clients, ch)
		m.mu.Unlock()
	}()

	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				return
			}
			writeN, writeErr := fmt.Fprint(w, msg)
			_ = writeN
			if writeErr != nil {
				return
			}
			flusher.Flush()
		case <-r.Context().Done():
			return
		case <-m.ctx.Done():
			return
		}
	}
}

// handleHealth serves GET /health as JSON. Returns 503 when the participant is down.
func (m *Monitor) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if m.hp == nil {
		w.WriteHeader(http.StatusNotImplemented)
		writeN, writeErr := w.Write([]byte(`{"status":"unknown","error":"health reporting not available"}`))
		_ = writeN
		if writeErr != nil {
			return
		}
		return
	}
	h := m.hp.Health()
	type healthResp struct {
		Status  string            `json:"status"`
		Details map[string]string `json:"details,omitempty"`
	}
	b, err := json.Marshal(healthResp{Status: h.Status.String(), Details: h.Details})
	if err != nil {
		return
	}
	if h.Status == dds.HealthDown {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	writeN, writeErr := w.Write(b)
	_ = writeN
	if writeErr != nil {
		return
	}
}

// handleAPITopics serves GET /api/topics as a JSON array of per-topic metrics.
func (m *Monitor) handleAPITopics(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if m.tp == nil {
		writeN, writeErr := w.Write([]byte(`[]`))
		_ = writeN
		if writeErr != nil {
			return
		}
		return
	}
	topics := m.tp.TopicMetrics()
	if topics == nil {
		topics = []dds.TopicMetrics{}
	}
	b, err := json.Marshal(topics)
	if err != nil {
		return
	}
	writeN, writeErr := w.Write(b)
	_ = writeN
	if writeErr != nil {
		return
	}
}

// handleAPIDiagnostics serves GET /api/diagnostics as a JSON object combining
// participant, discovery, and health snapshots.
func (m *Monitor) handleAPIDiagnostics(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	type diagHealth struct {
		Status  string            `json:"status"`
		Details map[string]string `json:"details,omitempty"`
	}
	type diagResp struct {
		Metrics   *dds.Metrics          `json:"metrics,omitempty"`
		Discovery *dds.DiscoveryMetrics `json:"discovery,omitempty"`
		Health    *diagHealth           `json:"health,omitempty"`
	}
	resp := diagResp{}
	if m.mp != nil {
		mt := m.mp.Metrics()
		resp.Metrics = &mt
	}
	if m.dp != nil {
		dm := m.dp.DiscoveryMetrics()
		resp.Discovery = &dm
	}
	if m.hp != nil {
		h := m.hp.Health()
		resp.Health = &diagHealth{Status: h.Status.String(), Details: h.Details}
	}
	b, err := json.Marshal(resp)
	if err != nil {
		return
	}
	writeN, writeErr := w.Write(b)
	_ = writeN
	if writeErr != nil {
		return
	}
}

// handleAPITSN serves GET /api/tsn as a JSON array of TSN stream health snapshots.
func (m *Monitor) handleAPITSN(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	m.tsnMu.RLock()
	trackers := m.tsnTrackers
	m.tsnMu.RUnlock()

	type tsnEntry struct {
		Name          string `json:"name"`
		Topic         string `json:"topic"`
		WriteCount    uint64 `json:"write_count"`
		LateWrites    uint64 `json:"late_writes"`
		MaxLatenessNS int64  `json:"max_lateness_ns"`
		Healthy       bool   `json:"healthy"`
	}
	entries := make([]tsnEntry, 0, len(trackers))
	for name, ht := range trackers {
		h := ht.Health()
		entries = append(entries, tsnEntry{
			Name:          name,
			Topic:         h.Topic,
			WriteCount:    h.WriteCount,
			LateWrites:    h.LateWrites,
			MaxLatenessNS: h.MaxLateness.Nanoseconds(),
			Healthy:       h.Healthy,
		})
	}
	b, err := json.Marshal(entries)
	if err != nil {
		return
	}
	writeN, writeErr := w.Write(b)
	_ = writeN
	if writeErr != nil {
		return
	}
}

func (m *Monitor) broadcast(eventType, data string) {
	msg := fmt.Sprintf("event: %s\ndata: %s\n\n", eventType, data)
	m.mu.RLock()
	defer m.mu.RUnlock()
	for ch := range m.clients {
		select {
		case ch <- msg:
		default: // slow client; drop
		}
	}
}

func (m *Monitor) metricsLoop() {
	tick := time.NewTicker(m.opts.metricsInterval())
	defer tick.Stop()
	for {
		select {
		case <-tick.C:
			m.tickMetrics()
		case <-m.ctx.Done():
			return
		}
	}
}

func (m *Monitor) tickMetrics() {
	if m.mp != nil {
		mt := m.mp.Metrics()
		type metricsEvent struct {
			WriteCount     uint64 `json:"write_count"`
			DeliverCount   uint64 `json:"deliver_count"`
			DropCount      uint64 `json:"drop_count"`
			BytesWritten   uint64 `json:"bytes_written"`
			BytesDelivered uint64 `json:"bytes_delivered"`
		}
		b, jsonErr := json.Marshal(metricsEvent{
			WriteCount:     mt.WriteCount,
			DeliverCount:   mt.DeliverCount,
			DropCount:      mt.DropCount,
			BytesWritten:   mt.BytesWritten,
			BytesDelivered: mt.BytesDelivered,
		})
		if jsonErr == nil {
			m.broadcast("metrics", string(b))
		}
	}

	if m.dp != nil {
		dm := m.dp.DiscoveryMetrics()
		type discoveryEvent struct {
			AnnouncesSent     uint64 `json:"announces_sent"`
			AnnouncesReceived uint64 `json:"announces_received"`
			PeersKnown        uint64 `json:"peers_known"`
			PeerEvictions     uint64 `json:"peer_evictions"`
			EndpointMatches   uint64 `json:"endpoint_matches"`
		}
		b, jsonErr := json.Marshal(discoveryEvent{
			AnnouncesSent:     dm.AnnouncesSent,
			AnnouncesReceived: dm.AnnouncesReceived,
			PeersKnown:        dm.PeersKnown,
			PeerEvictions:     dm.PeerEvictions,
			EndpointMatches:   dm.EndpointMatches,
		})
		if jsonErr == nil {
			m.broadcast("discovery", string(b))
		}
	}

	m.safetyMu.RLock()
	safetyProviders := m.safetyProviders
	m.safetyMu.RUnlock()
	for _, sp := range safetyProviders {
		snap := sp.SafetyMetrics()
		b, jsonErr := json.Marshal(snap)
		if jsonErr == nil {
			m.broadcast("safety_metrics", string(b))
		}
	}

	m.tsnMu.RLock()
	tsnTrackers := m.tsnTrackers
	m.tsnMu.RUnlock()
	type tsnHealthEvent struct {
		Name          string `json:"name"`
		Topic         string `json:"topic"`
		WriteCount    uint64 `json:"write_count"`
		LateWrites    uint64 `json:"late_writes"`
		MaxLatenessNS int64  `json:"max_lateness_ns"`
		Healthy       bool   `json:"healthy"`
	}
	for name, ht := range tsnTrackers {
		h := ht.Health()
		b, jsonErr := json.Marshal(tsnHealthEvent{
			Name:          name,
			Topic:         h.Topic,
			WriteCount:    h.WriteCount,
			LateWrites:    h.LateWrites,
			MaxLatenessNS: h.MaxLateness.Nanoseconds(),
			Healthy:       h.Healthy,
		})
		if jsonErr == nil {
			m.broadcast("tsn_health", string(b))
		}
	}
}
