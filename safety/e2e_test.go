// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package safety_test

//fusa:test REQ-SAFETY-001
//fusa:test REQ-SAFETY-004

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"
	"time"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/mock"
	"github.com/SoundMatt/go-DDS/safety"
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

const e2eHeaderSize = 18

// ── CRC sanity ────────────────────────────────────────────────────────────────

func TestE2E_RoundTrip_ValidFrame(t *testing.T) {
	p := newPart(t)
	topic := uniqueTopic("e2e/roundtrip")
	cfg := safety.E2EConfig{DataID: 1, SourceID: 2}

	rawPub, err := p.NewPublisher(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	rawSub, err := p.NewSubscriber(topic, dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer rawSub.Close()

	pub := safety.NewE2EPublisher(rawPub, cfg)
	defer func() { _ = pub.Close() }()
	sub := safety.NewE2ESubscriber(rawSub, cfg)
	defer func() { _ = sub.Close() }()

	want := []byte("protected payload")
	if err := pub.Write(want); err != nil {
		t.Fatalf("Write: %v", err)
	}
	select {
	case s := <-sub.C():
		if !bytes.Equal(s.Payload, want) {
			t.Errorf("payload: got %q, want %q", s.Payload, want)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout: no sample delivered")
	}
	select {
	case e := <-sub.Errors():
		t.Errorf("unexpected E2E error: %v", e)
	default:
	}
}

func TestE2EPublisher_CounterIncrements(t *testing.T) {
	p := newPart(t)
	topic := uniqueTopic("e2e/counter")
	cfg := safety.E2EConfig{}

	rawPub, _ := p.NewPublisher(topic, dds.DefaultQoS)
	rawSub, _ := p.NewSubscriber(topic, dds.DefaultQoS, dds.WithChannelDepth(10))
	defer rawSub.Close()

	pub := safety.NewE2EPublisher(rawPub, cfg)
	defer func() { _ = pub.Close() }()

	const n = 5
	for i := 0; i < n; i++ {
		if err := pub.Write([]byte("x")); err != nil {
			t.Fatalf("Write %d: %v", i, err)
		}
	}

	counters := make([]uint32, 0, n)
	for i := 0; i < n; i++ {
		select {
		case raw := <-rawSub.C():
			if len(raw.Payload) < e2eHeaderSize {
				t.Fatalf("frame[%d] too short", i)
			}
			ctr := binary.LittleEndian.Uint32(raw.Payload[4:8])
			counters = append(counters, ctr)
		case <-time.After(time.Second):
			t.Fatalf("timeout waiting for frame %d", i)
		}
	}
	for i, c := range counters {
		if c != uint32(i+1) {
			t.Errorf("counter[%d]: got %d, want %d", i, c, i+1)
		}
	}
}

func TestE2EPublisher_HeaderPresent(t *testing.T) {
	p := newPart(t)
	topic := uniqueTopic("e2e/header")
	rawPub, _ := p.NewPublisher(topic, dds.DefaultQoS)
	rawSub, _ := p.NewSubscriber(topic, dds.DefaultQoS)
	defer rawSub.Close()

	cfg := safety.E2EConfig{DataID: 0xAB, SourceID: 0xCD}
	pub := safety.NewE2EPublisher(rawPub, cfg)
	defer func() { _ = pub.Close() }()

	_ = pub.Write([]byte("payload"))
	select {
	case raw := <-rawSub.C():
		if len(raw.Payload) < e2eHeaderSize+len("payload") {
			t.Fatalf("frame too short: %d", len(raw.Payload))
		}
		gotDataID := binary.LittleEndian.Uint16(raw.Payload[0:2])
		gotSourceID := binary.LittleEndian.Uint16(raw.Payload[2:4])
		if gotDataID != cfg.DataID {
			t.Errorf("DataID: got %d, want %d", gotDataID, cfg.DataID)
		}
		if gotSourceID != cfg.SourceID {
			t.Errorf("SourceID: got %d, want %d", gotSourceID, cfg.SourceID)
		}
		// Verify payload is at offset 18
		if !bytes.Equal(raw.Payload[e2eHeaderSize:], []byte("payload")) {
			t.Errorf("payload at offset 18: got %q", raw.Payload[e2eHeaderSize:])
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

func TestE2ESubscriber_CRCMismatch_ReportsError(t *testing.T) {
	p := newPart(t)
	topic := uniqueTopic("e2e/crc")
	cfg := safety.E2EConfig{}

	rawPub, _ := p.NewPublisher(topic, dds.DefaultQoS)
	rawSub, _ := p.NewSubscriber(topic, dds.DefaultQoS)
	defer rawSub.Close()

	// Send a frame manually with a bad CRC.
	badFrame := make([]byte, e2eHeaderSize+3)
	copy(badFrame[e2eHeaderSize:], []byte("bad"))
	binary.LittleEndian.PutUint16(badFrame[16:18], 0xDEAD) // wrong CRC
	if err := rawPub.Write(badFrame); err != nil {
		t.Fatalf("rawPub.Write: %v", err)
	}
	_ = rawPub.Close()

	sub := safety.NewE2ESubscriber(rawSub, cfg)
	defer func() { _ = sub.Close() }()

	select {
	case e := <-sub.Errors():
		if e.Kind != safety.ErrCRCMismatch {
			t.Errorf("expected ErrCRCMismatch, got kind %d: %v", e.Kind, e)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout: expected CRC error")
	}
}

func TestE2ESubscriber_ShortPayload_ReportsError(t *testing.T) {
	p := newPart(t)
	topic := uniqueTopic("e2e/short")
	rawPub, _ := p.NewPublisher(topic, dds.DefaultQoS)
	rawSub, _ := p.NewSubscriber(topic, dds.DefaultQoS)
	defer rawSub.Close()

	// Payload shorter than headerSize.
	if err := rawPub.Write([]byte("tiny")); err != nil {
		t.Fatalf("rawPub.Write: %v", err)
	}
	_ = rawPub.Close()

	sub := safety.NewE2ESubscriber(rawSub, safety.E2EConfig{})
	defer func() { _ = sub.Close() }()

	select {
	case e := <-sub.Errors():
		if e.Kind != safety.ErrHeaderTooShort {
			t.Errorf("expected ErrHeaderTooShort, got kind %d", e.Kind)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout: expected header-too-short error")
	}
}

func TestE2ESubscriber_SequenceGap_ReportsError(t *testing.T) {
	p := newPart(t)
	topic := uniqueTopic("e2e/seqgap")
	cfg := safety.E2EConfig{}

	rawPub, _ := p.NewPublisher(topic, dds.DefaultQoS)
	rawSub, _ := p.NewSubscriber(topic, dds.DefaultQoS, dds.WithChannelDepth(10))
	defer rawSub.Close()

	pub := safety.NewE2EPublisher(rawPub, cfg)
	sub := safety.NewE2ESubscriber(rawSub, cfg)
	defer func() { _ = pub.Close() }()
	defer func() { _ = sub.Close() }()

	// Write 1, then skip to 3 by writing with a separate publisher that starts
	// its counter from 1 again — wait, we can't do that.
	// Instead: write two samples, consume first, then inject a raw frame with
	// counter=5 to trigger a gap.
	_ = pub.Write([]byte("first"))
	select {
	case <-sub.C():
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for first sample")
	}

	// Now inject a raw frame with counter=10 (gap after counter=1).
	gapFrame := buildFrame(cfg, 10, []byte("gap"))
	if err := rawPub.Write(gapFrame); err != nil {
		t.Fatalf("rawPub.Write: %v", err)
	}

	// The sample should still be delivered (gap doesn't suppress delivery).
	select {
	case <-sub.C():
	case <-time.After(time.Second):
		t.Fatal("timeout: sample with gap should still be delivered")
	}
	select {
	case e := <-sub.Errors():
		if e.Kind != safety.ErrSequenceGap {
			t.Errorf("expected ErrSequenceGap, got kind %d", e.Kind)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout: expected sequence gap error")
	}
}

func TestE2ESubscriber_FreshnessCheck_StaleReportsError(t *testing.T) {
	p := newPart(t)
	topic := uniqueTopic("e2e/stale")
	cfg := safety.E2EConfig{MaxAge: 10 * time.Millisecond}

	rawPub, _ := p.NewPublisher(topic, dds.DefaultQoS)
	rawSub, _ := p.NewSubscriber(topic, dds.DefaultQoS)
	defer rawSub.Close()

	// Inject a frame with a timestamp from 1 second ago.
	staleFrame := buildFrameAt(cfg, 1, []byte("old"), time.Now().Add(-time.Second))
	if err := rawPub.Write(staleFrame); err != nil {
		t.Fatalf("rawPub.Write: %v", err)
	}
	_ = rawPub.Close()

	sub := safety.NewE2ESubscriber(rawSub, cfg)
	defer func() { _ = sub.Close() }()

	select {
	case e := <-sub.Errors():
		if e.Kind != safety.ErrStaleSample {
			t.Errorf("expected ErrStaleSample, got kind %d: %v", e.Kind, e)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout: expected stale sample error")
	}
}

func TestE2ESubscriber_FreshnessCheck_FreshPasses(t *testing.T) {
	p := newPart(t)
	topic := uniqueTopic("e2e/fresh")
	cfg := safety.E2EConfig{DataID: 1, SourceID: 1, MaxAge: time.Second}

	rawPub, _ := p.NewPublisher(topic, dds.DefaultQoS)
	rawSub, _ := p.NewSubscriber(topic, dds.DefaultQoS)
	defer rawSub.Close()

	pub := safety.NewE2EPublisher(rawPub, cfg)
	sub := safety.NewE2ESubscriber(rawSub, cfg)
	defer func() { _ = pub.Close() }()
	defer func() { _ = sub.Close() }()

	if err := pub.Write([]byte("fresh")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	select {
	case s := <-sub.C():
		if string(s.Payload) != "fresh" {
			t.Errorf("payload: got %q", s.Payload)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
	select {
	case e := <-sub.Errors():
		t.Errorf("unexpected error for fresh sample: %v", e)
	default:
	}
}

func TestE2ESubscriber_DisabledFreshness_AcceptsOld(t *testing.T) {
	p := newPart(t)
	topic := uniqueTopic("e2e/no-age")
	cfg := safety.E2EConfig{MaxAge: 0} // disabled

	rawPub, _ := p.NewPublisher(topic, dds.DefaultQoS)
	rawSub, _ := p.NewSubscriber(topic, dds.DefaultQoS)
	defer rawSub.Close()

	// Very old timestamp — should pass with MaxAge=0.
	oldFrame := buildFrameAt(cfg, 1, []byte("ancient"), time.Now().Add(-24*time.Hour))
	if err := rawPub.Write(oldFrame); err != nil {
		t.Fatalf("rawPub.Write: %v", err)
	}
	_ = rawPub.Close()

	sub := safety.NewE2ESubscriber(rawSub, cfg)
	defer func() { _ = sub.Close() }()

	select {
	case <-sub.C():
		// delivered
	case e := <-sub.Errors():
		t.Errorf("unexpected error with MaxAge=0: %v", e)
	case <-time.After(time.Second):
		t.Fatal("timeout: ancient sample should be accepted when MaxAge=0")
	}
}

func TestE2ESubscriber_Close(t *testing.T) {
	p := newPart(t)
	topic := uniqueTopic("e2e/close")
	rawSub, _ := p.NewSubscriber(topic, dds.DefaultQoS)
	sub := safety.NewE2ESubscriber(rawSub, safety.E2EConfig{})
	if err := sub.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestE2ESubscriber_CloseIdempotent(t *testing.T) {
	p := newPart(t)
	topic := uniqueTopic("e2e/close-idem")
	rawSub, _ := p.NewSubscriber(topic, dds.DefaultQoS)
	sub := safety.NewE2ESubscriber(rawSub, safety.E2EConfig{})
	_ = sub.Close()
	_ = sub.Close() // must not panic
}

func TestE2EPublisher_Close(t *testing.T) {
	p := newPart(t)
	topic := uniqueTopic("e2e/pub-close")
	rawPub, _ := p.NewPublisher(topic, dds.DefaultQoS)
	pub := safety.NewE2EPublisher(rawPub, safety.E2EConfig{})
	if err := pub.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

// TestE2EError_Error verifies the Error() method returns the Message field.
func TestE2EError_Error(t *testing.T) {
	e := safety.E2EError{Kind: safety.ErrCRCMismatch, Message: "crc check failed"}
	if got := e.Error(); got != "crc check failed" {
		t.Errorf("Error(): got %q, want %q", got, "crc check failed")
	}
}

// TestE2ESubscriber_ClosedRawSub exercises the pump's !ok branch, which fires
// when the underlying subscriber's channel is closed directly.
func TestE2ESubscriber_ClosedRawSub(t *testing.T) {
	p := newPart(t)
	topic := uniqueTopic("e2e/rawclose")
	rawSub, _ := p.NewSubscriber(topic, dds.DefaultQoS)

	sub := safety.NewE2ESubscriber(rawSub, safety.E2EConfig{})

	// Closing the raw subscriber closes its channel; the pump sees !ok and exits.
	rawSub.Close()

	// Give the pump goroutine time to see the closed channel and stop.
	select {
	case <-sub.C():
		// ch was closed by pump; receiving zero-value or channel close is fine
	case <-time.After(500 * time.Millisecond):
		// pump exited without sending — also fine
	}
	// Close must not block since the pump has already exited.
	if err := sub.Close(); err != nil {
		t.Fatalf("sub.Close after rawSub.Close: %v", err)
	}
}

// ── frame builder helpers (white-box helpers for test scenarios) ──────────────

// buildFrame constructs a valid E2E frame — used to craft specific counter values.
func buildFrame(cfg safety.E2EConfig, counter uint32, payload []byte) []byte {
	return buildFrameAt(cfg, counter, payload, time.Now())
}

func buildFrameAt(cfg safety.E2EConfig, counter uint32, payload []byte, ts time.Time) []byte {
	const headerSize = 18
	buf := make([]byte, headerSize+len(payload))
	binary.LittleEndian.PutUint16(buf[0:2], cfg.DataID)
	binary.LittleEndian.PutUint16(buf[2:4], cfg.SourceID)
	binary.LittleEndian.PutUint32(buf[4:8], counter)
	binary.LittleEndian.PutUint64(buf[8:16], uint64(ts.UnixNano()))
	copy(buf[18:], payload)

	crcInput := make([]byte, 16+len(payload))
	copy(crcInput, buf[:16])
	copy(crcInput[16:], payload)
	binary.LittleEndian.PutUint16(buf[16:18], crc16CCITT(crcInput))
	return buf
}

// crc16CCITT is a copy of the safety package's internal CRC function for
// use in test frame construction.
func crc16CCITT(data []byte) uint16 {
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
