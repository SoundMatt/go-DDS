// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package xtypes_test

import (
	"sync"
	"testing"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/xtypes"
)

type Temperature struct{ Celsius float64 }
type Pressure struct{ Bar float64 }

// rawCodec[T] satisfies dds.Codec[T] (Marshal+Unmarshal) for registration tests.
type rawCodec[T any] struct{}

func (rawCodec[T]) Marshal(_ T) ([]byte, error)   { return nil, nil }
func (rawCodec[T]) Unmarshal(_ []byte) (T, error) { var zero T; return zero, nil }

var _ dds.Codec[Temperature] = rawCodec[Temperature]{}

// ── Register / Lookup ─────────────────────────────────────────────────────────

func TestTopicTypeRegistry_RegisterAndLookup(t *testing.T) {
	r := xtypes.NewTopicTypeRegistry()
	xtypes.RegisterTopicCodec(r, "sensors/temp", rawCodec[Temperature]{})

	name, ok := r.LookupTopicType("sensors/temp")
	if !ok {
		t.Fatal("LookupTopicType: not found")
	}
	if name == "" {
		t.Error("type name must not be empty")
	}
	// The registered name must contain the struct name.
	if name != "xtypes_test.Temperature" {
		t.Errorf("unexpected type name %q", name)
	}
}

func TestTopicTypeRegistry_LookupUnregistered(t *testing.T) {
	r := xtypes.NewTopicTypeRegistry()
	_, ok := r.LookupTopicType("nonexistent/topic")
	if ok {
		t.Error("expected false for unregistered topic")
	}
}

func TestTopicTypeRegistry_OverwriteRegistration(t *testing.T) {
	r := xtypes.NewTopicTypeRegistry()
	xtypes.RegisterTopicCodec(r, "sensors/mixed", rawCodec[Temperature]{})
	xtypes.RegisterTopicCodec(r, "sensors/mixed", rawCodec[Pressure]{})

	name, ok := r.LookupTopicType("sensors/mixed")
	if !ok {
		t.Fatal("topic not found after re-registration")
	}
	if name != "xtypes_test.Pressure" {
		t.Errorf("expected Pressure type name, got %q", name)
	}
}

// ── All ───────────────────────────────────────────────────────────────────────

func TestTopicTypeRegistry_All_Empty(t *testing.T) {
	r := xtypes.NewTopicTypeRegistry()
	if all := r.All(); len(all) != 0 {
		t.Errorf("expected empty, got %v", all)
	}
}

func TestTopicTypeRegistry_All_Sorted(t *testing.T) {
	r := xtypes.NewTopicTypeRegistry()
	xtypes.RegisterTopicCodec(r, "zzz/last", rawCodec[Temperature]{})
	xtypes.RegisterTopicCodec(r, "aaa/first", rawCodec[Pressure]{})
	xtypes.RegisterTopicCodec(r, "mmm/mid", rawCodec[Temperature]{})

	all := r.All()
	if len(all) != 3 {
		t.Fatalf("expected 3, got %d", len(all))
	}
	if all[0].Topic != "aaa/first" || all[1].Topic != "mmm/mid" || all[2].Topic != "zzz/last" {
		t.Errorf("not sorted: %v", all)
	}
}

func TestTopicTypeRegistry_All_FieldsPopulated(t *testing.T) {
	r := xtypes.NewTopicTypeRegistry()
	xtypes.RegisterTopicCodec(r, "my/topic", rawCodec[Temperature]{})

	all := r.All()
	if len(all) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(all))
	}
	if all[0].Topic == "" {
		t.Error("TopicCodecInfo.Topic is empty")
	}
	if all[0].TypeName == "" {
		t.Error("TopicCodecInfo.TypeName is empty")
	}
}

// ── Deregister ────────────────────────────────────────────────────────────────

func TestTopicTypeRegistry_Deregister(t *testing.T) {
	r := xtypes.NewTopicTypeRegistry()
	xtypes.RegisterTopicCodec(r, "sensors/temp", rawCodec[Temperature]{})
	r.Deregister("sensors/temp")

	_, ok := r.LookupTopicType("sensors/temp")
	if ok {
		t.Error("expected topic to be absent after Deregister")
	}
}

func TestTopicTypeRegistry_Deregister_Idempotent(t *testing.T) {
	r := xtypes.NewTopicTypeRegistry()
	r.Deregister("never/registered") // must not panic
	r.Deregister("never/registered") // second call also safe
}

// ── GlobalTopicRegistry ───────────────────────────────────────────────────────

func TestGlobalTopicRegistry_NotNil(t *testing.T) {
	if xtypes.GlobalTopicRegistry == nil {
		t.Error("GlobalTopicRegistry is nil")
	}
}

// ── Concurrency ───────────────────────────────────────────────────────────────

func TestTopicTypeRegistry_ConcurrentAccess(t *testing.T) {
	r := xtypes.NewTopicTypeRegistry()
	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(n int) {
			defer wg.Done()
			topic := "concurrent/sensor"
			switch n % 3 {
			case 0:
				xtypes.RegisterTopicCodec(r, topic, rawCodec[Temperature]{})
			case 1:
				r.LookupTopicType(topic)
			default:
				r.All()
			}
		}(i)
	}
	wg.Wait()
}

// ── Fuzz ──────────────────────────────────────────────────────────────────────

func FuzzTopicTypeRegistry_RegisterLookup(f *testing.F) {
	f.Add("sensors/temperature")
	f.Add("")
	f.Add("/leading-slash")
	f.Add("topic with spaces")
	f.Add("a/b/c/d/e/f/g")

	f.Fuzz(func(t *testing.T, topic string) {
		r := xtypes.NewTopicTypeRegistry()
		xtypes.RegisterTopicCodec(r, topic, rawCodec[Temperature]{})
		name, ok := r.LookupTopicType(topic)
		if !ok {
			t.Errorf("topic %q not found after registration", topic)
		}
		if name == "" {
			t.Error("type name must not be empty")
		}
		r.Deregister(topic)
		if _, ok := r.LookupTopicType(topic); ok {
			t.Errorf("topic %q still found after Deregister", topic)
		}
	})
}
