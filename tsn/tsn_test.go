// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package tsn_test

//fusa:test REQ-TSN-001
//fusa:test REQ-TSN-002
//fusa:test REQ-TSN-003
//fusa:test REQ-TSN-004

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/SoundMatt/go-DDS/tsn"
)

const sampleJSON = `{
  "streams": [
    {
      "topic": "vehicle/speed",
      "vid": 100,
      "pcp": 5,
      "dscp": 46,
      "max_frame_size": 1500,
      "max_interval_frames": 1,
      "interval_us": 125,
      "tx_offset_us": 50,
      "talker_id": "ecu-cluster-1"
    },
    {
      "topic": "vehicle/steering",
      "vid": 100,
      "pcp": 4,
      "dscp": 34,
      "max_frame_size": 1500,
      "max_interval_frames": 2,
      "interval_us": 250,
      "tx_offset_us": 0,
      "talker_id": "ecu-cluster-2"
    }
  ]
}`

func TestParseConfig_Valid(t *testing.T) {
	cfg, err := tsn.ParseConfig([]byte(sampleJSON))
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if len(cfg.Streams) != 2 {
		t.Fatalf("got %d streams, want 2", len(cfg.Streams))
	}
	s := cfg.Streams[0]
	if s.Topic != "vehicle/speed" {
		t.Errorf("topic = %q, want vehicle/speed", s.Topic)
	}
	if s.PCP != 5 {
		t.Errorf("PCP = %d, want 5", s.PCP)
	}
	if s.DSCP != 46 {
		t.Errorf("DSCP = %d, want 46", s.DSCP)
	}
	if s.VID != 100 {
		t.Errorf("VID = %d, want 100", s.VID)
	}
	if s.MaxFrameSize != 1500 {
		t.Errorf("MaxFrameSize = %d, want 1500", s.MaxFrameSize)
	}
	if s.MaxIntervalFrames != 1 {
		t.Errorf("MaxIntervalFrames = %d, want 1", s.MaxIntervalFrames)
	}
	if s.TalkerID != "ecu-cluster-1" {
		t.Errorf("TalkerID = %q, want ecu-cluster-1", s.TalkerID)
	}
}

func TestStream_DurationHelpers(t *testing.T) {
	s := tsn.Stream{IntervalUS: 125, TxOffsetUS: 50}
	if got := s.Interval(); got != 125*time.Microsecond {
		t.Errorf("Interval() = %v, want 125µs", got)
	}
	if got := s.TxOffset(); got != 50*time.Microsecond {
		t.Errorf("TxOffset() = %v, want 50µs", got)
	}
}

func TestStream_MaxFragPayload(t *testing.T) {
	cases := []struct {
		maxFrameSize int
		want         int
	}{
		{1500, 1452}, // 1500 - 48
		{64, 16},     // 64 - 48
		{47, 0},      // ≤ overhead → 0
		{0, 0},       // unset → 0
	}
	for _, tc := range cases {
		s := tsn.Stream{MaxFrameSize: tc.maxFrameSize}
		if got := s.MaxFragPayload(); got != tc.want {
			t.Errorf("MaxFrameSize=%d: MaxFragPayload()=%d, want %d", tc.maxFrameSize, got, tc.want)
		}
	}
}

func TestStreamConfig_StreamForTopic(t *testing.T) {
	cfg, err := tsn.ParseConfig([]byte(sampleJSON))
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	s := cfg.StreamForTopic("vehicle/speed")
	if s == nil {
		t.Fatal("StreamForTopic(vehicle/speed) = nil")
	}
	if s.PCP != 5 {
		t.Errorf("PCP = %d, want 5", s.PCP)
	}
	if got := cfg.StreamForTopic("unknown"); got != nil {
		t.Errorf("StreamForTopic(unknown) = %v, want nil", got)
	}
}

func TestStreamConfig_NilSafe(t *testing.T) {
	var cfg *tsn.StreamConfig
	if got := cfg.StreamForTopic("x"); got != nil {
		t.Errorf("nil StreamConfig.StreamForTopic = %v, want nil", got)
	}
	if got := cfg.Topics(); got != nil {
		t.Errorf("nil StreamConfig.Topics = %v, want nil", got)
	}
}

func TestStreamConfig_Topics(t *testing.T) {
	cfg, err := tsn.ParseConfig([]byte(sampleJSON))
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	topics := cfg.Topics()
	if len(topics) != 2 {
		t.Fatalf("got %d topics, want 2", len(topics))
	}
	want := map[string]bool{"vehicle/speed": true, "vehicle/steering": true}
	for _, t2 := range topics {
		if !want[t2] {
			t.Errorf("unexpected topic %q", t2)
		}
	}
}

