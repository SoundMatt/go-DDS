// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package services provides long-running operational service wrappers for
// go-DDS (Milestone 10 — Enterprise Services: Service Framework).
//
// Three services are provided:
//
//   - [RecorderService] — continuously records DDS topic samples to a writer.
//   - [ReplayService] — replays a recorded JSONL stream back into DDS topics.
//   - [MonitorService] — wraps [monitor.Monitor] as a managed service.
//
// Each service follows the same lifecycle:
//
//	svc := services.NewXxx(p, opts)
//	if err := svc.Start(...); err != nil { ... }
//	// ... application runs ...
//	svc.Stop()
package services

import (
	"context"
	"io"
	"sync"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/monitor"
	"github.com/SoundMatt/go-DDS/record"
)

// ── RecorderService ───────────────────────────────────────────────────────────

// RecorderOptions configures a [RecorderService].
type RecorderOptions struct {
	// Topics is the list of DDS topic names to record. Subscribing begins when
	// Start is called. Empty Topics results in a recorder that captures nothing.
	Topics []string
	// Output is the destination writer for the JSONL recording stream.
	// The caller retains ownership; RecorderService does not close it.
	Output io.Writer
}

// RecorderService continuously records DDS samples from Topics to Output in
// JSONL format. It wraps [record.Recorder] and provides a managed Start/Stop
// lifecycle.
type RecorderService struct {
	p    dds.Participant
	opts RecorderOptions
	rec  *record.Recorder
	mu   sync.Mutex
	subs []dds.Subscriber
}

// NewRecorderService creates a RecorderService for participant p.
func NewRecorderService(p dds.Participant, opts RecorderOptions) *RecorderService {
	return &RecorderService{p: p, opts: opts}
}

// Start subscribes to all configured topics and begins recording. It is safe
// to call Start only once; subsequent calls return nil without effect.
func (s *RecorderService) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.rec != nil {
		return nil
	}
	rec := record.NewRecorder(s.opts.Output)
	for _, topic := range s.opts.Topics {
		sub, err := s.p.NewSubscriber(topic, dds.DefaultQoS)
		if err != nil {
			for _, existing := range s.subs {
				_ = existing.Close()
			}
			s.subs = nil
			return err
		}
		s.subs = append(s.subs, sub)
		rec.AddTopic(sub)
	}
	rec.Start()
	s.rec = rec
	return nil
}

// Stop halts recording and closes all subscribers.
// It is safe to call Stop multiple times.
func (s *RecorderService) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.rec != nil {
		s.rec.Stop()
		s.rec = nil
	}
	for _, sub := range s.subs {
		_ = sub.Close()
	}
	s.subs = nil
}

// ── ReplayService ─────────────────────────────────────────────────────────────

// ReplayOptions configures a [ReplayService].
type ReplayOptions struct {
	// Input is the JSONL recording stream to replay. The caller retains
	// ownership; ReplayService does not close it.
	Input io.Reader
	// Speed is the playback speed multiplier (e.g. 2.0 = twice real-time).
	// 0 or negative values are clamped to 1.0 (real-time).
	Speed float64
	// Topics restricts replay to these topics. nil or empty = replay all.
	Topics []string
	// Loop, when true, restarts replay from the beginning when the recording ends.
	// The caller must pass a cancellable context to Stop replay via context cancellation.
	// Note: Loop requires Input to implement [io.Seeker].
	Loop bool
}

// ReplayService replays a JSONL recording into DDS topics via a managed
// lifecycle. It wraps [record.Player].
type ReplayService struct {
	p      dds.Participant
	opts   ReplayOptions
	cancel context.CancelFunc
	done   chan struct{}
	err    error
	once   sync.Once
}

// NewReplayService creates a ReplayService for participant p.
func NewReplayService(p dds.Participant, opts ReplayOptions) *ReplayService {
	return &ReplayService{p: p, opts: opts, done: make(chan struct{})}
}

// Start begins replay in a background goroutine. The context ctx controls
// the replay; cancel it (or call Stop) to halt playback. Start may only be
// called once; subsequent calls return nil immediately.
func (s *ReplayService) Start(ctx context.Context) error {
	started := false
	s.once.Do(func() {
		started = true
		replayCtx, cancel := context.WithCancel(ctx)
		s.cancel = cancel
		go s.run(replayCtx)
	})
	_ = started
	return nil
}

func (s *ReplayService) run(ctx context.Context) {
	defer close(s.done)
	for {
		pl := record.NewPlayer(s.opts.Input, s.p)
		var err error
		if len(s.opts.Topics) > 0 {
			err = pl.PlayFiltered(ctx, s.opts.Topics)
		} else if s.opts.Speed != 0 && s.opts.Speed != 1.0 {
			err = pl.PlayScaled(ctx, s.opts.Speed)
		} else {
			err = pl.Play(ctx)
		}
		if err != nil || !s.opts.Loop {
			s.err = err
			return
		}
		// Loop: seek back to start if Input is a Seeker.
		if seeker, ok := s.opts.Input.(io.Seeker); ok {
			if _, err := seeker.Seek(0, io.SeekStart); err != nil {
				s.err = err
				return
			}
		} else {
			// Non-seekable input: cannot loop; treat as done.
			return
		}
		if ctx.Err() != nil {
			return
		}
	}
}

// Stop halts the replay. It blocks until the replay goroutine exits.
// Safe to call multiple times.
func (s *ReplayService) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	<-s.done
}

// Err returns the error from the last replay run, if any.
// Only meaningful after Stop has returned.
func (s *ReplayService) Err() error { return s.err }

// Done returns a channel that is closed when the replay goroutine exits
// (either due to reaching the end of the recording, context cancellation,
// or Stop).
func (s *ReplayService) Done() <-chan struct{} { return s.done }

// ── MonitorService ────────────────────────────────────────────────────────────

// MonitorService wraps [monitor.Monitor] as a managed service.
type MonitorService struct {
	mon *monitor.Monitor
}

// NewMonitorService creates and starts a MonitorService for participant p.
func NewMonitorService(p dds.Participant, opts monitor.Options) (*MonitorService, error) {
	mon, err := monitor.New(p, opts)
	if err != nil {
		return nil, err
	}
	return &MonitorService{mon: mon}, nil
}

// Addr returns the TCP address the monitor HTTP server is listening on.
func (s *MonitorService) Addr() string { return s.mon.Addr() }

// Mon returns the underlying [monitor.Monitor], allowing direct use of its
// Publish method for injecting dashboard events.
func (s *MonitorService) Mon() *monitor.Monitor { return s.mon }

// Close shuts down the monitor HTTP server.
func (s *MonitorService) Close() error { return s.mon.Close() }
