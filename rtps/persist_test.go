// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package rtps

//fusa:test REQ-REL-002
//fusa:test REQ-REL-010
//fusa:test REQ-REL-011
//fusa:test REQ-REL-012

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// TestPersistLoad_EmptyDir verifies that persistLoad returns (nil, nil) when
// dir is empty — the "persistence disabled" fast-path.
func TestPersistLoad_EmptyDir(t *testing.T) {
	data, err := persistLoad("", "any/topic")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if data != nil {
		t.Errorf("expected nil data, got %v", data)
	}
}

// TestPersistLoad_FileNotFound verifies that persistLoad returns (nil, err)
// when the file does not exist — normal on first run.
func TestPersistLoad_FileNotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := persistLoad(dir, "no/such/topic")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

// TestPersistLoad_TruncatedHeader verifies that persistLoad returns an error
// when the file is too short to contain the 4-byte length header.
func TestPersistLoad_TruncatedHeader(t *testing.T) {
	dir := t.TempDir()
	path := persistPath(dir, "trunc/header")
	if err := os.WriteFile(path, []byte{0x01}, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	_, err := persistLoad(dir, "trunc/header")
	if err == nil {
		t.Fatal("expected error for truncated header")
	}
}

// TestPersistLoad_OversizedPayload verifies that persistLoad returns an error
// when the declared payload length exceeds the 64 MiB cap.
func TestPersistLoad_OversizedPayload(t *testing.T) {
	dir := t.TempDir()
	path := persistPath(dir, "oversized/topic")
	// Write a 4-byte length header declaring 65 MiB.
	var hdr [4]byte
	binary.LittleEndian.PutUint32(hdr[:], 65*1024*1024)
	if err := os.WriteFile(path, hdr[:], 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	_, err := persistLoad(dir, "oversized/topic")
	if err == nil {
		t.Fatal("expected error for oversized payload length")
	}
}

// TestPersistLoad_MissingPayload verifies that persistLoad returns an error
// when the file contains only the 4-byte length header but no payload bytes.
// f.Read on an empty remainder returns io.EOF which propagates as an error.
func TestPersistLoad_MissingPayload(t *testing.T) {
	dir := t.TempDir()
	path := persistPath(dir, "missing/payload")
	// Declare 5-byte payload but write zero payload bytes.
	var hdr [4]byte
	binary.LittleEndian.PutUint32(hdr[:], 5)
	if err := os.WriteFile(path, hdr[:], 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	_, err := persistLoad(dir, "missing/payload")
	if err == nil {
		t.Fatal("expected error for file with header but no payload")
	}
}

// TestPersistRoundtrip verifies persistFlush+persistLoad produces the
// original payload unchanged.
func TestPersistRoundtrip(t *testing.T) {
	dir := t.TempDir()
	want := []byte("hello, world!")
	persistFlush(dir, "test/topic", want)
	got, err := persistLoad(dir, "test/topic")
	if err != nil {
		t.Fatalf("persistLoad: %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestPersistPath_SpecialChars verifies that topic slashes and colons are
// replaced to produce a safe flat filename.
func TestPersistPath_SpecialChars(t *testing.T) {
	path := persistPath("/tmp/data", "a/b:c\\d")
	base := filepath.Base(path)
	for _, ch := range []byte{'/', ':', '\\'} {
		for _, b := range []byte(base) {
			if b == ch {
				t.Errorf("persistPath: unsafe char %q in %q", ch, base)
			}
		}
	}
}

// TestPersistFlush_EmptyDir verifies persistFlush is a no-op when dir is "".
func TestPersistFlush_EmptyDir(t *testing.T) {
	persistFlush("", "any/topic", []byte("data")) // must not panic
}

// TestPersistFlush_InvalidDir verifies persistFlush silently ignores errors
// when the directory does not exist.
func TestPersistFlush_InvalidDir(t *testing.T) {
	persistFlush("/nonexistent/dir", "any/topic", []byte("data")) // must not panic
}
