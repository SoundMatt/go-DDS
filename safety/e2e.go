// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package safety provides end-to-end data protection and deterministic
// queuing primitives for safety-oriented DDS deployments.
//
// E2EPublisher prepends an 18-byte protection header to every payload before
// writing. E2ESubscriber strips the header and validates CRC, sequence
// counter, and sample freshness on every received sample.
//
// Wire format (little-endian, 18 bytes followed by original payload):
//
//	Bytes  0–1   DataID (uint16)
//	Bytes  2–3   SourceID (uint16)
//	Bytes  4–7   SequenceCounter (uint32, monotonically increasing per publisher)
//	Bytes  8–15  Timestamp (int64, Unix nanoseconds at time of Write)
//	Bytes 16–17  CRC-16/CCITT-FALSE over bytes 0–15 plus the original payload
//	Bytes 18+    Original payload
//
// The CRC slot is treated as zero when computing the CRC.
package safety

//fusa:req REQ-SAFETY-001
//fusa:req REQ-SAFETY-003
//fusa:req REQ-SAFETY-004
//fusa:req REQ-SAFETY-005
//fusa:req REQ-SAFETY-006
//fusa:req REQ-SAFETY-007
//fusa:req REQ-SAFETY-008
//fusa:req REQ-SAFETY-009
//fusa:req REQ-SAFETY-013
//fusa:req REQ-SEOOC-002

import (
	"encoding/binary"
	"fmt"
	"sync"
	"time"

	dds "github.com/SoundMatt/go-DDS"
)

const headerSize = 18

// E2EConfig configures end-to-end protection parameters shared by a publisher
// and subscriber pair.
type E2EConfig struct {
	// DataID identifies the logical data element (0–65535).
	DataID uint16
	// SourceID identifies the sender (0–65535).
	SourceID uint16
	// MaxAge is the maximum permitted age of a received sample, measured from
	// the timestamp written by the publisher. Zero disables freshness checking.
	MaxAge time.Duration
}

// E2EErrorKind categorises safety check failures reported by E2ESubscriber.
type E2EErrorKind int

const (
	// ErrCRCMismatch means the CRC in the frame header did not match.
	ErrCRCMismatch E2EErrorKind = iota
	// ErrSequenceGap means one or more sequence numbers were skipped.
	ErrSequenceGap
	// ErrStaleSample means the sample's timestamp is older than MaxAge.
	ErrStaleSample
	// ErrHeaderTooShort means the payload is shorter than the 18-byte header.
	ErrHeaderTooShort
)

// E2EError is sent over the Errors channel when a safety check fails.
type E2EError struct {
	Kind    E2EErrorKind
	Counter uint32
	Message string
}

func (e E2EError) Error() string { return e.Message }

// E2EPublisher wraps a dds.Publisher and prepends an E2E protection header to
// every Write payload. It satisfies dds.Publisher.
type E2EPublisher struct {
	pub     dds.Publisher
	cfg     E2EConfig
	counter uint32
	mu      sync.Mutex
}

// NewE2EPublisher wraps pub with end-to-end protection configured by cfg.
func NewE2EPublisher(pub dds.Publisher, cfg E2EConfig) *E2EPublisher {
	return &E2EPublisher{pub: pub, cfg: cfg}
}

// Write prepends an E2E header to payload and writes the frame to the
// underlying publisher.
func (p *E2EPublisher) Write(payload []byte) error {
	p.mu.Lock()
	p.counter++
	counter := p.counter
	p.mu.Unlock()

	return p.pub.Write(makeFrame(p.cfg, counter, payload))
}

// Close closes the underlying publisher.
func (p *E2EPublisher) Close() error { return p.pub.Close() }

// E2ESubscriber wraps a dds.Subscriber, strips the E2E header from each
// received sample, and validates CRC, sequence counter, and freshness. Valid
// samples are forwarded to C(); failures are reported on Errors().
//
// The background pump goroutine starts on construction and exits when Close
// is called or the underlying subscriber channel is closed.
type E2ESubscriber struct {
	sub         dds.Subscriber
	cfg         E2EConfig
	lastCounter uint32
	hasFirst    bool
	ch          chan dds.Sample
	errCh       chan E2EError
	done        chan struct{}
	once        sync.Once
	wg          sync.WaitGroup
}

// NewE2ESubscriber wraps sub with E2E validation using cfg.
func NewE2ESubscriber(sub dds.Subscriber, cfg E2EConfig) *E2ESubscriber {
	s := &E2ESubscriber{
		sub:   sub,
		cfg:   cfg,
		ch:    make(chan dds.Sample, 64),
		errCh: make(chan E2EError, 32),
		done:  make(chan struct{}),
	}
	s.wg.Add(1)
	go s.pump()
	return s
}

