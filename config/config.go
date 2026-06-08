// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package config provides JSON-based configuration for DDS participants.
//
// Load a configuration file with [LoadConfig] or parse JSON from an [io.Reader]
// with [ParseConfig]. Call [ParticipantConfig.Validate] to resolve duration
// strings before passing the config to an RTPS option such as rtps.WithConfig.
//
// Example config file:
//
//	{
//	    "domain": 0,
//	    "heartbeat_period": "200ms",
//	    "spdp_interval": "2s",
//	    "spdp_jitter": "500ms",
//	    "no_multicast": false,
//	    "peer_locators": ["192.168.1.10:7410"],
//	    "log_level": "info"
//	}
package config

//fusa:req REQ-CONF-001
//fusa:req REQ-CONF-002
//fusa:req REQ-CONF-003
//fusa:req REQ-CONF-004

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"
)

// ParticipantConfig is the JSON-serialisable configuration for a DDS participant.
//
// All duration fields accept Go duration strings (e.g. "200ms", "2s", "500ms").
// Zero values use the implementation default.
type ParticipantConfig struct {
	// Domain is the DDS domain ID (0–232). 0 is valid and is the default.
	Domain int `json:"domain"`

	// HeartbeatPeriod is the interval between periodic HEARTBEAT submessages
	// sent by reliable writers. Default: 200ms.
	HeartbeatPeriod string `json:"heartbeat_period,omitempty"`

	// SPDPInterval is the interval between SPDP participant announcements.
	// Default: 2s.
	SPDPInterval string `json:"spdp_interval,omitempty"`

	// SPDPJitter is the maximum random delay before each SPDP announcement,
	// used to spread simultaneous startup floods. Default: 0 (no jitter).
	SPDPJitter string `json:"spdp_jitter,omitempty"`

	// NoMulticast disables SPDP multicast discovery. Combine with PeerLocators
	// to supply peers explicitly when multicast is unavailable.
	NoMulticast bool `json:"no_multicast,omitempty"`

	// PeerLocators is a list of static unicast peer addresses (host:port) for
	// unicast-only or TSN deployments where SPDP multicast is undesirable.
	PeerLocators []string `json:"peer_locators,omitempty"`

	// LogLevel is the minimum log level: "debug", "info", "warn", or "error".
	// An empty string means "use the application default".
	LogLevel string `json:"log_level,omitempty"`

	// Resolved durations — set by Validate, not from JSON.
	HeartbeatPeriodDur time.Duration `json:"-"`
	SPDPIntervalDur    time.Duration `json:"-"`
	SPDPJitterDur      time.Duration `json:"-"`
}

// LoadConfig reads and parses a JSON participant configuration from the file at path.
// It calls Validate before returning, so HeartbeatPeriodDur, SPDPIntervalDur,
// and SPDPJitterDur are always populated on success.
func LoadConfig(path string) (*ParticipantConfig, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("config: open %s: %w", path, err)
	}
	defer f.Close()
	return ParseConfig(f)
}

// ParseConfig decodes a JSON participant configuration from r.
// It calls Validate before returning.
func ParseConfig(r io.Reader) (*ParticipantConfig, error) {
	var cfg ParticipantConfig
	if err := json.NewDecoder(r).Decode(&cfg); err != nil {
		return nil, fmt.Errorf("config: decode: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// Validate checks all fields for validity and resolves the duration string
// fields (HeartbeatPeriod, SPDPInterval, SPDPJitter) into their time.Duration
// counterparts. It returns an error on the first invalid field encountered.
func (c *ParticipantConfig) Validate() error {
	if c.Domain < 0 || c.Domain > 232 {
		return fmt.Errorf("config: domain %d out of range 0–232", c.Domain)
	}

	if c.HeartbeatPeriod != "" {
		d, err := time.ParseDuration(c.HeartbeatPeriod)
		if err != nil {
			return fmt.Errorf("config: heartbeat_period: %w", err)
		}
		if d <= 0 {
			return errors.New("config: heartbeat_period must be positive")
		}
		c.HeartbeatPeriodDur = d
	}

	if c.SPDPInterval != "" {
		d, err := time.ParseDuration(c.SPDPInterval)
		if err != nil {
			return fmt.Errorf("config: spdp_interval: %w", err)
		}
		if d <= 0 {
			return errors.New("config: spdp_interval must be positive")
		}
		c.SPDPIntervalDur = d
	}

	if c.SPDPJitter != "" {
		d, err := time.ParseDuration(c.SPDPJitter)
		if err != nil {
			return fmt.Errorf("config: spdp_jitter: %w", err)
		}
		if d < 0 {
			return errors.New("config: spdp_jitter must not be negative")
		}
		c.SPDPJitterDur = d
	}

	switch c.LogLevel {
	case "", "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("config: log_level %q must be one of: debug, info, warn, error", c.LogLevel)
	}

	return nil
}
