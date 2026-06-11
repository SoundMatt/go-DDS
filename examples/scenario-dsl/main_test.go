// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package scenariodsl_test demonstrates the testutil/scenario declarative DSL.
//
// The scenario DSL lets you write integration tests as a sequence of named
// steps — Publish, Expect, ExpectNone, Wait, Assert — without boilerplate
// publisher/subscriber lifecycle code.
//
// Run:
//
//	go test -v ./examples/scenario-dsl/
package scenariodsl_test

import (
	"context"
	"strings"
	"testing"
	"time"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/mock"
	"github.com/SoundMatt/go-DDS/testutil/scenario"
)

func TestExample_sensor_publish_expect(t *testing.T) {
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		t.Fatalf("mock.New: %v", err)
	}
	defer func() { _ = p.Close() }()

	// ReliableQoS (TransientLocal) keeps the last sample so the Expect step,
	// which creates its subscriber after Publish, still receives it.
	err = scenario.Run(context.Background(), p,
		scenario.Publish("sensors/temperature",
			[]byte(`{"value":22.5}`),
			dds.ReliableQoS,
		),
		scenario.Expect("sensors/temperature", dds.ReliableQoS,
			200*time.Millisecond,
			func(s dds.Sample) bool {
				return strings.Contains(string(s.Payload), "22.5")
			},
		),
		scenario.Assert("domain check", func(_ context.Context, part dds.Participant) error {
			t.Logf("running on domain %d", part.Domain())
			return nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
}

func TestExample_expect_none(t *testing.T) {
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		t.Fatalf("mock.New: %v", err)
	}
	defer func() { _ = p.Close() }()

	// Verify that nothing is published on a topic during a 50 ms window.
	err = scenario.Run(context.Background(), p,
		scenario.ExpectNone("silent/topic", dds.DefaultQoS, 50*time.Millisecond),
	)
	if err != nil {
		t.Fatal(err)
	}
}

func TestExample_wait_step(t *testing.T) {
	p, err := mock.New(dds.Domain(0))
	if err != nil {
		t.Fatalf("mock.New: %v", err)
	}
	defer func() { _ = p.Close() }()

	start := time.Now()
	err = scenario.Run(context.Background(), p,
		scenario.Wait(30*time.Millisecond),
	)
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(start) < 30*time.Millisecond {
		t.Fatal("Wait returned too early")
	}
}
