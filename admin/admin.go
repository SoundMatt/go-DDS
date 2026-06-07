// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package admin provides an HTTP administration API for a go-DDS participant
// (Milestone 10 — Enterprise Services: Administration).
//
// The admin server exposes read-only diagnostic endpoints and a sample
// injection endpoint for remote testing and operations:
//
//	GET  /admin/health      — participant health (requires HealthProvider)
//	GET  /admin/topics      — topic metrics (requires TopicMetricsProvider)
//	GET  /admin/discovery   — discovery metrics (requires DiscoveryMetricsProvider)
//	POST /admin/publish     — inject a sample into a topic
//
// Optionally, bearer-token authentication can be enforced by setting
// [Options.APIKey]. When set, every request must carry the header:
//
//	Authorization: Bearer <key>
//
// The server binds on [Options.Addr] (default ":7070"). Use "host:0" for an
// OS-assigned port (useful in tests).
package admin

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"

	dds "github.com/SoundMatt/go-DDS"
)

// Options configures an admin [Server].
type Options struct {
	// Addr is the TCP address to listen on. Defaults to ":7070".
	Addr string
	// APIKey, if non-empty, requires every request to carry
	// "Authorization: Bearer <APIKey>".
	APIKey string
}

// Server is an HTTP admin API server for a DDS participant.
// It is safe for concurrent use from multiple goroutines.
type Server struct {
	p   dds.Participant
	srv *http.Server
	ln  net.Listener
	key string
}

// New creates and starts an admin Server for p.
// Returns an error if the listen address is unavailable.
func New(p dds.Participant, opts Options) (*Server, error) {
	addr := opts.Addr
	if addr == "" {
		addr = ":7070"
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	s := &Server{p: p, ln: ln, key: opts.APIKey}
	mux := http.NewServeMux()
	mux.HandleFunc("/admin/health", s.auth(s.handleHealth))
	mux.HandleFunc("/admin/topics", s.auth(s.handleTopics))
	mux.HandleFunc("/admin/discovery", s.auth(s.handleDiscovery))
	mux.HandleFunc("/admin/publish", s.auth(s.handlePublish))
	s.srv = &http.Server{Handler: mux}
	go func() { _ = s.srv.Serve(ln) }()
	return s, nil
}

// Addr returns the TCP address the server is listening on.
func (s *Server) Addr() string { return s.ln.Addr().String() }

// Close shuts down the admin server.
func (s *Server) Close() error { return s.srv.Close() }

// ── middleware ────────────────────────────────────────────────────────────────

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.key != "" {
			hdr := r.Header.Get("Authorization")
			if !strings.HasPrefix(hdr, "Bearer ") || strings.TrimPrefix(hdr, "Bearer ") != s.key {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		next(w, r)
	}
}

// ── handlers ──────────────────────────────────────────────────────────────────

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	hp, ok := s.p.(dds.HealthProvider)
	if !ok {
		http.Error(w, "health not available", http.StatusNotImplemented)
		return
	}
	h := hp.Health()
	w.Header().Set("Content-Type", "application/json")
	if h.Status == dds.HealthDown {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":  h.Status.String(),
		"details": h.Details,
	})
}

func (s *Server) handleTopics(w http.ResponseWriter, _ *http.Request) {
	var topics []dds.TopicMetrics
	if tp, ok := s.p.(dds.TopicMetricsProvider); ok {
		topics = tp.TopicMetrics()
	}
	if topics == nil {
		topics = []dds.TopicMetrics{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(topics)
}

func (s *Server) handleDiscovery(w http.ResponseWriter, _ *http.Request) {
	type response struct {
		Metrics any `json:"metrics"`
	}
	var metrics any
	if dp, ok := s.p.(dds.DiscoveryMetricsProvider); ok {
		metrics = dp.DiscoveryMetrics()
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response{Metrics: metrics})
}

// publishRequest is the JSON body accepted by POST /admin/publish.
type publishRequest struct {
	Topic   string `json:"topic"`
	Payload string `json:"payload"` // base64-encoded bytes
}

func (s *Server) handlePublish(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req publishRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Topic == "" {
		http.Error(w, "bad request: topic required", http.StatusBadRequest)
		return
	}
	payload, err := base64.StdEncoding.DecodeString(req.Payload)
	if err != nil {
		http.Error(w, "bad request: payload must be base64", http.StatusBadRequest)
		return
	}
	pub, err := s.p.NewPublisher(req.Topic, dds.DefaultQoS)
	if err != nil {
		if errors.Is(err, dds.ErrClosed) {
			http.Error(w, "participant closed", http.StatusServiceUnavailable)
		} else {
			http.Error(w, "cannot create publisher: "+err.Error(), http.StatusInternalServerError)
		}
		return
	}
	defer pub.Close()
	if err := pub.Write(payload); err != nil {
		http.Error(w, "write error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
