// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package main

// Conformance tests for the §12 CLI documents against the embedded RELAY
// v0.3 JSON Schemas (relay.Schema). A full JSON-Schema validator is not pulled
// in as a dependency; instead this checks the two constraints that matter for
// CLI conformance — required fields are present, and (when additionalProperties
// is false) no field outside the schema's declared properties is emitted.

import (
	"encoding/json"
	"testing"

	relay "github.com/SoundMatt/RELAY/v2"
)

// schemaShape is the subset of JSON Schema we enforce here.
type schemaShape struct {
	Properties           map[string]json.RawMessage `json:"properties"`
	Required             []string                   `json:"required"`
	AdditionalProperties *bool                      `json:"additionalProperties"`
}

func loadSchema(t *testing.T, name string) schemaShape {
	t.Helper()
	raw, err := relay.Schema(name)
	if err != nil {
		t.Fatalf("relay.Schema(%q): %v", name, err)
	}
	var s schemaShape
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatalf("unmarshal schema %q: %v", name, err)
	}
	return s
}

func assertConforms(t *testing.T, schemaName string, doc map[string]any) {
	t.Helper()
	s := loadSchema(t, schemaName)

	for _, req := range s.Required {
		if _, ok := doc[req]; !ok {
			t.Errorf("%s: missing required field %q", schemaName, req)
		}
	}
	if s.AdditionalProperties != nil && !*s.AdditionalProperties {
		for k := range doc {
			if _, ok := s.Properties[k]; !ok {
				t.Errorf("%s: field %q is not allowed (additionalProperties=false)", schemaName, k)
			}
		}
	}
}

func TestVersionDoc_Conforms(t *testing.T) {
	assertConforms(t, "cli-version", versionDoc())
}

func TestCapabilitiesDoc_Conforms(t *testing.T) {
	doc := capabilitiesDoc()
	assertConforms(t, "cli-capabilities", doc)

	// commands MUST contain "version", "capabilities", "status" (§12.2).
	cmds, ok := doc["commands"].([]string)
	if !ok {
		t.Fatalf("commands is not []string: %T", doc["commands"])
	}
	for _, want := range []string{"version", "capabilities", "status"} {
		found := false
		for _, c := range cmds {
			if c == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("commands missing required entry %q", want)
		}
	}
}

func TestStatusDoc_Conforms(t *testing.T) {
	assertConforms(t, "cli-status", statusDoc(true))
}
