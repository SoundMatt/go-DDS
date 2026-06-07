// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package rtps

import "strings"

// TopicMatches reports whether pattern (which may contain MQTT-style + and #
// wildcards) matches the concrete topic name.
//
// Rules:
//   - '+' matches exactly one topic level (no slashes).
//   - '#' at the end of a segment matches zero or more remaining levels.
//   - Literal segments must match exactly (case-sensitive).
//   - "foo/" and "foo" are distinct topics (two levels vs one level).
func TopicMatches(pattern, topic string) bool {
	return matchSlices(strings.Split(pattern, "/"), strings.Split(topic, "/"))
}

func matchSlices(pSegs, tSegs []string) bool {
	if len(pSegs) == 0 {
		return len(tSegs) == 0
	}
	if pSegs[0] == "#" {
		return true
	}
	if len(tSegs) == 0 {
		return false
	}
	if pSegs[0] == "+" || pSegs[0] == tSegs[0] {
		return matchSlices(pSegs[1:], tSegs[1:])
	}
	return false
}
