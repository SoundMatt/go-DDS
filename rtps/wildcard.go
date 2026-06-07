// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package rtps

// TopicMatches reports whether pattern (which may contain MQTT-style + and #
// wildcards) matches the concrete topic name.
//
// Rules:
//   - '+' matches exactly one topic level (no slashes).
//   - '#' at the end of a segment matches zero or more remaining levels.
//   - Literal segments must match exactly (case-sensitive).
func TopicMatches(pattern, topic string) bool {
	return matchSegments(pattern, topic)
}

func matchSegments(pat, top string) bool {
	for {
		if pat == "" {
			return top == ""
		}
		pi := indexByte(pat, '/')
		var pseg string
		if pi < 0 {
			pseg, pat = pat, ""
		} else {
			pseg, pat = pat[:pi], pat[pi+1:]
		}

		if pseg == "#" {
			return true
		}

		if top == "" {
			return false
		}
		ti := indexByte(top, '/')
		var tseg string
		if ti < 0 {
			tseg, top = top, ""
		} else {
			tseg, top = top[:ti], top[ti+1:]
		}

		if pseg != "+" && pseg != tseg {
			return false
		}
	}
}

func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}
