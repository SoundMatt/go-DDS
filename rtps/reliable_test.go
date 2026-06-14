// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package rtps

//fusa:test REQ-REL-001
//fusa:test REQ-REL-005
//fusa:test REQ-REL-008
//fusa:test REQ-REL-009

import "testing"

// TestSendHistory_Store_Eviction covers the eviction branch in store: when the
// history grows beyond maxHistoryDepth, the oldest entry is deleted.
func TestSendHistory_Store_Eviction(t *testing.T) {
	h := newSendHistory()
	// Fill history to exactly maxHistoryDepth entries.
	for i := uint32(0); i < maxHistoryDepth; i++ {
		h.store(i, []byte{byte(i)})
	}
	if h.get(0) == nil {
		t.Fatalf("entry 0 should still be present at depth == maxHistoryDepth")
	}
	// Adding one more triggers eviction of seqLo=0.
	h.store(maxHistoryDepth, []byte{0xFF})
	if h.get(0) != nil {
		t.Errorf("expected entry 0 to be evicted after exceeding maxHistoryDepth")
	}
	if h.get(maxHistoryDepth) == nil {
		t.Errorf("expected newest entry %d to be present", maxHistoryDepth)
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

// TestSendHistory_FirstLast_NonEmpty covers the non-empty path in firstLast.
func TestSendHistory_FirstLast_NonEmpty(t *testing.T) {
	h := newSendHistory()
	h.store(5, []byte("a"))
	h.store(10, []byte("b"))
	h.store(3, []byte("c"))
	first, last, ok := h.firstLast()
	if !ok {
		t.Fatal("expected ok=true for non-empty history")
	}
	if first != 3 {
		t.Errorf("first: got %d, want 3", first)
	}
	if last != 10 {
		t.Errorf("last: got %d, want 10", last)
	}
}
