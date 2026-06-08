// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package grpcbridge

import (
	"fmt"
	"os"
	"strings"

	dds "github.com/SoundMatt/go-DDS"
	"gopkg.in/yaml.v3"
)

// Config is the bridge configuration loaded from a YAML file.
//
// Example:
//
//	listen: ":9090"
//	auth_token: "shared-secret"
//	topics:
//	  - name: "sensors/temperature"
//	    qos: "reliable"
//	  - name: "vehicle/speed"
//	    qos: "best_effort"
type Config struct {
	// Listen is the gRPC listen address (e.g. ":9090"). Used by LoadAndApply.
	Listen string `yaml:"listen"`
	// AuthToken, if non-empty, requires every RPC to carry a matching Bearer token.
	AuthToken string `yaml:"auth_token"`
	// Topics lists topics to pre-subscribe when the bridge starts.
	Topics []TopicConfig `yaml:"topics"`
}

// TopicConfig configures a single topic in the bridge.
type TopicConfig struct {
	// Name is the DDS topic name (e.g. "sensors/temperature").
	Name string `yaml:"name"`
	// QoS is "reliable" or "best_effort". Defaults to DefaultQoS if unset.
	QoS string `yaml:"qos"`
}

func (tc TopicConfig) qos() dds.QoS {
	switch strings.ToLower(tc.QoS) {
	case "reliable":
		return dds.QoS{Reliability: dds.Reliable}
	case "best_effort", "besteffort":
		return dds.QoS{Reliability: dds.BestEffort}
	default:
		return dds.DefaultQoS
	}
}

// LoadConfig reads a YAML config file at path and returns the parsed Config.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("grpcbridge: read config %q: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("grpcbridge: parse config %q: %w", path, err)
	}
	return &cfg, nil
}

// ApplyConfig pre-subscribes the topics listed in cfg on p. The Bridge opts
// are updated with the config's auth token when opts.AuthToken is empty.
func ApplyConfig(b *Bridge, cfg *Config) error {
	for _, tc := range cfg.Topics {
		if tc.Name == "" {
			continue
		}
		q := tc.qos()
		_, err := b.p.NewSubscriber(tc.Name, q)
		if err != nil {
			return fmt.Errorf("grpcbridge: pre-subscribe %q: %w", tc.Name, err)
		}
	}
	return nil
}
