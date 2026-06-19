// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package rtps

//fusa:test REQ-REL-001
//fusa:test REQ-REL-005
//fusa:test REQ-REL-008
//fusa:test REQ-REL-009
//fusa:test REQ-LLR-001
//fusa:test REQ-LLR-002

import "testing"

// TestSendHistory_Store_Eviction covers the bounded ring: once more than
// maxHistoryDepth contiguous sequence numbers are stored, the oldest falls out
// of the retained window.
func TestSendHistory_Store_Eviction(t *testing.T) {
	h := newSendHistory()
	// Store maxHistoryDepth contiguous SNs (1..depth).
	for i := uint64(1); i <= maxHistoryDepth; i++ {
		h.store(i, []byte{byte(i)})
	}
	if h.get(1) == nil {
		t.Fatalf("entry 1 should still be present at depth == maxHistoryDepth")
	}
	// One more push evicts SN 1 (now outside the last `depth` window).
	h.store(maxHistoryDepth+1, []byte{0xFF})
	if h.get(1) != nil {
		t.Errorf("expected entry 1 to be evicted after exceeding maxHistoryDepth")
	}
	if h.get(maxHistoryDepth+1) == nil {
		t.Errorf("expected newest entry %d to be present", maxHistoryDepth+1)
	}
}

// TestSendHistory_NeverExceedsDepth asserts the store is provably bounded: even
// after many stores, no more than maxHistoryDepth slots are ever occupied.
func TestSendHistory_NeverExceedsDepth(t *testing.T) {
	h := newSendHistory()
	for i := uint64(1); i <= maxHistoryDepth*4; i++ {
		h.store(i, []byte{0x01})
	}
	occupied := 0
	for _, f := range h.filled {
		if f {
			occupied++
		}
	}
	if occupied > maxHistoryDepth {
		t.Errorf("ring holds %d entries, exceeds depth %d", occupied, maxHistoryDepth)
	}
	first, last, ok := h.firstLast()
	if !ok || last != maxHistoryDepth*4 || first != last-maxHistoryDepth+1 {
		t.Errorf("window: first=%d last=%d ok=%v", first, last, ok)
	}
}

// TestSendHistory_FirstLast_Empty covers the empty-history path in firstLast.
func TestSendHistory_FirstLast_Empty(t *testing.T) {
	h := newSendHistory()
	_, _, ok := h.firstLast()
	if ok {
		t.Error("expected ok=false for empty history")
	}
}

// TestSendHistory_FirstLast_NonEmpty covers the non-empty path in firstLast for
// the contiguous, monotonic stores a writer actually produces.
func TestSendHistory_FirstLast_NonEmpty(t *testing.T) {
	h := newSendHistory()
	h.store(3, []byte("a"))
	h.store(4, []byte("b"))
	h.store(5, []byte("c"))
	first, last, ok := h.firstLast()
	if !ok {
		t.Fatal("expected ok=true for non-empty history")
	}
	if first != 3 {
		t.Errorf("first: got %d, want 3", first)
	}
	if last != 5 {
		t.Errorf("last: got %d, want 5", last)
	}
}

// TestSendHistory_64BitNoAlias verifies a sequence number above 2^32 is stored
// and retrieved without aliasing a low-32-bit collision.
func TestSendHistory_64BitNoAlias(t *testing.T) {
	h := newSendHistory()
	const high = uint64(1) << 33
	h.store(high+1, []byte("hi"))
	if got := h.get(high + 1); got == nil || string(got) != "hi" {
		t.Fatalf("64-bit SN not retrievable: %q", got)
	}
	// The aliasing low-32-bit key (1) must not collide.
	if h.get(1) != nil {
		t.Error("low-32-bit key 1 aliased a 2^33+1 entry")
	}
}
