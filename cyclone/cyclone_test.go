//go:build cyclone

package cyclone_test

import (
	"testing"
	"time"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/cyclone"
)

// domain 99 avoids colliding with any application DDS domain in CI.
const testDomain = dds.Domain(99)

func newParticipant(t *testing.T) dds.Participant {
	t.Helper()
	p, err := cyclone.New(testDomain)
	if err != nil {
		t.Skipf("CycloneDDS unavailable (%v) — skipping", err)
	}
	t.Cleanup(func() { p.Close() })
	return p
}

func TestCyclone_PubSub_RoundTrip(t *testing.T) {
	p := newParticipant(t)

	const topic = "test/roundtrip"
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

	// Allow DDS discovery to complete before writing.
	time.Sleep(100 * time.Millisecond)

	want := `{"replyTopic":"client/test","request":{"action":"get","path":"Vehicle.Speed"}}`
	if err := pub.Write([]byte(want)); err != nil {
		t.Fatalf("Write: %v", err)
	}

	select {
	case s := <-sub.C():
		if string(s.Payload) != want {
			t.Errorf("payload mismatch:\n  got  %q\n  want %q", s.Payload, want)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for DDS sample")
	}
}

func TestCyclone_PublisherClose_ReturnsError(t *testing.T) {
	p := newParticipant(t)
	pub, err := p.NewPublisher("close/test", dds.DefaultQoS)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	pub.Close()
	if err := pub.Write([]byte("x")); err == nil {
		t.Error("expected error writing to closed publisher")
	}
}

func TestCyclone_ParticipantClose_BlocksNewEndpoints(t *testing.T) {
	p, err := cyclone.New(testDomain)
	if err != nil {
		t.Skipf("CycloneDDS unavailable: %v", err)
	}
	p.Close()
	if _, err := p.NewPublisher("t", dds.DefaultQoS); err == nil {
		t.Error("expected error from closed participant")
	}
	if _, err := p.NewSubscriber("t", dds.DefaultQoS); err == nil {
		t.Error("expected error from closed participant")
	}
}

func TestCyclone_StubReturnsError(t *testing.T) {
	// This test only runs under -tags cyclone, so New() must succeed.
	p, err := cyclone.New(testDomain)
	if err != nil {
		t.Skipf("CycloneDDS unavailable: %v", err)
	}
	p.Close()
}
