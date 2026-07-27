// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package services_test

//fusa:test REQ-SVC-001
//fusa:test REQ-SVC-002
//fusa:test REQ-SVC-003

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/mock"
	"github.com/SoundMatt/go-DDS/observability/monitor"
	"github.com/SoundMatt/go-DDS/observability/record"
	"github.com/SoundMatt/go-DDS/observability/services"
)

// failAfterNParticipant wraps a real participant and fails NewSubscriber after
// the first N successes, covering the cleanup-loop branch in RecorderService.Start.
type failAfterNParticipant struct {
	dds.Participant
	n    int64
	done atomic.Int64
}

func (p *failAfterNParticipant) NewSubscriber(topic string, qos dds.QoS, opts ...dds.SubscriberOption) (dds.Subscriber, error) {
	if p.done.Add(1) > p.n {
		return nil, errors.New("forced subscriber failure")
	}
	return p.Participant.NewSubscriber(topic, qos, opts...)
}

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

// failOnSecondSubPart wraps a participant and returns an error on the second
// NewSubscriber call. This exercises the cleanup loop in RecorderService.Start
// when a later subscriber creation fails after earlier ones succeeded.
type failOnSecondSubPart struct {
	dds.Participant
	calls int
}

func (f *failOnSecondSubPart) NewSubscriber(topic string, qos dds.QoS, opts ...dds.SubscriberOption) (dds.Subscriber, error) {
	f.calls++
	if f.calls >= 2 {
		return nil, dds.ErrClosed
	}
	return f.Participant.NewSubscriber(topic, qos, opts...)
}

// TestRecorderService_Start_CleanupOnError covers the _ = existing.Close()
// cleanup loop in Start when NewSubscriber fails on a subsequent topic.
func TestRecorderService_Start_CleanupOnError(t *testing.T) {
	realPart := newPart(t)
	p := &failOnSecondSubPart{Participant: realPart}
	var buf bytes.Buffer
	svc := services.NewRecorderService(p, services.RecorderOptions{
		Topics: []string{uniqueTopic("svc/cleanup/a"), uniqueTopic("svc/cleanup/b")},
		Output: &buf,
	})
	err := svc.Start()
	if err == nil {
		t.Fatal("expected error when second subscriber creation fails")
	}
}

// ── RecorderService ───────────────────────────────────────────────────────────

func TestRecorderService_StartStop(t *testing.T) {
	p := newPart(t)
	var buf bytes.Buffer
	svc := services.NewRecorderService(p, services.RecorderOptions{
		Topics: []string{uniqueTopic("svc/rec")},
		Output: &buf,
	})
	if err := svc.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	svc.Stop()
}

