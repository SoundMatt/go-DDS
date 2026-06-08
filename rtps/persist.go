// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package rtps

//fusa:req REQ-REL-002
//fusa:req REQ-REL-010
//fusa:req REQ-REL-011
//fusa:req REQ-REL-012

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// WithPersistentHistory configures TransientLocal durability to be backed by
// files in dir. On participant start the last sample for each topic is loaded
// from disk; on every Write the new last sample is flushed to disk. The file
// name for topic T is <dir>/topic-<sanitised(T)>.bin. Each file holds a
// 4-byte length prefix followed by the raw sample bytes.
//
// The option is a no-op when dir is "". Failures during load are silently
// ignored (missing files are normal on first run); failures during flush are
// also silently ignored so that a write to a read-only directory does not
// block the caller.
func WithPersistentHistory(dir string) Option {
	return func(p *participant) { p.persistDir = dir }
}

// persistLoad reads the last-written sample for topic from dir, if present.
func persistLoad(dir, topic string) ([]byte, error) {
	if dir == "" {
		return nil, nil
	}
	path := persistPath(dir, topic)
	f, err := os.Open(path)
	if err != nil {
		return nil, err // file not found on first run — normal
	}
	defer f.Close()
	var length uint32
	if err := binary.Read(f, binary.LittleEndian, &length); err != nil {
		return nil, err
	}
	if length > 64*1024*1024 {
		return nil, fmt.Errorf("persist: payload %d bytes exceeds 64 MiB cap", length)
	}
	buf := make([]byte, length)
	if _, err := f.Read(buf); err != nil {
		return nil, err
	}
	return buf, nil
}

// persistFlush writes payload to the topic's file in dir, replacing any
// previous content. A write failure is silently ignored.
func persistFlush(dir, topic string, payload []byte) {
	if dir == "" {
		return
	}
	path := persistPath(dir, topic)
	f, err := os.Create(path)
	if err != nil {
		return
	}
	defer f.Close()
	var length [4]byte
	binary.LittleEndian.PutUint32(length[:], uint32(len(payload)))
	_, _ = f.Write(length[:])
	_, _ = f.Write(payload)
}

// persistPath returns the file path for a topic inside dir.
// Topic slashes are replaced with '_' so the name is a single flat file.
func persistPath(dir, topic string) string {
	safe := strings.NewReplacer("/", "_", "\\", "_", ":", "_").Replace(topic)
	return filepath.Join(dir, "topic-"+safe+".bin")
}
