// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package record provides topic recording, deterministic replay, and fault
// injection for go-DDS applications.
//
// Recording captures all samples delivered to a set of subscribers and writes
// them to an io.Writer as newline-delimited JSON (JSONL). Each line is a
// RecordedSample object.
//
// Replay reads a JSONL recording and re-publishes the samples through a
// Participant, preserving original timing (or scaling it).
//
// Fault injection wraps any Publisher and injects configurable faults —
// packet loss, delay, payload corruption, and duplication — to stress-test
// DDS consumers without modifying the network or transport.
package record

//fusa:req REQ-RT-001
//fusa:req REQ-REC-001
//fusa:req REQ-REC-002
//fusa:req REQ-REC-003
//fusa:req REQ-REC-010

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	dds "github.com/SoundMatt/go-DDS"
)

// RecordedSample is the JSON-line format written by Recorder.
// Payload is base64-encoded when marshalled to JSON (standard encoding/json
// behaviour for []byte fields).
type RecordedSample struct {
	Topic      string    `json:"topic"`
	Payload    []byte    `json:"payload"`
	Timestamp  time.Time `json:"timestamp"`
	RecordedAt time.Time `json:"recorded_at"`
}

// Recorder subscribes to one or more topics and writes all received samples
// to an io.Writer as JSONL. Each call to AddTopic registers a subscriber whose
// channel will be drained into the recording.
type Recorder struct {
	enc  *json.Encoder
	mu   sync.Mutex
	subs []dds.Subscriber
	done chan struct{}
	once sync.Once
	wg   sync.WaitGroup
}

// NewRecorder returns a Recorder that writes JSONL to w.
func NewRecorder(w io.Writer) *Recorder {
	return &Recorder{
		enc:  json.NewEncoder(w),
		done: make(chan struct{}),
	}
}

// AddTopic registers sub for recording. Must be called before Start.
// Returns r for chaining.
func (r *Recorder) AddTopic(sub dds.Subscriber) *Recorder {
	r.subs = append(r.subs, sub)
	return r
}

// Start launches one drain goroutine per registered subscriber. Returns r for
// chaining.
func (r *Recorder) Start() *Recorder {
	done := r.done
	for _, sub := range r.subs {
		r.wg.Add(1)
		go func(sub dds.Subscriber, done <-chan struct{}) {
			defer r.wg.Done()
			r.drain(sub)
		}(sub, done)
	}
	return r
}

func (r *Recorder) drain(sub dds.Subscriber) {
	for {
		select {
		case s, ok := <-sub.C():
			if !ok {
				return
			}
			rec := RecordedSample{
				Topic:      s.Topic,
				Payload:    s.Payload,
				Timestamp:  s.Timestamp,
				RecordedAt: time.Now(),
			}
			r.mu.Lock()
			_ = r.enc.Encode(rec)
			r.mu.Unlock()
		case <-r.done:
			return
		}
	}
}

// Stop signals all drain goroutines to exit and waits for them to finish.
// Safe to call more than once.
func (r *Recorder) Stop() {
	r.once.Do(func() { close(r.done) })
	r.wg.Wait()
}

// Player reads a JSONL recording written by Recorder and replays all samples
// through freshly created publishers on a Participant.
type Player struct {
	r io.Reader
	p dds.Participant
}

// NewPlayer returns a Player that reads from r and creates publishers on p.
func NewPlayer(r io.Reader, p dds.Participant) *Player {
	return &Player{r: r, p: p}
}

// Play replays the recording at 1× speed (real-time). The first sample is
// published immediately; subsequent samples are delayed to match the original
// inter-sample timing recorded by the Recorder.
func (pl *Player) Play(ctx context.Context) error {
	return pl.playFiltered(ctx, 1.0, nil)
}

// PlayScaled replays with time scaling. speed > 1.0 compresses time; speed
// < 1.0 stretches it. Values ≤ 0 are clamped to 1.0.
func (pl *Player) PlayScaled(ctx context.Context, speed float64) error {
	if speed <= 0 {
		speed = 1.0
	}
	return pl.playFiltered(ctx, speed, nil)
}

// PlayFiltered replays only samples whose topic is in the allow-list. A nil
// or empty allow-list plays all topics.
func (pl *Player) PlayFiltered(ctx context.Context, topics []string) error {
	return pl.playFiltered(ctx, 1.0, topics)
}

func (pl *Player) playFiltered(ctx context.Context, speed float64, topicFilter []string) error {
	filter := make(map[string]struct{}, len(topicFilter))
	for _, t := range topicFilter {
		filter[t] = struct{}{}
	}
	useFilter := len(filter) > 0

	pubs := make(map[string]dds.Publisher)
	defer func() {
		for _, pub := range pubs {
			_ = pub.Close()
		}
	}()

	pubFor := func(topic string) (dds.Publisher, error) {
		if pub, ok := pubs[topic]; ok {
			return pub, nil
		}
		pub, err := pl.p.NewPublisher(topic, dds.DefaultQoS)
		if err != nil {
			return nil, err
		}
		pubs[topic] = pub
		return pub, nil
	}

	dec := json.NewDecoder(pl.r)
	var origin time.Time    // recorded_at of the first sample
	var wallStart time.Time // wall-clock time when playback began

	for dec.More() {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var rec RecordedSample
		if err := dec.Decode(&rec); err != nil {
			return fmt.Errorf("record: decode: %w", err)
		}
		if useFilter {
			if _, ok := filter[rec.Topic]; !ok {
				continue
			}
		}

		// Compute the wall-clock deadline for this sample.
		if origin.IsZero() {
			origin = rec.RecordedAt
			wallStart = time.Now()
		}
		elapsed := rec.RecordedAt.Sub(origin)
		scaled := time.Duration(float64(elapsed) / speed)
		deadline := wallStart.Add(scaled)
		if delay := time.Until(deadline); delay > 0 {
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		pub, err := pubFor(rec.Topic)
		if err != nil {
			return err
		}
		if err := pub.Write(rec.Payload); err != nil {
			return fmt.Errorf("record: replay write %s: %w", rec.Topic, err)
		}
	}
	return nil
}
