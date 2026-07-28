// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// A small, dependency-free base64 codec, implemented against nothing but
// Uint8Array/string so it behaves identically in a browser, Node.js, and
// edge/Workers runtimes — deliberately not using `Buffer` (Node-only) or
// `btoa`/`atob` (browser-only, and lossy for non-Latin1 byte values), the
// same portability goal the rest of this package holds itself to (see
// README.md's "Runtime support" section).

const B64_CHARS = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";

const B64_LOOKUP: Record<string, number> = (() => {
  const map: Record<string, number> = {};
  for (let i = 0; i < B64_CHARS.length; i++) {
    map[B64_CHARS[i]!] = i;
  }
  return map;
})();

/** encodeBase64 encodes bytes as standard (RFC 4648 §4) base64 with padding. */
/** char returns the base64 alphabet character at index n (0-63); n is always in range by construction at every call site below. */
function char(n: number): string {
  return B64_CHARS[n & 63]!;
}

export function encodeBase64(bytes: Uint8Array): string {
  let out = "";
  let i = 0;
  for (; i + 3 <= bytes.length; i += 3) {
    const n = (bytes[i]! << 16) | (bytes[i + 1]! << 8) | bytes[i + 2]!;
    out += char(n >> 18) + char(n >> 12) + char(n >> 6) + char(n);
  }
  const remaining = bytes.length - i;
  if (remaining === 1) {
    const n = bytes[i]! << 16;
    out += char(n >> 18) + char(n >> 12) + "==";
  } else if (remaining === 2) {
    const n = (bytes[i]! << 16) | (bytes[i + 1]! << 8);
    out += char(n >> 18) + char(n >> 12) + char(n >> 6) + "=";
  }
  return out;
}

/**
 * decodeBase64 decodes standard base64 (padded or not; whitespace and any
 * character outside the base64 alphabet is skipped rather than rejected,
 * matching the tolerant-decoder convention most base64 implementations —
 * including Go's encoding/base64.StdEncoding on the go-DDS bridge/ws server
 * side this client talks to — already follow for received data).
 */
export function decodeBase64(s: string): Uint8Array {
  const bytes: number[] = [];
  let buffer = 0;
  let bits = 0;
  for (let i = 0; i < s.length; i++) {
    const ch = s[i]!;
    if (ch === "=") break;
    const val = B64_LOOKUP[ch];
    if (val === undefined) continue;
    buffer = (buffer << 6) | val;
    bits += 6;
    if (bits >= 8) {
      bits -= 8;
      bytes.push((buffer >> bits) & 0xff);
    }
  }
  return new Uint8Array(bytes);
}
