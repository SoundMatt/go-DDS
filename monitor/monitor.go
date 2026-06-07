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
	mp      dds.MetricsProvider // non-nil when p implements MetricsProvider
	opts    Options
	server  *http.Server
	ln      net.Listener
	cancel  context.CancelFunc
	ctx     context.Context
	mu      sync.RWMutex
	clients map[chan string]struct{}

	// Intercept publisher and subscriber creation so we can observe samples.
	pubSubs []dds.Subscriber
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

	mux := http.NewServeMux()
	mux.HandleFunc("/", m.handleIndex)
	mux.HandleFunc("/events", m.handleSSE)
	m.server = &http.Server{Handler: mux}

	go m.server.Serve(ln)           //nolint:errcheck
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
	b, _ := json.Marshal(sampleEvent{Topic: s.Topic, Payload: string(s.Payload)})
	m.broadcast("sample", string(b))
}

// Close stops the HTTP server and all background goroutines.
func (m *Monitor) Close() error {
	m.cancel()
	return m.server.Shutdown(context.Background())
}

// ── Internal ──────────────────────────────────────────────────────────────────

func (m *Monitor) handleIndex(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(indexHTML)
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
			_, _ = fmt.Fprint(w, msg)
			flusher.Flush()
		case <-r.Context().Done():
			return
		case <-m.ctx.Done():
			return
		}
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
	if m.mp == nil {
		return
	}
	tick := time.NewTicker(m.opts.metricsInterval())
	defer tick.Stop()
	for {
		select {
		case <-tick.C:
			mt := m.mp.Metrics()
			type metricsEvent struct {
				WriteCount     uint64 `json:"write_count"`
				DeliverCount   uint64 `json:"deliver_count"`
				DropCount      uint64 `json:"drop_count"`
				BytesWritten   uint64 `json:"bytes_written"`
				BytesDelivered uint64 `json:"bytes_delivered"`
			}
			b, _ := json.Marshal(metricsEvent{
				WriteCount:     mt.WriteCount,
				DeliverCount:   mt.DeliverCount,
				DropCount:      mt.DropCount,
				BytesWritten:   mt.BytesWritten,
				BytesDelivered: mt.BytesDelivered,
			})
			m.broadcast("metrics", string(b))
		case <-m.ctx.Done():
			return
		}
	}
}
