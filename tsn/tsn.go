// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package tsn provides Time-Sensitive Networking stream descriptors and
// configuration utilities following the OMG DDS-TSN specification.
//
// A TSN stream maps a DDS topic to a bounded-latency IEEE 802.1 network flow.
// Stream descriptors control VLAN tagging, DSCP marking, frame size bounds,
// and scheduled transmit timing (SO_TXTIME / ETF qdisc).
//
// Usage — load from a JSON config file:
//
//	cfg, err := tsn.LoadConfig("tsn_streams.json")
//	p, err := rtps.New(0, rtps.WithTSNConfig(cfg))
//
// Config file format (JSON):
//
//	{
//	  "streams": [
//	    {
//	      "topic":               "vehicle/speed",
//	      "vid":                 100,
//	      "pcp":                 5,
//	      "dscp":                46,
//	      "max_frame_size":      1500,
//	      "max_interval_frames": 1,
//	      "interval_us":         125,
//	      "tx_offset_us":        50,
//	      "talker_id":           "ecu-cluster-1"
//	    }
//	  ]
//	}
package tsn

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"
)

// ErrNoStream is returned when StreamConfig.StreamForTopic finds no match.
var ErrNoStream = errors.New("tsn: no stream configured for topic")

// Stream is a DDS-TSN stream descriptor as defined in the OMG DDS-TSN
// specification (OMG Document ptc/2021-03-01). It binds a DDS topic to
// a TSN flow with specific IEEE 802.1 network scheduling constraints.
type Stream struct {
	// Topic is the DDS topic name this stream applies to (exact match).
	Topic string `json:"topic"`

	// VID is the IEEE 802.1Q VLAN ID (0 = untagged).
	VID uint16 `json:"vid"`

	// PCP is the VLAN Priority Code Point (0–7). Linux maps PCP to a
	// traffic class via tc; the kernel also exposes it as SO_PRIORITY.
	PCP uint8 `json:"pcp"`

	// DSCP is the IP Differentiated Services Code Point (0–63).
	// Written into the IP ToS field (DSCP << 2) via IP_TOS.
	DSCP uint8 `json:"dscp"`

	// MaxFrameSize is the maximum Ethernet payload bytes per frame.
	// Use 1500 for standard Ethernet (no jumbo frames).
	// The RTPS layer uses this to bound DATA_FRAG payload size.
	MaxFrameSize int `json:"max_frame_size"`

	// MaxIntervalFrames is the maximum number of frames allowed per Interval.
	MaxIntervalFrames int `json:"max_interval_frames"`

	// IntervalUS is the TSN transmit interval in microseconds.
	// 125 µs = 8000 fps (Audio Video Bridging Class A / AVTP).
	IntervalUS int64 `json:"interval_us"`

	// TxOffsetUS is the transmit offset within the Interval in microseconds.
	// Used with SO_TXTIME / ETF qdisc for scheduled transmit.
	TxOffsetUS int64 `json:"tx_offset_us"`

	// TalkerID is an informational talker identifier used when programming
	// VLAN / CNC (e.g., IEEE 802.1Qcc YANG model TalkerID).
	TalkerID string `json:"talker_id"`
}

// Interval returns the TSN transmit interval as a time.Duration.
func (s *Stream) Interval() time.Duration {
	return time.Duration(s.IntervalUS) * time.Microsecond
}

// TxOffset returns the transmit offset within the interval as a time.Duration.
func (s *Stream) TxOffset() time.Duration {
	return time.Duration(s.TxOffsetUS) * time.Microsecond
}

// MaxFragPayload returns the maximum RTPS fragment payload bytes for this
// stream, reserving space for RTPS headers (~48 bytes). Returns 0 when
// MaxFrameSize is unset (meaning no fragmentation bound is configured).
func (s *Stream) MaxFragPayload() int {
	const rtpsHeaderOverhead = 48
	if s.MaxFrameSize <= rtpsHeaderOverhead {
		return 0
	}
	return s.MaxFrameSize - rtpsHeaderOverhead
}

// StreamConfig is the top-level structure for a tsn_streams JSON file.
type StreamConfig struct {
	Streams []Stream `json:"streams"`
}

// LoadConfig reads and parses a TSN stream configuration from a JSON file.
// The file must contain a top-level "streams" array of Stream objects.
func LoadConfig(path string) (*StreamConfig, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("tsn: open %s: %w", path, err)
	}
	defer f.Close()
	var cfg StreamConfig
	if err := json.NewDecoder(f).Decode(&cfg); err != nil {
		return nil, fmt.Errorf("tsn: parse %s: %w", path, err)
	}
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("tsn: %s: %w", path, err)
	}
	return &cfg, nil
}

// ParseConfig parses TSN stream configuration from JSON bytes.
func ParseConfig(data []byte) (*StreamConfig, error) {
	var cfg StreamConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("tsn: parse: %w", err)
	}
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("tsn: %w", err)
	}
	return &cfg, nil
}

func (c *StreamConfig) validate() error {
	for i := range c.Streams {
		s := &c.Streams[i]
		if s.Topic == "" {
			return fmt.Errorf("stream[%d]: topic must not be empty", i)
		}
		if s.PCP > 7 {
			return fmt.Errorf("stream %q: PCP %d out of range 0–7", s.Topic, s.PCP)
		}
		if s.DSCP > 63 {
			return fmt.Errorf("stream %q: DSCP %d out of range 0–63", s.Topic, s.DSCP)
		}
		if s.MaxFrameSize < 0 {
			return fmt.Errorf("stream %q: MaxFrameSize must be ≥ 0", s.Topic)
		}
		if s.IntervalUS < 0 {
			return fmt.Errorf("stream %q: IntervalUS must be ≥ 0", s.Topic)
		}
		if s.TxOffsetUS < 0 {
			return fmt.Errorf("stream %q: TxOffsetUS must be ≥ 0", s.Topic)
		}
	}
	return nil
}

// StreamForTopic returns a pointer to the first Stream whose Topic matches,
// or nil if no stream is configured for the given topic.
func (c *StreamConfig) StreamForTopic(topic string) *Stream {
	if c == nil {
		return nil
	}
	for i := range c.Streams {
		if c.Streams[i].Topic == topic {
			return &c.Streams[i]
		}
	}
	return nil
}

// Topics returns the set of all configured topic names.
func (c *StreamConfig) Topics() []string {
	if c == nil {
		return nil
	}
	names := make([]string, len(c.Streams))
	for i, s := range c.Streams {
		names[i] = s.Topic
	}
	return names
}
