// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package archtest is a permanent CI-enforced guardrail for the module
// boundaries described in ROADMAP.md, "Architecture Initiative — Multi-Module
// Repository Split" (#71). It has no production code of its own: it exists
// solely to run TestCoreIsDependencyLeaf as part of the normal `go test
// ./...` suite.
package archtest

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// modulePath is this repo's Go module import path.
const modulePath = "github.com/SoundMatt/go-DDS"

// coreGroups are the top-level packages ROADMAP.md's target layout assigns
// to the future core go.mod: "dds (root), rtps, mock, shmem, auto, pool,
// security". "" denotes the module root package itself.
var coreGroups = []string{"", "rtps", "mock", "shmem", "auto", "pool", "security"}

// peripheryGroups are the top-level packages ROADMAP.md assigns to the
// future bridges/tools/observability/safety go.mods. Any package whose
// import path is one of these, or nested under one of these (e.g.
// bridge/grpc), counts as periphery.
var peripheryGroups = []string{
	"tsn", "safety", "bridge", "idl", "cdr", "xtypes",
	"otel", "monitor", "admin", "services", "record", "cmd/ddstool",
}

// goListPackage is the subset of `go list -json` output this test needs.
// Imports lists only the production (non-test) imports of the package —
// exactly the edges that matter for a go.mod dependency-leaf guarantee.
type goListPackage struct {
	ImportPath string
	Imports    []string
}

// groupImportPath returns the full import path for a top-level group name
// ("" for the module root itself).
func groupImportPath(group string) string {
	if group == "" {
		return modulePath
	}
	return modulePath + "/" + group
}

// inGroup reports whether importPath is exactly one of the given top-level
// groups, or a sub-package of one of them (e.g. "bridge/grpc" is in group
// "bridge"). The module-root group ("") only ever matches the module path
// itself — it must NOT prefix-match, or every package in the repo would
// look like a "sub-package" of the root.
func inGroup(importPath string, groups []string) (group string, ok bool) {
	for _, g := range groups {
		full := groupImportPath(g)
		if importPath == full {
			return g, true
		}
		if g != "" && strings.HasPrefix(importPath, full+"/") {
			return g, true
		}
	}
	return "", false
}

// TestCoreIsDependencyLeaf fails if any proposed "core" package (dds root,
// rtps, mock, shmem, auto, pool, security) imports any proposed "periphery"
// package (tsn, safety, bridge/..., idl, cdr, xtypes, otel, monitor, admin,
// services, record, cmd/ddstool) in production code.
//
// This is the durable form of the one-time Phase 0 fix (rtps no longer
// imports tsn — see rtps/tsn.go, tsn/rtps.go): core must stay a dependency
// leaf so its go.mod never transitively pins a periphery module version,
// which is the entire point of eventually cutting core into its own module
// ahead of v1.0. See ROADMAP.md, "Architecture Initiative", Phase 0.
func TestCoreIsDependencyLeaf(t *testing.T) {
	pkgs := loadPackages(t)

	var violations []string
	for _, pkg := range pkgs {
		coreGroup, isCore := inGroup(pkg.ImportPath, coreGroups)
		if !isCore {
			continue
		}
		for _, imp := range pkg.Imports {
			if periGroup, isPeriphery := inGroup(imp, peripheryGroups); isPeriphery {
				violations = append(violations, "core package \""+pkg.ImportPath+
					"\" (group \""+displayGroup(coreGroup)+"\") imports periphery package \""+
					imp+"\" (group \""+periGroup+"\")")
			}
		}
	}

	if len(violations) > 0 {
		t.Errorf("core is not a dependency leaf — %d violation(s) found:\n  %s\n"+
			"See ROADMAP.md \"Architecture Initiative\" §Phase 0: core (dds root, rtps, "+
			"mock, shmem, auto, pool, security) must never import bridges/tools/"+
			"observability/safety packages in production code. If rtps needs\n"+
			"something from a periphery package, define a small interface in rtps "+
			"capturing only the shape it needs, and move the concrete wiring to the "+
			"periphery package (which may import rtps — see tsn/rtps.go for the "+
			"pattern).",
			len(violations), strings.Join(violations, "\n  "))
	}
}

// loadPackages runs `go list -json ./...` from the module root and decodes
// the (concatenated, not array-wrapped) JSON package stream it produces.
func loadPackages(t *testing.T) []goListPackage {
	t.Helper()

	moduleDir := moduleRoot(t)

	cmd := exec.CommandContext(t.Context(), "go", "list", "-json", "./...")
	cmd.Dir = moduleDir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("go list -json ./... failed: %v\nstderr:\n%s", err, stderr.String())
	}

	var pkgs []goListPackage
	dec := json.NewDecoder(&stdout)
	for dec.More() {
		var pkg goListPackage
		if err := dec.Decode(&pkg); err != nil {
			t.Fatalf("decoding go list -json output: %v", err)
		}
		pkgs = append(pkgs, pkg)
	}
	if len(pkgs) == 0 {
		t.Fatal("go list -json ./... returned no packages — test harness is broken")
	}
	return pkgs
}

// moduleRoot returns the root directory of the core (root) go-DDS module.
//
// This deliberately uses `go env GOMOD` rather than `go list -m -f
// {{.Dir}}`: once a repo-root `go.work` exists (ROADMAP.md "Architecture
// Initiative" #71, Phase B added one for local multi-module dev), `go list
// -m` with no module argument lists every module in the workspace — one
// line per module (core, bridge, safety, ...) — which broke this test by
// turning the expected single directory into a multi-line string `exec`
// then failed to `chdir` into. `go env GOMOD` instead reports the go.mod
// governing the *current* working directory regardless of workspace mode,
// which is exactly the one archtest itself lives in (the core module).
func moduleRoot(t *testing.T) string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "go", "env", "GOMOD")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("go env GOMOD failed: %v\nstderr:\n%s", err, stderr.String())
	}
	gomod := strings.TrimSpace(stdout.String())
	if gomod == "" {
		t.Fatal("go env GOMOD returned empty — archtest must run inside a Go module")
	}
	return filepath.Dir(gomod)
}

// displayGroup renders the module root group ("") as "." for readability.
func displayGroup(group string) string {
	if group == "" {
		return "."
	}
	return group
}
