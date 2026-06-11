// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package scenario provides a declarative scenario DSL for DDS integration
// tests. A scenario is a sequence of Steps executed against a Participant.
// Steps include publishing samples, expecting samples, waiting for quiet
// periods, and running custom assertions.
//
// Example:
//
//	err := scenario.Run(ctx, p,
//	    scenario.Publish("sensor/temp", []byte("42"), dds.DefaultQoS),
//	    scenario.Expect("sensor/temp", dds.DefaultQoS, 100*time.Millisecond, nil),
//	)
package scenario

import (
	"context"
	"fmt"
	"time"

	dds "github.com/SoundMatt/go-DDS"
)

// Step is a single action in a scenario.
type Step interface {
	run(ctx context.Context, p dds.Participant) error
}

// Run executes steps in order against p. Returns the first error encountered.
// Each step's context carries the outer ctx so cancellation propagates.
func Run(ctx context.Context, p dds.Participant, steps ...Step) error {
	for i, s := range steps {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("scenario step %d: context cancelled: %w", i, err)
		}
		if err := s.run(ctx, p); err != nil {
			return fmt.Errorf("scenario step %d (%T): %w", i, s, err)
		}
	}
	return nil
}

// ── Step types ────────────────────────────────────────────────────────────────

// publishStep publishes a single sample to a topic.
type publishStep struct {
	topic   string
	payload []byte
	qos     dds.QoS
}

// Publish returns a Step that creates a publisher, writes payload, and closes it.
func Publish(topic string, payload []byte, qos dds.QoS) Step {
	return &publishStep{topic: topic, payload: payload, qos: qos}
}

func (s *publishStep) run(ctx context.Context, p dds.Participant) error {
	pub, err := p.NewPublisher(s.topic, s.qos)
	if err != nil {
		return fmt.Errorf("publish %q: new publisher: %w", s.topic, err)
	}
	defer pub.Close() //nolint:errcheck // step-scoped publisher; close errors are not actionable here
	if err := pub.WriteCtx(ctx, s.payload); err != nil {
		return fmt.Errorf("publish %q: write: %w", s.topic, err)
	}
	return nil
}

// expectStep waits for a sample on a topic, optionally matching a predicate.
type expectStep struct {
	topic   string
	qos     dds.QoS
	timeout time.Duration
	match   func(dds.Sample) bool
}

// Expect returns a Step that subscribes to topic and waits up to timeout for a
// sample. If match is non-nil, the step fails if the received sample does not
// satisfy the predicate.
func Expect(topic string, qos dds.QoS, timeout time.Duration, match func(dds.Sample) bool) Step {
	return &expectStep{topic: topic, qos: qos, timeout: timeout, match: match}
}

func (s *expectStep) run(ctx context.Context, p dds.Participant) error {
	sub, err := p.NewSubscriber(s.topic, s.qos)
	if err != nil {
		return fmt.Errorf("expect %q: new subscriber: %w", s.topic, err)
	}
	defer sub.Close() //nolint:errcheck // step-scoped subscriber; close errors are not actionable here

	deadline := time.After(s.timeout)
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("expect %q: %w", s.topic, ctx.Err())
		case <-deadline:
			return fmt.Errorf("expect %q: timeout after %v", s.topic, s.timeout)
		case sample, ok := <-sub.C():
			if !ok {
				return fmt.Errorf("expect %q: subscriber closed", s.topic)
			}
			if s.match == nil || s.match(sample) {
				return nil
			}
		}
	}
}

// waitStep sleeps for a duration.
type waitStep struct{ d time.Duration }

// Wait returns a Step that sleeps for d, respecting context cancellation.
func Wait(d time.Duration) Step { return &waitStep{d: d} }

func (s *waitStep) run(ctx context.Context, _ dds.Participant) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(s.d):
		return nil
	}
}

// assertStep runs an arbitrary assertion function.
type assertStep struct {
	desc string
	fn   func(ctx context.Context, p dds.Participant) error
}

// Assert returns a Step that calls fn. desc is used in error messages.
func Assert(desc string, fn func(ctx context.Context, p dds.Participant) error) Step {
	return &assertStep{desc: desc, fn: fn}
}

func (s *assertStep) run(ctx context.Context, p dds.Participant) error {
	if err := s.fn(ctx, p); err != nil {
		return fmt.Errorf("assert %q: %w", s.desc, err)
	}
	return nil
}

// expectNoneStep verifies no sample arrives within timeout.
type expectNoneStep struct {
	topic   string
	qos     dds.QoS
	timeout time.Duration
}

// ExpectNone returns a Step that fails if any sample arrives on topic within timeout.
func ExpectNone(topic string, qos dds.QoS, timeout time.Duration) Step {
	return &expectNoneStep{topic: topic, qos: qos, timeout: timeout}
}

func (s *expectNoneStep) run(ctx context.Context, p dds.Participant) error {
	sub, err := p.NewSubscriber(s.topic, s.qos)
	if err != nil {
		return fmt.Errorf("expect-none %q: new subscriber: %w", s.topic, err)
	}
	defer sub.Close() //nolint:errcheck // step-scoped subscriber; close errors are not actionable here

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(s.timeout):
		return nil
	case sample := <-sub.C():
		return fmt.Errorf("expect-none %q: unexpected sample payload=%q", s.topic, sample.Payload)
	}
}
