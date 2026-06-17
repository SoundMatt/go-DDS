// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Enforcement tests for topic ACL (WithAccessControl) and anti-replay
// (WithAntiReplay), and their composition with encryption.

package rtps_test

//fusa:test REQ-SEC-013
//fusa:test REQ-SEC-003

import (
	"bytes"
	"errors"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/rtps"
	"github.com/SoundMatt/go-DDS/security"
)

func TestAccessControl_PublishDenied(t *testing.T) {
	policy := security.NewAccessPolicy(
		security.Rule{Pattern: "sensors/*", Allow: security.PermRead}, // read-only
		security.Rule{Pattern: "cmd/*", Allow: security.PermWrite},
	)
	p, newErr := rtps.New(testDomain, rtps.WithNoMulticast(), rtps.WithAccessControl(policy))
	if newErr != nil {
		t.Skipf("rtps.New: %v", newErr)
	}
	defer p.Close()

	// Write denied on a read-only topic.
	if _, err := p.NewPublisher("sensors/temp", dds.DefaultQoS); !errors.Is(err, dds.ErrAccessDenied) {
		t.Errorf("NewPublisher on read-only topic: got %v, want ErrAccessDenied", err)
	}
	// Write allowed on a write topic.
	pub, err := p.NewPublisher("cmd/move", dds.DefaultQoS)
	if err != nil {
		t.Errorf("NewPublisher on writable topic: %v", err)
	} else {
		_ = pub.Close()
	}
	// A topic matching no rule is denied.
	if _, err := p.NewPublisher("other/topic", dds.DefaultQoS); !errors.Is(err, dds.ErrAccessDenied) {
		t.Errorf("NewPublisher on unlisted topic: got %v, want ErrAccessDenied", err)
	}
}

func TestAccessControl_SubscribeDenied(t *testing.T) {
	policy := security.NewAccessPolicy(
		security.Rule{Pattern: "sensors/*", Allow: security.PermRead},
		security.Rule{Pattern: "cmd/*", Allow: security.PermWrite}, // write-only
	)
	p, newErr := rtps.New(testDomain, rtps.WithNoMulticast(), rtps.WithAccessControl(policy))
	if newErr != nil {
		t.Skipf("rtps.New: %v", newErr)
	}
	defer p.Close()

	// Read denied on a write-only topic.
	if _, err := p.NewSubscriber("cmd/move", dds.DefaultQoS); !errors.Is(err, dds.ErrAccessDenied) {
		t.Errorf("NewSubscriber on write-only topic: got %v, want ErrAccessDenied", err)
	}
	// Read allowed on a read topic.
	sub, err := p.NewSubscriber("sensors/temp", dds.DefaultQoS)
	if err != nil {
		t.Errorf("NewSubscriber on readable topic: %v", err)
	} else {
		_ = sub.Close()
	}
}

func TestAccessControl_NoPolicyAllowsAll(t *testing.T) {
	p, err := rtps.New(testDomain, rtps.WithNoMulticast())
	if err != nil {
		t.Skipf("rtps.New: %v", err)
	}
	defer p.Close()

	// With no access controller configured, any topic is permitted.
	pub, err := p.NewPublisher("anything/at/all", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher without policy should succeed: %v", err)
	}
	_ = pub.Close()
	sub, err := p.NewSubscriber("anything/at/all", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber without policy should succeed: %v", err)
	}
	_ = sub.Close()
}

// recordingReplay is a ReplayChecker stub: it counts calls and, when reject is
// set, reports every sample as a replay.
type recordingReplay struct {
	calls  atomic.Int32
	reject bool
}

func (r *recordingReplay) Check(seq uint64, ts time.Time) error {
	r.calls.Add(1)
	if r.reject {
		return errors.New("replay")
	}
	return nil
}

// TestAntiReplay_ReceivePath verifies that the configured ReplayChecker is
// consulted on the inbound DATA path and that a rejected sample is dropped.
func TestAntiReplay_ReceivePath(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping two-participant test in -short mode (requires network)")
	}
	if runtime.GOOS != "linux" {
		t.Skipf("UDP multicast loopback unreliable on %s; skipping", runtime.GOOS)
	}

	guard := &recordingReplay{reject: true}
	p1, err := rtps.New(testDomain)
	if err != nil {
		t.Skipf("rtps.New(p1): %v", err)
	}
	defer p1.Close()
	p2, err := rtps.New(testDomain, rtps.WithAntiReplay(guard))
	if err != nil {
		t.Skipf("rtps.New(p2): %v", err)
	}
	defer p2.Close()

	sub, err := p2.NewSubscriber("test/replay/drop", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()
	pub, err := p1.NewPublisher("test/replay/drop", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	time.Sleep(2200 * time.Millisecond) // SPDP + SEDP
	if err := pub.Write([]byte("blocked")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	select {
	case s := <-sub.C():
		t.Errorf("sample should have been dropped by replay guard, got %q", s.Payload)
	case <-time.After(1500 * time.Millisecond):
		// Expected: nothing delivered.
	}
	if guard.calls.Load() == 0 {
		t.Error("replay checker was never consulted on the receive path")
	}
}

// TestSecurity_Composes verifies encryption + ACL + anti-replay can all be
// configured together and an authorised pub/sub round-trips locally.
func TestSecurity_Composes(t *testing.T) {
	policy := security.NewAccessPolicy(security.Rule{Pattern: "secure/*", Allow: security.PermReadWrite})
	enc := security.NewHMACPlugin([]byte("composition-test-key"))
	guard := security.NewReplayGuard(5 * time.Second)

	p, err := rtps.New(testDomain,
		rtps.WithNoMulticast(),
		rtps.WithSecurity(enc),
		rtps.WithAccessControl(policy),
		rtps.WithAntiReplay(guard),
	)
	if err != nil {
		t.Skipf("rtps.New: %v", err)
	}
	defer p.Close()

	sub, err := p.NewSubscriber("secure/topic", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Close()
	pub, err := p.NewPublisher("secure/topic", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	want := []byte("composed")
	if err := pub.Write(want); err != nil {
		t.Fatalf("Write: %v", err)
	}
	select {
	case s := <-sub.C():
		if !bytes.Equal(s.Payload, want) {
			t.Errorf("payload: got %q, want %q", s.Payload, want)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for composed-security sample")
	}
}
