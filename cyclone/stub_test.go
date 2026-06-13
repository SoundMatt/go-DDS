//go:build !cyclone

// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package cyclone_test

//fusa:test REQ-CYCLONE-001

import (
	"testing"
	"time"

	dds "github.com/SoundMatt/go-DDS"
	"github.com/SoundMatt/go-DDS/cyclone"
)

// TestStub_New verifies the stub returns an informative error when the cyclone
// build tag is absent. Prevents silent nil-pointer dereference at the call site.
func TestStub_New(t *testing.T) {
	p, err := cyclone.New(dds.Domain(0))
	if err == nil {
		_ = p.Close()
		t.Fatal("cyclone.New should return an error without -tags cyclone")
	}
}

// TestStub_NewWithOptions verifies the same for the options constructor.
func TestStub_NewWithOptions(t *testing.T) {
	p, err := cyclone.NewWithOptions(dds.Domain(0), cyclone.Options{PollInterval: time.Millisecond})
	if err == nil {
		_ = p.Close()
		t.Fatal("cyclone.NewWithOptions should return an error without -tags cyclone")
	}
}
