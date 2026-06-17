// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Runnable examples that back the security snippets in README.md. Keeping them
// as Example functions means `go test` fails if the public API drifts from the
// documented signatures (issue #65).

package security_test

import (
	"fmt"
	"time"

	"github.com/SoundMatt/go-DDS/security"
)

// ExampleAccessPolicy mirrors the README topic-ACL helper snippet.
func ExampleAccessPolicy() {
	policy := security.NewAccessPolicy(
		security.Rule{Pattern: "vehicle/speed", Allow: security.PermWrite},
		security.Rule{Pattern: "vehicle/*", Allow: security.PermRead},
	)
	fmt.Println(policy.CanWrite("vehicle/speed"))
	fmt.Println(policy.CanRead("vehicle/status"))
	fmt.Println(policy.CanWrite("vehicle/status"))
	// Output:
	// true
	// true
	// false
}

// ExampleReplayGuard mirrors the README anti-replay helper snippet.
func ExampleReplayGuard() {
	guard := security.NewReplayGuard(5 * time.Second)
	now := time.Now()

	first := guard.Check(1, now) // fresh
	dup := guard.Check(1, now)   // duplicate sequence
	fmt.Println(first == nil, dup == nil)
	// Output:
	// true false
}
