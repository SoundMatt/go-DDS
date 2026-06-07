// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package config_test

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/SoundMatt/go-DDS/config"
)

// ── ParseConfig ───────────────────────────────────────────────────────────────

func TestParseConfig_MinimalJSON(t *testing.T) {
	const raw = `{"domain":0}`
	cfg, err := config.ParseConfig(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if cfg.Domain != 0 {
		t.Errorf("domain: got %d, want 0", cfg.Domain)
	}
}

func TestParseConfig_AllFields(t *testing.T) {
	const raw = `{
		"domain": 5,
		"heartbeat_period": "100ms",
		"spdp_interval": "3s",
		"spdp_jitter": "250ms",
		"no_multicast": true,
		"peer_locators": ["192.168.1.1:7410", "192.168.1.2:7410"],
		"log_level": "debug"
	}`
	cfg, err := config.ParseConfig(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if cfg.Domain != 5 {
		t.Errorf("domain: got %d, want 5", cfg.Domain)
	}
	if cfg.HeartbeatPeriodDur != 100*time.Millisecond {
		t.Errorf("heartbeat_period: got %v, want 100ms", cfg.HeartbeatPeriodDur)
	}
	if cfg.SPDPIntervalDur != 3*time.Second {
		t.Errorf("spdp_interval: got %v, want 3s", cfg.SPDPIntervalDur)
	}
	if cfg.SPDPJitterDur != 250*time.Millisecond {
		t.Errorf("spdp_jitter: got %v, want 250ms", cfg.SPDPJitterDur)
	}
	if !cfg.NoMulticast {
		t.Error("no_multicast: got false, want true")
	}
	if len(cfg.PeerLocators) != 2 {
		t.Errorf("peer_locators: got %d, want 2", len(cfg.PeerLocators))
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("log_level: got %q, want debug", cfg.LogLevel)
	}
}

func TestParseConfig_InvalidJSON(t *testing.T) {
	_, err := config.ParseConfig(strings.NewReader(`{not valid json`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestParseConfig_EmptyReader(t *testing.T) {
	_, err := config.ParseConfig(strings.NewReader(""))
	if err == nil {
		t.Fatal("expected error for empty input")
	}
}

func TestParseConfig_ValidationError(t *testing.T) {
	// Valid JSON but fails Validate: domain out of range triggers return nil, err.
	_, err := config.ParseConfig(strings.NewReader(`{"domain": -1}`))
	if err == nil {
		t.Fatal("expected error for invalid domain in ParseConfig")
	}
}

// ── LoadConfig ────────────────────────────────────────────────────────────────

func TestLoadConfig_HappyPath(t *testing.T) {
	f, err := os.CreateTemp("", "participant-*.json")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	defer os.Remove(f.Name())
	_ = json.NewEncoder(f).Encode(map[string]any{
		"domain":           42,
		"heartbeat_period": "500ms",
		"log_level":        "warn",
	})
	_ = f.Close()

	cfg, err := config.LoadConfig(f.Name())
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Domain != 42 {
		t.Errorf("domain: got %d, want 42", cfg.Domain)
	}
	if cfg.HeartbeatPeriodDur != 500*time.Millisecond {
		t.Errorf("heartbeat_period: got %v, want 500ms", cfg.HeartbeatPeriodDur)
	}
}

func TestLoadConfig_FileNotFound(t *testing.T) {
	_, err := config.LoadConfig("/nonexistent/path/config.json")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

// ── Validate — domain ─────────────────────────────────────────────────────────

func TestValidate_Domain_Zero(t *testing.T) {
	cfg := &config.ParticipantConfig{Domain: 0}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("domain 0 should be valid: %v", err)
	}
}

func TestValidate_Domain_MaxValid(t *testing.T) {
	cfg := &config.ParticipantConfig{Domain: 232}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("domain 232 should be valid: %v", err)
	}
}

func TestValidate_Domain_Negative(t *testing.T) {
	cfg := &config.ParticipantConfig{Domain: -1}
	if err := cfg.Validate(); err == nil {
		t.Fatal("domain -1 should be invalid")
	}
}

func TestValidate_Domain_TooHigh(t *testing.T) {
	cfg := &config.ParticipantConfig{Domain: 233}
	if err := cfg.Validate(); err == nil {
		t.Fatal("domain 233 should be invalid")
	}
}

// ── Validate — heartbeat_period ───────────────────────────────────────────────

func TestValidate_HeartbeatPeriod_Valid(t *testing.T) {
	cfg := &config.ParticipantConfig{HeartbeatPeriod: "200ms"}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("200ms should be valid: %v", err)
	}
	if cfg.HeartbeatPeriodDur != 200*time.Millisecond {
		t.Errorf("got %v, want 200ms", cfg.HeartbeatPeriodDur)
	}
}

func TestValidate_HeartbeatPeriod_Zero(t *testing.T) {
	cfg := &config.ParticipantConfig{HeartbeatPeriod: "0s"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("0s heartbeat_period should be invalid")
	}
}

func TestValidate_HeartbeatPeriod_Negative(t *testing.T) {
	cfg := &config.ParticipantConfig{HeartbeatPeriod: "-100ms"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("negative heartbeat_period should be invalid")
	}
}

func TestValidate_HeartbeatPeriod_BadSyntax(t *testing.T) {
	cfg := &config.ParticipantConfig{HeartbeatPeriod: "notaduration"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("malformed duration should be invalid")
	}
}

func TestValidate_HeartbeatPeriod_Empty_OK(t *testing.T) {
	cfg := &config.ParticipantConfig{}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("empty heartbeat_period (use default) should be valid: %v", err)
	}
	if cfg.HeartbeatPeriodDur != 0 {
		t.Errorf("unset field should resolve to 0 (use default), got %v", cfg.HeartbeatPeriodDur)
	}
}

// ── Validate — spdp_interval ──────────────────────────────────────────────────

func TestValidate_SPDPInterval_Valid(t *testing.T) {
	cfg := &config.ParticipantConfig{SPDPInterval: "5s"}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("5s should be valid: %v", err)
	}
	if cfg.SPDPIntervalDur != 5*time.Second {
		t.Errorf("got %v, want 5s", cfg.SPDPIntervalDur)
	}
}

func TestValidate_SPDPInterval_Zero(t *testing.T) {
	cfg := &config.ParticipantConfig{SPDPInterval: "0"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("0 spdp_interval should be invalid")
	}
}

func TestValidate_SPDPInterval_BadSyntax(t *testing.T) {
	cfg := &config.ParticipantConfig{SPDPInterval: "bad"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("bad duration should be invalid")
	}
}

// ── Validate — spdp_jitter ────────────────────────────────────────────────────

func TestValidate_SPDPJitter_Valid(t *testing.T) {
	cfg := &config.ParticipantConfig{SPDPJitter: "500ms"}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("500ms should be valid: %v", err)
	}
	if cfg.SPDPJitterDur != 500*time.Millisecond {
		t.Errorf("got %v, want 500ms", cfg.SPDPJitterDur)
	}
}

