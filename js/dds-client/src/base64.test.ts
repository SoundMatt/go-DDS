// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

import assert from "node:assert/strict";
import { test } from "node:test";
import { decodeBase64, encodeBase64 } from "./base64.js";

test("encodeBase64/decodeBase64 round-trip: empty", () => {
  const bytes = new Uint8Array([]);
  assert.equal(encodeBase64(bytes), "");
  assert.deepEqual(decodeBase64(""), bytes);
});

test("encodeBase64/decodeBase64 round-trip: 1/2/3-byte remainders", () => {
  const cases: number[][] = [
    [1],
    [1, 2],
    [1, 2, 3],
    [1, 2, 3, 4],
    [1, 2, 3, 4, 5],
    [1, 2, 3, 4, 5, 6],
  ];
  for (const c of cases) {
    const bytes = new Uint8Array(c);
    const encoded = encodeBase64(bytes);
    assert.deepEqual(Array.from(decodeBase64(encoded)), c, `round-trip for ${JSON.stringify(c)}`);
  }
});

test("encodeBase64 matches a known vector", () => {
  const bytes = new TextEncoder().encode("hello-ws-bridge");
  // Computed independently (Go's base64.StdEncoding.EncodeToString) to
  // cross-check against the exact encoding bridge/ws's Go server produces.
  assert.equal(encodeBase64(bytes), "aGVsbG8td3MtYnJpZGdl");
});

test("decodeBase64 matches a known vector and ignores padding", () => {
  const got = decodeBase64("aGVsbG8td3MtYnJpZGdl");
  assert.equal(new TextDecoder().decode(got), "hello-ws-bridge");
});

test("encodeBase64/decodeBase64 round-trip: 300 pseudo-random bytes", () => {
  const bytes = new Uint8Array(300);
  for (let i = 0; i < bytes.length; i++) bytes[i] = (i * 37 + 11) % 256;
  const encoded = encodeBase64(bytes);
  const decoded = decodeBase64(encoded);
  assert.deepEqual(Array.from(decoded), Array.from(bytes));
});
