// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package monitor

// Internal tests for HTTP handler write-error and SSE-flusher paths.
// These use an unexported Monitor to call handlers directly with a mock
// ResponseWriter that always returns a write error.

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/tsn"
)

// errResponseWriter is an http.ResponseWriter whose Write always fails.
// It does NOT implement http.Flusher so it also covers the non-Flusher path.
type errResponseWriter struct {
	header http.Header
	code   int
}

func newErrRW() *errResponseWriter {
	return &errResponseWriter{header: make(http.Header)}
}

func (w *errResponseWriter) Header() http.Header      { return w.header }
func (w *errResponseWriter) Write(_ []byte) (int, error) { return 0, errors.New("forced write error") }
func (w *errResponseWriter) WriteHeader(code int)        { w.code = code }

// stubHealthProvider implements dds.HealthProvider returning healthy.
type stubHealthProvider struct{}

func (s *stubHealthProvider) Health() dds.Health { return dds.Health{Status: dds.HealthOK} }

// stubHealthDownProvider returns a HealthDown status to exercise the 503 write path.
type stubHealthDownProvider struct{}

func (s *stubHealthDownProvider) Health() dds.Health { return dds.Health{Status: dds.HealthDown} }

// flusherErrResponseWriter implements http.Flusher but always fails on Write.
type flusherErrResponseWriter struct {
	errResponseWriter
}

func (w *flusherErrResponseWriter) Flush() {}

// stubTopicMetricsProvider implements dds.TopicMetricsProvider.
type stubTopicMetricsProvider struct{}

func (s *stubTopicMetricsProvider) TopicMetrics() []dds.TopicMetrics {
	return []dds.TopicMetrics{{Topic: "test", WriteCount: 1}}
}

// newTestMonitor builds a minimal Monitor for direct handler calls (no TCP listener).
func newTestMonitor() *Monitor {
	ctx, cancel := context.WithCancel(context.Background())
	return &Monitor{
		ctx:         ctx,
		cancel:      cancel,
		clients:     make(map[chan string]struct{}),
		tsnTrackers: make(map[string]*tsn.HealthTracker),
	}
}

func TestHandleIndex_WriteError(t *testing.T) {
	m := newTestMonitor()
	defer m.cancel()
	m.handleIndex(newErrRW(), httptest.NewRequest(http.MethodGet, "/", nil))
	// No panic = pass; write error is silently swallowed.
}

func TestHandleSSE_NonFlusher(t *testing.T) {
	m := newTestMonitor()
	defer m.cancel()
	// errResponseWriter does not implement http.Flusher → triggers the
	// "streaming unsupported" branch.
	req := httptest.NewRequest(http.MethodGet, "/events", nil)
	m.handleSSE(newErrRW(), req)
}

func TestHandleHealth_WriteError_NoProvider(t *testing.T) {
	m := newTestMonitor()
	defer m.cancel()
	m.hp = nil
	m.handleHealth(newErrRW(), httptest.NewRequest(http.MethodGet, "/health", nil))
}

func TestHandleHealth_WriteError_WithProvider(t *testing.T) {
	m := newTestMonitor()
	defer m.cancel()
	// provide a health provider so the second Write path is hit
	m.hp = &stubHealthProvider{}
	m.handleHealth(newErrRW(), httptest.NewRequest(http.MethodGet, "/health", nil))
}

func TestHandleAPITopics_WriteError_NilProvider(t *testing.T) {
	m := newTestMonitor()
	defer m.cancel()
	m.tp = nil
	m.handleAPITopics(newErrRW(), httptest.NewRequest(http.MethodGet, "/api/topics", nil))
}

func TestHandleAPITopics_WriteError_WithProvider(t *testing.T) {
	m := newTestMonitor()
	defer m.cancel()
	m.tp = &stubTopicMetricsProvider{}
	m.handleAPITopics(newErrRW(), httptest.NewRequest(http.MethodGet, "/api/topics", nil))
}

func TestHandleAPIDiagnostics_WriteError(t *testing.T) {
	m := newTestMonitor()
	defer m.cancel()
	m.handleAPIDiagnostics(newErrRW(), httptest.NewRequest(http.MethodGet, "/api/diagnostics", nil))
}

func TestHandleAPITSN_WriteError(t *testing.T) {
	m := newTestMonitor()
	defer m.cancel()
	m.handleAPITSN(newErrRW(), httptest.NewRequest(http.MethodGet, "/api/tsn", nil))
}

func TestHandleHealth_WriteError_HealthDown(t *testing.T) {
	m := newTestMonitor()
	defer m.cancel()
	m.hp = &stubHealthDownProvider{}
	m.handleHealth(newErrRW(), httptest.NewRequest(http.MethodGet, "/health", nil))
}

func TestHandleSSE_FlusherWriteError(t *testing.T) {
	m := newTestMonitor()
	defer m.cancel()

	rw := &flusherErrResponseWriter{errResponseWriter: *newErrRW()}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/events", nil).WithContext(ctx)

	done := make(chan struct{})
	go func() {
		defer close(done)
		m.handleSSE(rw, req)
	}()

	// Wait briefly for handleSSE to register the client, then broadcast.
	time.Sleep(20 * time.Millisecond)
	m.broadcast("test", "data") // Write fails → handleSSE returns

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("handleSSE did not exit after write error")
	}
}
