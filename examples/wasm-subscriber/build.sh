#!/usr/bin/env bash
# Copyright (c) 2026 Matt Jones. All rights reserved.
# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this
# file, You can obtain one at http://mozilla.org/MPL/2.0/.
#
# Builds wasm-subscriber's main.wasm (GOOS=js GOARCH=wasm) and copies the
# matching wasm_exec.js glue out of the local Go toolchain. wasm_exec.js is
# deliberately not vendored into the repo: its content is tied to the exact
# Go version used to build main.wasm (its job is bridging the Go scheduler,
# GC, and syscall/js runtime into the browser, which changes between Go
# releases), so copying it fresh from `go env GOROOT` at build time is the
# only way to guarantee the two stay in lock-step. See README.md.
set -euo pipefail
cd "$(dirname "$0")"

echo "Building main.wasm (GOOS=js GOARCH=wasm)..."
GOOS=js GOARCH=wasm go build -o main.wasm .

goroot="$(go env GOROOT)"
wasm_exec="$(find "$goroot" -name wasm_exec.js 2>/dev/null | head -1)"
if [ -z "$wasm_exec" ]; then
  echo "error: wasm_exec.js not found under GOROOT ($goroot) — is this Go toolchain missing its js/wasm support files?" >&2
  exit 1
fi
cp "$wasm_exec" wasm_exec.js
echo "Copied $wasm_exec -> wasm_exec.js"

echo
echo "Built. Serve this directory over HTTP (not file://) and open index.html, e.g.:"
echo "  python3 -m http.server 8081"
echo "then browse to http://localhost:8081/ — see README.md for the ?ws=/?domain=/?topic= query parameters."
