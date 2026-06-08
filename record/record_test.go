// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package record_test

//fusa:test REQ-REC-001
//fusa:test REQ-REC-002
//fusa:test REQ-REC-003
//fusa:test REQ-REC-010

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/mock"
	"github.com/SoundMatt/go-DDS/record"
)

// ── helpers ───────────────────────────────────────────────────────────────────

func newPart(t *testing.T) dds.Participant {
	t.Helper()
	p, err := mock.New(0)
	if err != nil {
		t.Fatalf("mock.New: %v", err)
	}
	t.Cleanup(func() { _ = p.Close() })
	return p
}

func uniqueTopic(prefix string) string {
	return fmt.Sprintf("%s/%d", prefix, time.Now().UnixNano())
}

// ── Recorder ──────────────────────────────────────────────────────────────────

func TestRecorder_WritesJSONL(t *testing.T) {
	p := newPart(t)
	topic := uniqueTopic("rec/basic")

	sub, err := p.NewSubscriber(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()
	pub, err := p.NewPublisher(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	var buf bytes.Buffer
	rec := record.NewRecorder(&buf)
	rec.AddTopic(sub).Start()

	if err := pub.Write([]byte("hello")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	rec.Stop()

	if buf.Len() == 0 {
		t.Fatal("expected non-empty JSONL output")
	}
	var rs record.RecordedSample
	if err := json.NewDecoder(&buf).Decode(&rs); err != nil {
		t.Fatalf("decode JSONL: %v", err)
	}
	if string(rs.Payload) != "hello" {
		t.Errorf("payload: got %q, want hello", rs.Payload)
	}
	if rs.Topic != topic {
		t.Errorf("topic: got %q, want %q", rs.Topic, topic)
	}
	if rs.RecordedAt.IsZero() {
		t.Error("recorded_at should not be zero")
	}
}

func TestRecorder_MultipleTopics(t *testing.T) {
	p := newPart(t)
	ta := uniqueTopic("rec/multi/a")
	tb := uniqueTopic("rec/multi/b")

	subA, err := p.NewSubscriber(ta, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	subB, err := p.NewSubscriber(tb, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer subA.Close()
	defer subB.Close()

	pubA, err := p.NewPublisher(ta, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	pubB, err := p.NewPublisher(tb, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pubA.Close()
	defer pubB.Close()

	var buf bytes.Buffer
	rec := record.NewRecorder(&buf)
	rec.AddTopic(subA).AddTopic(subB).Start()

	_ = pubA.Write([]byte("from-a"))
	_ = pubB.Write([]byte("from-b"))
	time.Sleep(60 * time.Millisecond)
	rec.Stop()

	var lines []record.RecordedSample
	dec := json.NewDecoder(&buf)
	for dec.More() {
		var rs record.RecordedSample
		if err := dec.Decode(&rs); err != nil {
			t.Fatalf("decode: %v", err)
		}
		lines = append(lines, rs)
	}
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 lines, got %d", len(lines))
	}
	topics := map[string]bool{}
	for _, l := range lines {
		topics[l.Topic] = true
	}
	if !topics[ta] || !topics[tb] {
		t.Errorf("missing topics: got %v", topics)
	}
}

func TestRecorder_StopIdempotent(t *testing.T) {
	var buf bytes.Buffer
	rec := record.NewRecorder(&buf)
	rec.Start()
	rec.Stop()
	rec.Stop() // must not panic
}

func TestRecorder_StopBeforeStart(t *testing.T) {
	var buf bytes.Buffer
	rec := record.NewRecorder(&buf)
	rec.Stop() // no goroutines started — must not panic
}

// ── Player ────────────────────────────────────────────────────────────────────

// makeJSONL builds a JSONL recording from the given samples.
func makeJSONL(t *testing.T, samples []record.RecordedSample) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	for _, s := range samples {
		if err := enc.Encode(s); err != nil {
			t.Fatalf("encode: %v", err)
		}
	}
	return &buf
}

func TestPlayer_Play(t *testing.T) {
	topic := uniqueTopic("play/basic")
	now := time.Now()
	samples := []record.RecordedSample{
		{Topic: topic, Payload: []byte("s1"), RecordedAt: now, Timestamp: now},
	}
	buf := makeJSONL(t, samples)

	p := newPart(t)
	sub, err := p.NewSubscriber(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()

	pl := record.NewPlayer(buf, p)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := pl.Play(ctx); err != nil {
		t.Fatalf("Play: %v", err)
	}

	select {
	case s := <-sub.C():
		if string(s.Payload) != "s1" {
			t.Errorf("payload: got %q, want s1", s.Payload)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for replayed sample")
	}
}

func TestPlayer_PlayScaled_SpeedTwo(t *testing.T) {
	topic := uniqueTopic("play/scaled")
	base := time.Now()
	samples := []record.RecordedSample{
		{Topic: topic, Payload: []byte("a"), RecordedAt: base},
		{Topic: topic, Payload: []byte("b"), RecordedAt: base.Add(200 * time.Millisecond)},
	}
	buf := makeJSONL(t, samples)

	p := newPart(t)
	sub, err := p.NewSubscriber(topic, dds.DefaultQoS, dds.WithChannelDepth(10))
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()

	pl := record.NewPlayer(buf, p)
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := pl.PlayScaled(ctx, 2.0); err != nil {
		t.Fatalf("PlayScaled: %v", err)
	}
	elapsed := time.Since(start)

	// At 2× speed, the 200 ms gap should compress to ~100 ms.
	if elapsed > 160*time.Millisecond {
		t.Errorf("PlayScaled 2×: elapsed %v, expected < 160 ms", elapsed)
	}
}

func TestPlayer_PlayFiltered(t *testing.T) {
	topicA := uniqueTopic("play/filter/a")
	topicB := uniqueTopic("play/filter/b")
	now := time.Now()
	samples := []record.RecordedSample{
		{Topic: topicA, Payload: []byte("keep"), RecordedAt: now},
		{Topic: topicB, Payload: []byte("skip"), RecordedAt: now},
	}
	buf := makeJSONL(t, samples)

	p := newPart(t)
	subA, err := p.NewSubscriber(topicA, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	subB, err := p.NewSubscriber(topicB, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer subA.Close()
	defer subB.Close()

	pl := record.NewPlayer(buf, p)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := pl.PlayFiltered(ctx, []string{topicA}); err != nil {
		t.Fatalf("PlayFiltered: %v", err)
	}

	// topicA should deliver
	select {
	case s := <-subA.C():
		if string(s.Payload) != "keep" {
			t.Errorf("payload: got %q, want keep", s.Payload)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout for topicA")
	}

	// topicB should not deliver within a short window
	select {
	case <-subB.C():
		t.Error("topicB should have been filtered out")
	case <-time.After(60 * time.Millisecond):
		// correct: nothing received
	}
}

func TestPlayer_EmptyRecording(t *testing.T) {
	p := newPart(t)
	pl := record.NewPlayer(strings.NewReader(""), p)
	ctx := context.Background()
	if err := pl.Play(ctx); err != nil {
		t.Fatalf("Play on empty recording: %v", err)
	}
}

func TestPlayer_DecodeError(t *testing.T) {
	p := newPart(t)
	pl := record.NewPlayer(strings.NewReader("{invalid-json}\n"), p)
	ctx := context.Background()
	err := pl.Play(ctx)
	if err == nil {
		t.Fatal("expected decode error")
	}
	if !strings.Contains(err.Error(), "record: decode") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPlayer_ContextCancel(t *testing.T) {
	topic := uniqueTopic("play/cancel")
	base := time.Now()
	// Three samples with 500 ms gaps — context will cancel after first.
	samples := []record.RecordedSample{
		{Topic: topic, Payload: []byte("1"), RecordedAt: base},
		{Topic: topic, Payload: []byte("2"), RecordedAt: base.Add(500 * time.Millisecond)},
		{Topic: topic, Payload: []byte("3"), RecordedAt: base.Add(1000 * time.Millisecond)},
	}
	buf := makeJSONL(t, samples)

	p := newPart(t)
	pl := record.NewPlayer(buf, p)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	err := pl.Play(ctx)
	if err == nil {
		t.Fatal("expected context error")
	}
}

func TestPlayer_PlayScaled_ZeroSpeedClamped(t *testing.T) {
	topic := uniqueTopic("play/clamp")
	now := time.Now()
	samples := []record.RecordedSample{
		{Topic: topic, Payload: []byte("x"), RecordedAt: now},
	}
	buf := makeJSONL(t, samples)
	p := newPart(t)
	sub, err := p.NewSubscriber(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()

	pl := record.NewPlayer(buf, p)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	// speed <= 0 should clamp to 1.0, not divide-by-zero
	if err := pl.PlayScaled(ctx, 0); err != nil {
		t.Fatalf("PlayScaled(0): %v", err)
	}
}

// TestRecorder_Drain_ClosedSubscriber covers the drain goroutine's !ok branch,
// which fires when the underlying subscriber's channel is closed externally.
func TestRecorder_Drain_ClosedSubscriber(t *testing.T) {
	p := newPart(t)
	topic := uniqueTopic("rec/drain-close")

	sub, err := p.NewSubscriber(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}

	var buf bytes.Buffer
	rec := record.NewRecorder(&buf).AddTopic(sub).Start()

	// Closing the subscriber closes its channel; the drain goroutine sees !ok.
	sub.Close()
	time.Sleep(30 * time.Millisecond)

	// Stop must complete promptly — the drain goroutine has already exited.
	rec.Stop()
}

// TestPlayer_PlayFiltered_CancelledContext covers the ctx.Err() early-exit
// branch inside playFiltered when the context is already cancelled before the
// loop body runs.
func TestPlayer_PlayFiltered_CancelledContext(t *testing.T) {
	topic := uniqueTopic("play/cancelled")
	now := time.Now()
	samples := []record.RecordedSample{
		{Topic: topic, Payload: []byte("a"), RecordedAt: now},
		{Topic: topic, Payload: []byte("b"), RecordedAt: now.Add(100 * time.Millisecond)},
	}
	buf := makeJSONL(t, samples)

	p := newPart(t)
	pl := record.NewPlayer(buf, p)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before Play so ctx.Err() is non-nil on first loop iteration

	err := pl.Play(ctx)
	if err == nil {
		t.Fatal("expected context error from already-cancelled context")
	}
}

// TestPlayer_Play_WriteError covers the pub.Write error path in playFiltered,
// triggered when the participant is closed before Play can publish samples.
func TestPlayer_Play_WriteError(t *testing.T) {
	p := newPart(t)
	topic := uniqueTopic("play/writeerr")
	now := time.Now()
	samples := []record.RecordedSample{
		{Topic: topic, Payload: []byte("a"), RecordedAt: now},
	}
	buf := makeJSONL(t, samples)
	pl := record.NewPlayer(buf, p)

	// Close the participant so NewPublisher succeeds but Write fails.
	p.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := pl.Play(ctx)
	if err == nil {
		t.Fatal("expected error when participant is closed during play")
	}
}

// ── RecordedSample JSON round-trip ────────────────────────────────────────────

func TestRecordedSample_JSONRoundTrip(t *testing.T) {
	want := record.RecordedSample{
		Topic:      "test/topic",
		Payload:    []byte{0x01, 0x02, 0x03},
		Timestamp:  time.Now().Round(time.Millisecond),
		RecordedAt: time.Now().Round(time.Millisecond),
	}
	data, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got record.RecordedSample
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if string(got.Payload) != string(want.Payload) {
		t.Errorf("Payload: got %v, want %v", got.Payload, want.Payload)
	}
	if got.Topic != want.Topic {
		t.Errorf("Topic: got %q, want %q", got.Topic, want.Topic)
	}
}
