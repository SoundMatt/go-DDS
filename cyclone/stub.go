//go:build !cyclone

// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package cyclone provides a CycloneDDS-backed implementation of the dds
// interfaces via CGo. This stub is compiled when the cyclone build tag is
// absent; both constructors return a descriptive error so callers can fall
// back gracefully to the mock implementation.
//
// To build the real CGo implementation:
//
//	apt-get install -y libcyclonedds-dev   # Debian/Ubuntu
//	brew install cyclonedds                # macOS
//	go build -tags cyclone ./...
package cyclone

import (
	"errors"
	"time"

	dds "github.com/SoundMatt/go-DDS"
)

// Options configures a CycloneDDS participant. Defined here so that code
// referencing cyclone.Options compiles regardless of build tags.
type Options struct {
	PollInterval time.Duration
}

const errMsg = "cyclone: not built; rebuild with -tags cyclone and CycloneDDS installed " +
	"(apt install libcyclonedds-dev / brew install cyclonedds)"

// New returns an error when the cyclone build tag is absent.
func New(_ dds.Domain) (dds.Participant, error) {
	return nil, errors.New(errMsg)
}

// NewWithOptions returns an error when the cyclone build tag is absent.
func NewWithOptions(_ dds.Domain, _ Options) (dds.Participant, error) {
	return nil, errors.New(errMsg)
}