func TestValidate_SPDPJitter_ZeroOK(t *testing.T) {
	cfg := &config.ParticipantConfig{SPDPJitter: "0s"}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("zero jitter should be valid: %v", err)
	}
	if cfg.SPDPJitterDur != 0 {
		t.Errorf("zero jitter: got %v, want 0", cfg.SPDPJitterDur)
	}
}

func TestValidate_SPDPJitter_Negative(t *testing.T) {
	cfg := &config.ParticipantConfig{SPDPJitter: "-1s"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("negative spdp_jitter should be invalid")
	}
}

func TestValidate_SPDPJitter_BadSyntax(t *testing.T) {
	cfg := &config.ParticipantConfig{SPDPJitter: "whoops"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("bad duration should be invalid")
	}
}

// ── Validate — log_level ──────────────────────────────────────────────────────

func TestValidate_LogLevel_AllValid(t *testing.T) {
	for _, level := range []string{"", "debug", "info", "warn", "error"} {
		cfg := &config.ParticipantConfig{LogLevel: level}
		if err := cfg.Validate(); err != nil {
			t.Errorf("log_level %q should be valid: %v", level, err)
		}
	}
}

func TestValidate_LogLevel_Invalid(t *testing.T) {
	cfg := &config.ParticipantConfig{LogLevel: "verbose"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("unknown log_level should be invalid")
	}
}

// ── JSON round-trip ───────────────────────────────────────────────────────────

func TestParseConfig_JSONRoundTrip(t *testing.T) {
	original := config.ParticipantConfig{
		Domain:          7,
		HeartbeatPeriod: "300ms",
		SPDPInterval:    "4s",
		SPDPJitter:      "100ms",
		NoMulticast:     true,
		PeerLocators:    []string{"10.0.0.1:7410"},
		LogLevel:        "info",
	}
	buf := &bytes.Buffer{}
	if err := json.NewEncoder(buf).Encode(original); err != nil {
		t.Fatalf("encode: %v", err)
	}
	cfg, err := config.ParseConfig(buf)
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if cfg.Domain != original.Domain {
		t.Errorf("domain: got %d, want %d", cfg.Domain, original.Domain)
	}
	if cfg.HeartbeatPeriodDur != 300*time.Millisecond {
		t.Errorf("heartbeat_period: got %v, want 300ms", cfg.HeartbeatPeriodDur)
	}
	if cfg.SPDPIntervalDur != 4*time.Second {
		t.Errorf("spdp_interval: got %v, want 4s", cfg.SPDPIntervalDur)
	}
	if cfg.SPDPJitterDur != 100*time.Millisecond {
		t.Errorf("spdp_jitter: got %v, want 100ms", cfg.SPDPJitterDur)
	}
	if !cfg.NoMulticast {
		t.Error("no_multicast: got false, want true")
	}
	if len(cfg.PeerLocators) != 1 || cfg.PeerLocators[0] != "10.0.0.1:7410" {
		t.Errorf("peer_locators: got %v", cfg.PeerLocators)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("log_level: got %q, want info", cfg.LogLevel)
	}
}

// ── ResolvedDurations not in JSON ─────────────────────────────────────────────

func TestParseConfig_ResolvedDursNotInJSON(t *testing.T) {
	// HeartbeatPeriodDur is tagged json:"-" and must not appear in the output.
	cfg := config.ParticipantConfig{HeartbeatPeriodDur: 99 * time.Second}
	b, _ := json.Marshal(cfg)
	if bytes.Contains(b, []byte("HeartbeatPeriodDur")) || bytes.Contains(b, []byte("99")) {
		t.Errorf("resolved duration should not appear in JSON: %s", b)
	}
}