func TestLoadConfig_FromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tsn.json")
	if err := os.WriteFile(path, []byte(sampleJSON), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cfg, err := tsn.LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if len(cfg.Streams) != 2 {
		t.Errorf("got %d streams, want 2", len(cfg.Streams))
	}
}

func TestLoadConfig_FileNotFound(t *testing.T) {
	ignoredRet, err := tsn.LoadConfig("/nonexistent/path/tsn.json")
	_ = ignoredRet
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestLoadConfig_DecodeError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, []byte(`{broken json`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	ignoredRet, err := tsn.LoadConfig(path)
	_ = ignoredRet
	if err == nil {
		t.Error("expected error for invalid JSON file")
	}
}

func TestLoadConfig_ValidationError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "invalid.json")
	// Valid JSON but stream has empty topic → fails Validate.
	data := `{"streams":[{"topic":"","pcp":1}]}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	ignoredRet, err := tsn.LoadConfig(path)
	_ = ignoredRet
	if err == nil {
		t.Error("expected validation error from LoadConfig")
	}
}

func TestParseConfig_InvalidJSON(t *testing.T) {
	ignoredRet, err := tsn.ParseConfig([]byte(`{broken`))
	_ = ignoredRet
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestParseConfig_EmptyTopic(t *testing.T) {
	bad := `{"streams":[{"topic":"","pcp":1}]}`
	ignoredRet, err := tsn.ParseConfig([]byte(bad))
	_ = ignoredRet
	if err == nil {
		t.Error("expected error for empty topic")
	}
}

func TestParseConfig_PCPOutOfRange(t *testing.T) {
	bad := `{"streams":[{"topic":"x","pcp":8}]}`
	ignoredRet, err := tsn.ParseConfig([]byte(bad))
	_ = ignoredRet
	if err == nil {
		t.Error("expected error for PCP > 7")
	}
}

func TestParseConfig_DSCPOutOfRange(t *testing.T) {
	bad := `{"streams":[{"topic":"x","dscp":64}]}`
	ignoredRet, err := tsn.ParseConfig([]byte(bad))
	_ = ignoredRet
	if err == nil {
		t.Error("expected error for DSCP > 63")
	}
}

func TestParseConfig_RoundTrip(t *testing.T) {
	cfg, err := tsn.ParseConfig([]byte(sampleJSON))
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	b, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	cfg2, err := tsn.ParseConfig(b)
	if err != nil {
		t.Fatalf("ParseConfig roundtrip: %v", err)
	}
	if len(cfg2.Streams) != len(cfg.Streams) {
		t.Errorf("roundtrip: got %d streams, want %d", len(cfg2.Streams), len(cfg.Streams))
	}
	for i := range cfg.Streams {
		if cfg2.Streams[i].Topic != cfg.Streams[i].Topic {
			t.Errorf("stream[%d].Topic mismatch after roundtrip", i)
		}
	}
}

func TestParseConfig_NegativeMaxFrameSize(t *testing.T) {
	bad := `{"streams":[{"topic":"x","max_frame_size":-1}]}`
	ignoredRet, err := tsn.ParseConfig([]byte(bad))
	_ = ignoredRet
	if err == nil {
		t.Error("expected error for negative max_frame_size")
	}
}

func TestParseConfig_NegativeMaxIntervalFrames(t *testing.T) {
	bad := `{"streams":[{"topic":"x","max_interval_frames":-1}]}`
	ignoredRet, err := tsn.ParseConfig([]byte(bad))
	_ = ignoredRet
	if err == nil {
		t.Error("expected error for negative max_interval_frames")
	}
}

func TestParseConfig_NegativeIntervalUS(t *testing.T) {
	bad := `{"streams":[{"topic":"x","interval_us":-1}]}`
	ignoredRet, err := tsn.ParseConfig([]byte(bad))
	_ = ignoredRet
	if err == nil {
		t.Error("expected error for negative interval_us")
	}
}

func TestParseConfig_NegativeTxOffsetUS(t *testing.T) {
	bad := `{"streams":[{"topic":"x","tx_offset_us":-1}]}`
	ignoredRet, err := tsn.ParseConfig([]byte(bad))
	_ = ignoredRet
	if err == nil {
		t.Error("expected error for negative tx_offset_us")
	}
}

func TestParseConfig_EmptyStreams(t *testing.T) {
	cfg, err := tsn.ParseConfig([]byte(`{"streams":[]}`))
	if err != nil {
		t.Fatalf("unexpected error for empty streams: %v", err)
	}
	if len(cfg.Streams) != 0 {
		t.Errorf("got %d streams, want 0", len(cfg.Streams))
	}
}
