// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package pool_test

//fusa:test REQ-MEM-001
//fusa:test REQ-MEM-002
//fusa:test REQ-MEM-003
//fusa:test REQ-MEM-004
//fusa:test REQ-MEM-005
//fusa:test REQ-MEM-006

import (
	"fmt"
	"sync"
	"testing"
	"time"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/pool"
)

// ── BytePool ──────────────────────────────────────────────────────────────────

func TestBytePool_GetReturnsCorrectCapacity(t *testing.T) {
	bp := pool.New(512)
	buf := bp.Get()
	if cap(buf) < 512 {
		t.Errorf("Get: cap %d < 512", cap(buf))
	}
	if len(buf) != 0 {
		t.Errorf("Get: len %d, want 0", len(buf))
	}
}

func TestBytePool_PutAndGetReusesBuffer(t *testing.T) {
	bp := pool.New(256)
	buf := bp.Get()
	buf = append(buf, 1, 2, 3)
	bp.Put(buf)

	reused := bp.Get()
	if cap(reused) < 256 {
		t.Errorf("reused buf cap %d < 256", cap(reused))
	}
	if len(reused) != 0 {
		t.Errorf("reused buf len %d, want 0", len(reused))
	}
}

func TestBytePool_PutDiscardsUndersizedBuffer(t *testing.T) {
	bp := pool.New(1024)
	small := make([]byte, 0, 16)
	bp.Put(small) // should not panic, just discarded
	// Get should still return a properly-sized buffer.
	buf := bp.Get()
	if cap(buf) < 1024 {
		t.Errorf("Get after discarded Put: cap %d < 1024", cap(buf))
	}
}

func TestBytePool_ZeroSizeDefaulted(t *testing.T) {
	// size <= 0 should default to 4096.
	bp := pool.New(0)
	buf := bp.Get()
	if cap(buf) < 4096 {
		t.Errorf("cap %d < 4096 for zero-size pool", cap(buf))
	}
}

func TestBytePool_NegativeSizeDefaulted(t *testing.T) {
	bp := pool.New(-1)
	buf := bp.Get()
	if cap(buf) < 4096 {
		t.Errorf("cap %d < 4096 for negative-size pool", cap(buf))
	}
}

func TestBytePool_Concurrent(t *testing.T) {
	bp := pool.New(128)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			buf := bp.Get()
			buf = append(buf, byte(len(buf)))
			bp.Put(buf)
		}()
	}
	wg.Wait()
}

func TestBytePool_AppendWithinCapacity(t *testing.T) {
	const size = 64
	bp := pool.New(size)
	buf := bp.Get()
	for i := 0; i < size; i++ {
		buf = append(buf, byte(i))
	}
	if cap(buf) != size {
		t.Errorf("unexpected reallocation: cap grew from %d to %d", size, cap(buf))
	}
}

// ── SampleBuffer ──────────────────────────────────────────────────────────────

func TestSampleBuffer_PushAndPop(t *testing.T) {
	sb := pool.NewSampleBuffer(4)
	s := dds.Sample{Topic: "t", Payload: []byte("p")}
	if !sb.Push(s) {
		t.Fatal("Push: expected true on empty buffer")
	}
	got, ok := sb.Pop()
	if !ok {
		t.Fatal("Pop: expected true with one item")
	}
	if got.Topic != "t" || string(got.Payload) != "p" {
		t.Errorf("Pop: got %+v, want {Topic:t Payload:p}", got)
	}
}

func TestSampleBuffer_PopEmpty(t *testing.T) {
	sb := pool.NewSampleBuffer(4)
	ignoredRet, ok := sb.Pop()
	_ = ignoredRet
	if ok {
		t.Fatal("Pop on empty buffer should return false")
	}
}

func TestSampleBuffer_PushFull(t *testing.T) {
	sb := pool.NewSampleBuffer(2)
	if !sb.Push(dds.Sample{Topic: "a"}) {
		t.Fatal("first Push should succeed")
	}
	if !sb.Push(dds.Sample{Topic: "b"}) {
		t.Fatal("second Push should succeed")
	}
	if sb.Push(dds.Sample{Topic: "c"}) {
		t.Fatal("Push on full buffer should return false")
	}
}

func TestSampleBuffer_Wraparound(t *testing.T) {
	sb := pool.NewSampleBuffer(3)
	for i := 0; i < 3; i++ {
		sb.Push(dds.Sample{Topic: fmt.Sprintf("t%d", i)})
	}
	sb.Pop()                         // head moves to 1
	sb.Push(dds.Sample{Topic: "t3"}) // tail wraps to 0

	expected := []string{"t1", "t2", "t3"}
	for i, want := range expected {
		s, ok := sb.Pop()
		if !ok {
			t.Fatalf("Pop[%d]: expected ok=true", i)
		}
		if s.Topic != want {
			t.Errorf("Pop[%d]: got %q, want %q", i, s.Topic, want)
		}
	}
}

func TestSampleBuffer_Len(t *testing.T) {
	sb := pool.NewSampleBuffer(8)
	if sb.Len() != 0 {
		t.Errorf("initial Len: got %d, want 0", sb.Len())
	}
	sb.Push(dds.Sample{})
	if sb.Len() != 1 {
		t.Errorf("after 1 push: Len=%d, want 1", sb.Len())
	}
	sb.Pop()
	if sb.Len() != 0 {
		t.Errorf("after pop: Len=%d, want 0", sb.Len())
	}
}

func TestSampleBuffer_Cap(t *testing.T) {
	sb := pool.NewSampleBuffer(16)
	if sb.Cap() != 16 {
		t.Errorf("Cap: got %d, want 16", sb.Cap())
	}
}

func TestSampleBuffer_DefaultCapacity(t *testing.T) {
	sb := pool.NewSampleBuffer(0)
	if sb.Cap() != 64 {
		t.Errorf("default Cap: got %d, want 64", sb.Cap())
	}
}

func TestSampleBuffer_Concurrent(t *testing.T) {
	sb := pool.NewSampleBuffer(128)
	var wg sync.WaitGroup
	const producers = 10
	const perProducer = 10
	for i := 0; i < producers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < perProducer; j++ {
				sb.Push(dds.Sample{Topic: "concurrent"})
				time.Sleep(time.Microsecond)
			}
		}()
	}
	go func() {
		for {
			ignoredRet, ok := sb.Pop()
			_ = ignoredRet
			if !ok {
				time.Sleep(time.Microsecond)
			}
		}
	}()
	wg.Wait()
}

func TestSampleBuffer_FullThenDrain(t *testing.T) {
	const cap = 4
	sb := pool.NewSampleBuffer(cap)
	for i := 0; i < cap; i++ {
		if !sb.Push(dds.Sample{Topic: fmt.Sprintf("s%d", i)}) {
			t.Fatalf("Push[%d] failed prematurely", i)
		}
	}
	for i := 0; i < cap; i++ {
		s, ok := sb.Pop()
		if !ok {
			t.Fatalf("Pop[%d] returned ok=false", i)
		}
		want := fmt.Sprintf("s%d", i)
		if s.Topic != want {
			t.Errorf("Pop[%d]: got %q, want %q", i, s.Topic, want)
		}
	}
	ignoredRet, ok := sb.Pop()
	_ = ignoredRet
	if ok {
		t.Fatal("Pop after full drain should return false")
	}
}
