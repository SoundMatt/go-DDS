// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package auto provides automatic DDS transport selection. It picks the
// best available transport for a given domain at participant creation time:
//
//   - TransportAuto (default): try shmem first; if unavailable or disabled,
//     fall back to RTPS/UDP.
//   - TransportShmem: use shared-memory transport only; error if unavailable.
//   - TransportRTPS: use RTPS/UDP transport unconditionally; skip shmem.
//
// Usage:
//
//	p, err := auto.NewParticipant(dds.Domain(0))
//	// shmem on same host, rtps when shmem is unavailable
//
//	p, err := auto.NewParticipant(dds.Domain(0), auto.WithTransport(auto.TransportRTPS))
//	// always RTPS, e.g. for cross-host deployments
//
// The returned participant satisfies dds.Participant and may optionally
// implement dds.MetricsProvider, dds.DiscoveryMetricsProvider, and
// dds.HealthProvider depending on the selected backend.
package auto

//fusa:req REQ-AUTO-001
//fusa:req REQ-AUTO-002
//fusa:req REQ-AUTO-003

import (
	"fmt"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/rtps"
	"github.com/SoundMatt/go-DDS/shmem"
)

// newShmem and newRTPS are package-level vars so test code can inject
// failing factories to cover error paths without OS-level manipulation.
var (
	newShmem = func(d dds.Domain) (dds.Participant, error) { return shmem.New(d) }
	newRTPS  = func(d dds.Domain, opts ...rtps.Option) (dds.Participant, error) { return rtps.New(d, opts...) }
)

// Transport identifies which DDS transport to use.
type Transport int

const (
	// TransportAuto tries shmem first and falls back to RTPS/UDP if shmem is
	// unavailable. This is the default.
	TransportAuto Transport = iota
	// TransportShmem uses shared memory only. Returns an error if shmem cannot
	// be initialised (e.g. filesystem permissions, unsupported OS).
	TransportShmem
	// TransportRTPS always uses the RTPS/UDP transport regardless of whether
	// shmem is available. Use this for cross-host deployments.
	TransportRTPS
)

// String returns a human-readable transport name.
func (t Transport) String() string {
	switch t {
	case TransportShmem:
		return "shmem"
	case TransportRTPS:
		return "rtps"
	default:
		return "auto"
	}
}

// Option configures the automatic transport selection.
type Option func(*cfg)

type cfg struct {
	prefer   Transport
	rtpsOpts []rtps.Option
}

// WithTransport overrides the default transport preference.
func WithTransport(t Transport) Option {
	return func(c *cfg) { c.prefer = t }
}

// WithRTPSOpts passes additional options to the RTPS participant when the RTPS
// transport is selected (either directly or as the auto fallback).
func WithRTPSOpts(opts ...rtps.Option) Option {
	return func(c *cfg) { c.rtpsOpts = append(c.rtpsOpts, opts...) }
}

// NewParticipant creates a DDS participant on domain using the best available
// transport. The selection algorithm is determined by the WithTransport option
// (default: TransportAuto).
//
// The returned participant satisfies dds.Participant. Callers can type-assert
// to dds.HealthProvider, dds.MetricsProvider, etc. depending on the backend.
func NewParticipant(domain dds.Domain, opts ...Option) (dds.Participant, error) {
	c := &cfg{prefer: TransportAuto}
	for _, o := range opts {
		o(c)
	}

	switch c.prefer {
	case TransportShmem:
		p, err := newShmem(domain)
		if err != nil {
			return nil, fmt.Errorf("auto: shmem: %w", err)
		}
		return p, nil

	case TransportRTPS:
		p, err := newRTPS(domain, c.rtpsOpts...)
		if err != nil {
			return nil, fmt.Errorf("auto: rtps: %w", err)
		}
		return p, nil

	default: // TransportAuto
		if p, err := newShmem(domain); err == nil {
			return p, nil
		}
		p, err := newRTPS(domain, c.rtpsOpts...)
		if err != nil {
			return nil, fmt.Errorf("auto: %w", err)
		}
		return p, nil
	}
}
