// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package ws

// RFC 6455 §1.3 opening-handshake helpers, shared by ws.go's ServeHTTP.

import (
	"crypto/sha1" //nolint:gosec // RFC 6455 mandates SHA-1 here; not a security use
	"encoding/base64"
	"io"
	"strings"
)

// magicGUID is the fixed GUID RFC 6455 §1.3 defines for computing
// Sec-WebSocket-Accept from a client's Sec-WebSocket-Key.
const magicGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

// acceptKey computes the Sec-WebSocket-Accept value for a client's
// Sec-WebSocket-Key.
func acceptKey(key string) string {
	h := sha1.New()
	_, _ = io.WriteString(h, key+magicGUID)
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// headerContainsToken reports whether header — a comma-separated list per
// RFC 7230 §7 — contains token, case-insensitively.
func headerContainsToken(header, token string) bool {
	for _, part := range strings.Split(header, ",") {
		if strings.EqualFold(strings.TrimSpace(part), token) {
			return true
		}
	}
	return false
}