func TestRecorderService_CapturesSamples(t *testing.T) {
	p := newPart(t)
	topic := uniqueTopic("svc/rec/capture")

	var buf bytes.Buffer
	svc := services.NewRecorderService(p, services.RecorderOptions{
		Topics: []string{topic},
		Output: &buf,
	})
	if err := svc.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	pub, err := p.NewPublisher(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()
	_ = pub.Write([]byte("captured"))
	time.Sleep(50 * time.Millisecond)

	svc.Stop()

	if buf.Len() == 0 {
		t.Fatal("expected non-empty recording after Stop")
	}
	var rs record.RecordedSample
	if err := json.NewDecoder(&buf).Decode(&rs); err != nil {
		t.Fatalf("decode JSONL: %v", err)
	}
	if string(rs.Payload) != "captured" {
		t.Errorf("payload: got %q, want captured", rs.Payload)
	}
}

func TestRecorderService_StartIdempotent(t *testing.T) {
	p := newPart(t)
	var buf bytes.Buffer
	svc := services.NewRecorderService(p, services.RecorderOptions{
		Output: &buf,
	})
	_ = svc.Start()
	if err := svc.Start(); err != nil {
		t.Errorf("second Start should be no-op: %v", err)
	}
	svc.Stop()
}

func TestRecorderService_StopIdempotent(t *testing.T) {
	p := newPart(t)
	var buf bytes.Buffer
	svc := services.NewRecorderService(p, services.RecorderOptions{Output: &buf})
	_ = svc.Start()
	svc.Stop()
	svc.Stop() // must not panic
}

func TestRecorderService_NoTopics_NoError(t *testing.T) {
	p := newPart(t)
	var buf bytes.Buffer
	svc := services.NewRecorderService(p, services.RecorderOptions{Output: &buf})
	if err := svc.Start(); err != nil {
		t.Fatalf("Start with no topics: %v", err)
	}
	svc.Stop()
}

func TestRecorderService_ClosedParticipant_StartError(t *testing.T) {
	p := newPart(t)
	p.Close()
	var buf bytes.Buffer
	svc := services.NewRecorderService(p, services.RecorderOptions{
		Topics: []string{"some/topic"},
		Output: &buf,
	})
	if err := svc.Start(); err == nil {
		t.Error("expected error starting recorder on closed participant")
		svc.Stop()
	}
}

// ── ReplayService ─────────────────────────────────────────────────────────────

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

func TestReplayService_Plays(t *testing.T) {
	p := newPart(t)
	topic := uniqueTopic("svc/replay")
	now := time.Now()
	samples := []record.RecordedSample{
		{Topic: topic, Payload: []byte("replayed"), RecordedAt: now},
	}
	buf := makeJSONL(t, samples)

	sub, err := p.NewSubscriber(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()

	svc := services.NewReplayService(p, services.ReplayOptions{Input: buf})
	ctx := context.Background()
	if err := svc.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	select {
	case s := <-sub.C():
		if string(s.Payload) != "replayed" {
			t.Errorf("payload: got %q, want replayed", s.Payload)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for replayed sample")
	}

	svc.Stop()
	if svc.Err() != nil {
		t.Errorf("Err after normal playback: %v", svc.Err())
	}
}

func TestReplayService_Done_ClosedAfterStop(t *testing.T) {
	p := newPart(t)
	svc := services.NewReplayService(p, services.ReplayOptions{
		Input: strings.NewReader(""),
	})
	ctx, cancel := context.WithCancel(context.Background())
	_ = svc.Start(ctx)
	cancel()
	svc.Stop()

	select {
	case <-svc.Done():
		// correct
	case <-time.After(time.Second):
		t.Fatal("Done() not closed after Stop")
	}
}

func TestReplayService_StartIdempotent(t *testing.T) {
	p := newPart(t)
	svc := services.NewReplayService(p, services.ReplayOptions{
		Input: strings.NewReader(""),
	})
	ctx := context.Background()
	_ = svc.Start(ctx)
	if err := svc.Start(ctx); err != nil {
		t.Errorf("second Start should be no-op: %v", err)
	}
	svc.Stop()
}

func TestReplayService_Loop_SeekableInput(t *testing.T) {
	p := newPart(t)
	topic := uniqueTopic("svc/loop")
	now := time.Now()
	samples := []record.RecordedSample{
		{Topic: topic, Payload: []byte("looped"), RecordedAt: now},
	}
	buf := makeJSONL(t, samples)

	sub, err := p.NewSubscriber(topic, dds.DefaultQoS, dds.WithChannelDepth(8))
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()

	svc := services.NewReplayService(p, services.ReplayOptions{
		Input: bytes.NewReader(buf.Bytes()), // *bytes.Reader implements io.Seeker
		Loop:  true,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	_ = svc.Start(ctx)

	// Collect at least 2 deliveries to confirm looping.
	count := 0
	deadline := time.After(time.Second)
	for count < 2 {
		select {
		case <-sub.C():
			count++
		case <-deadline:
			t.Fatalf("timeout: only got %d deliveries, want ≥ 2 (looping)", count)
			return
		}
	}
	svc.Stop()
}

func TestReplayService_FilteredTopics(t *testing.T) {
	p := newPart(t)
	topicA := uniqueTopic("svc/filter/a")
	topicB := uniqueTopic("svc/filter/b")
	now := time.Now()
	samples := []record.RecordedSample{
		{Topic: topicA, Payload: []byte("keep"), RecordedAt: now},
		{Topic: topicB, Payload: []byte("skip"), RecordedAt: now},
	}
	buf := makeJSONL(t, samples)

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

	svc := services.NewReplayService(p, services.ReplayOptions{
		Input:  buf,
		Topics: []string{topicA},
	})
	ctx := context.Background()
	_ = svc.Start(ctx)

	// Wait for the replay goroutine to finish (non-looping, ends when recording ends).
	select {
	case <-svc.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("replay did not complete within 2s")
	}

	select {
	case s := <-subA.C():
		if string(s.Payload) != "keep" {
			t.Errorf("topicA payload: got %q, want keep", s.Payload)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout for topicA")
	}

	select {
	case <-subB.C():
		t.Error("topicB should have been filtered out")
	case <-time.After(60 * time.Millisecond):
	}
}

// ── MonitorService ────────────────────────────────────────────────────────────

func TestMonitorService_New(t *testing.T) {
	p := newPart(t)
	svc, err := services.NewMonitorService(p, monitor.Options{Addr: "127.0.0.1:0"})
	if err != nil {
		t.Fatalf("NewMonitorService: %v", err)
	}
	defer svc.Close()
}

func TestMonitorService_Addr_NonEmpty(t *testing.T) {
	p := newPart(t)
	svc, err := services.NewMonitorService(p, monitor.Options{Addr: "127.0.0.1:0"})
	if err != nil {
		t.Fatalf("NewMonitorService: %v", err)
	}
	defer svc.Close()
	if svc.Addr() == "" {
		t.Error("Addr() must not be empty")
	}
}

func TestMonitorService_Mon_NotNil(t *testing.T) {
	p := newPart(t)
	svc, err := services.NewMonitorService(p, monitor.Options{Addr: "127.0.0.1:0"})
	if err != nil {
		t.Fatalf("NewMonitorService: %v", err)
	}
	defer svc.Close()
	if svc.Mon() == nil {
		t.Error("Mon() must not be nil")
	}
}

func TestMonitorService_Close(t *testing.T) {
	p := newPart(t)
	svc, err := services.NewMonitorService(p, monitor.Options{Addr: "127.0.0.1:0"})
	if err != nil {
		t.Fatalf("NewMonitorService: %v", err)
	}
	if err := svc.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

func TestMonitorService_New_ListenError(t *testing.T) {
	p := newPart(t)
	ignoredRet, err := services.NewMonitorService(p, monitor.Options{Addr: "127.0.0.1:99999"})
	_ = ignoredRet
	if err == nil {
		t.Fatal("expected error for invalid port")
	}
}

// TestReplayService_ScaledSpeed exercises the PlayScaled branch in run()
// (Speed != 0 && Speed != 1.0, no Topics filter).
func TestReplayService_ScaledSpeed(t *testing.T) {
	p := newPart(t)
	topic := uniqueTopic("svc/scaled")
	now := time.Now()
	samples := []record.RecordedSample{
		{Topic: topic, Payload: []byte("scaled"), RecordedAt: now},
	}
	buf := makeJSONL(t, samples)

	sub, err := p.NewSubscriber(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()

	svc := services.NewReplayService(p, services.ReplayOptions{
		Input: buf,
		Speed: 2.0, // != 0 && != 1.0 → PlayScaled branch
	})
	ctx := context.Background()
	if err := svc.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	select {
	case s := <-sub.C():
		if string(s.Payload) != "scaled" {
			t.Errorf("payload: got %q, want scaled", s.Payload)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for scaled-speed sample")
	}
	svc.Stop()
}

// failSeeker wraps an io.Reader with a Seek method that always returns an error.
// Used to test the seek-error path in ReplayService.
type failSeeker struct {
	io.Reader
}

func (f *failSeeker) Seek(_ int64, _ int) (int64, error) {
	return 0, fmt.Errorf("seek failed deliberately")
}

// TestReplayService_SeekError exercises the Seek error branch in run():
// Loop:true with a seekable input whose Seek call returns an error causes the
// service to exit with a non-nil Err().
func TestReplayService_SeekError(t *testing.T) {
	p := newPart(t)
	topic := uniqueTopic("svc/seek/err")
	now := time.Now()
	samples := []record.RecordedSample{
		{Topic: topic, Payload: []byte("x"), RecordedAt: now},
	}
	buf := makeJSONL(t, samples)

	svc := services.NewReplayService(p, services.ReplayOptions{
		Input: &failSeeker{Reader: buf},
		Loop:  true,
	})
	ctx := context.Background()
	if err := svc.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	select {
	case <-svc.Done():
		if svc.Err() == nil {
			t.Error("expected Err() to be set after seek failure")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: service did not exit after seek error")
	}
	svc.Stop()
}

// TestReplayService_Loop_NonSeekable verifies that a non-seekable input with
// Loop:true runs once and exits cleanly (cannot seek back to start).
func TestReplayService_Loop_NonSeekable(t *testing.T) {
	p := newPart(t)
	topic := uniqueTopic("svc/loop/nonseek")
	now := time.Now()
	samples := []record.RecordedSample{
		{Topic: topic, Payload: []byte("once"), RecordedAt: now},
	}
	buf := makeJSONL(t, samples)
	// strings.Reader does not implement io.Seeker in the services sense —
	// we wrap it in a non-seekable reader.
	var nonSeekable io.Reader = io.NopCloser(buf)

	svc := services.NewReplayService(p, services.ReplayOptions{
		Input: nonSeekable,
		Loop:  true,
	})
	ctx := context.Background()
	_ = svc.Start(ctx)

	select {
	case <-svc.Done():
		// correct: non-seekable loop exits after one pass
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: non-seekable loop should exit after one pass")
	}
	svc.Stop()
}

// TestRecorderService_Start_PartialSubscriberFailure covers the cleanup loop in
// RecorderService.Start: the first NewSubscriber succeeds, the second fails, so
// the already-created subscriber is closed before returning the error.
func TestRecorderService_Start_PartialSubscriberFailure(t *testing.T) {
	realPart, err := mock.New(0)
	if err != nil {
		t.Fatalf("mock.New: %v", err)
	}
	defer realPart.Close()

	// Allow exactly 1 successful NewSubscriber, then fail.
	stub := &failAfterNParticipant{Participant: realPart, n: 1}

	var buf bytes.Buffer
	svc := services.NewRecorderService(stub, services.RecorderOptions{
		Topics: []string{uniqueTopic("svc/partial-a"), uniqueTopic("svc/partial-b")},
		Output: &buf,
	})
	if startErr := svc.Start(); startErr == nil {
		t.Fatal("expected error when second NewSubscriber fails")
	}
}