// C returns the channel that delivers validated, header-stripped samples.
// It satisfies dds.Subscriber.
func (s *E2ESubscriber) C() <-chan dds.Sample { return s.ch }

// Errors returns a channel that receives E2EError values for every safety
// check failure. The channel is buffered (32); overflows are silently dropped.
func (s *E2ESubscriber) Errors() <-chan E2EError { return s.errCh }

// Close stops the pump goroutine and closes the underlying subscriber.
func (s *E2ESubscriber) Close() error {
	s.once.Do(func() {
		close(s.done)
		s.wg.Wait()
	})
	return s.sub.Close()
}

func (s *E2ESubscriber) pump() {
	defer s.wg.Done()
	defer close(s.ch)
	subCh := s.sub.C()
	for {
		select {
		case raw, ok := <-subCh:
			if !ok {
				return
			}
			s.process(raw)
		case <-s.done:
			return
		}
	}
}

func (s *E2ESubscriber) process(raw dds.Sample) {
	if len(raw.Payload) < headerSize {
		select {
		case s.errCh <- E2EError{
			Kind:    ErrHeaderTooShort,
			Message: fmt.Sprintf("safety: payload %d bytes is shorter than header %d bytes", len(raw.Payload), headerSize),
		}:
		default:
		}
		return
	}

	counter, ts, payload, err := parseFrame(raw.Payload)
	if err != nil {
		select {
		case s.errCh <- E2EError{Kind: ErrCRCMismatch, Counter: counter, Message: err.Error()}:
		default:
		}
		return
	}

	if s.cfg.MaxAge > 0 {
		age := time.Since(time.Unix(0, ts))
		if age > s.cfg.MaxAge {
			select {
			case s.errCh <- E2EError{
				Kind:    ErrStaleSample,
				Counter: counter,
				Message: fmt.Sprintf("safety: sample age %v exceeds MaxAge %v", age.Round(time.Microsecond), s.cfg.MaxAge),
			}:
			default:
			}
			return
		}
	}

	if s.hasFirst && counter != s.lastCounter+1 {
		select {
		case s.errCh <- E2EError{
			Kind:    ErrSequenceGap,
			Counter: counter,
			Message: fmt.Sprintf("safety: sequence gap — received %d, expected %d", counter, s.lastCounter+1),
		}:
		default:
		}
		// Still deliver the sample; the application decides how to handle gaps.
	}
	s.hasFirst = true
	s.lastCounter = counter

	select {
	case s.ch <- dds.Sample{Topic: raw.Topic, Payload: payload, Timestamp: raw.Timestamp}:
	case <-s.done:
	default:
	}
}

// ── Frame encoding ────────────────────────────────────────────────────────────

// makeFrame encodes an E2E-protected wire frame.
func makeFrame(cfg E2EConfig, counter uint32, payload []byte) []byte {
	buf := make([]byte, headerSize+len(payload))
	binary.LittleEndian.PutUint16(buf[0:2], cfg.DataID)
	binary.LittleEndian.PutUint16(buf[2:4], cfg.SourceID)
	binary.LittleEndian.PutUint32(buf[4:8], counter)
	binary.LittleEndian.PutUint64(buf[8:16], uint64(time.Now().UnixNano()))
	copy(buf[18:], payload)

	// CRC covers header fields [0:16] (CRC slot [16:18] is zero) plus payload.
	crcInput := make([]byte, 16+len(payload))
	copy(crcInput, buf[:16])
	copy(crcInput[16:], payload)
	binary.LittleEndian.PutUint16(buf[16:18], crc16(crcInput))
	return buf
}

// parseFrame decodes and validates a frame, returning counter, timestamp,
// stripped payload, and any error.
func parseFrame(data []byte) (counter uint32, ts int64, payload []byte, err error) {
	counter = binary.LittleEndian.Uint32(data[4:8])
	ts = int64(binary.LittleEndian.Uint64(data[8:16]))
	stored := binary.LittleEndian.Uint16(data[16:18])
	payload = data[18:]

	crcInput := make([]byte, 16+len(payload))
	copy(crcInput, data[:16])
	copy(crcInput[16:], payload)
	if computed := crc16(crcInput); computed != stored {
		return counter, ts, nil, fmt.Errorf("safety: CRC mismatch — got 0x%04x, want 0x%04x", computed, stored)
	}
	return counter, ts, payload, nil
}

// crc16 computes a CRC-16/CCITT-FALSE checksum (polynomial 0x1021, init 0xFFFF,
// no reflection).
func crc16(data []byte) uint16 {
	const poly = uint16(0x1021)
	crc := uint16(0xFFFF)
	for _, b := range data {
		crc ^= uint16(b) << 8
		for i := 0; i < 8; i++ {
			if crc&0x8000 != 0 {
				crc = (crc << 1) ^ poly
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}
