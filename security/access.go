// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package security

//fusa:req REQ-SEC-013

import "path"

// Permission is a bitfield of allowed operations on a topic.
type Permission uint8

const (
	// PermRead grants read access (subscribe) to a topic.
	PermRead Permission = 1 << 0
	// PermWrite grants write access (publish) to a topic.
	PermWrite Permission = 1 << 1
	// PermReadWrite grants both read and write access.
	PermReadWrite = PermRead | PermWrite
)

// Rule pairs a topic pattern with the permissions it grants.
//
// Pattern follows the semantics of [path.Match]:
//   - '*' matches any sequence of non-'/' characters (one path segment)
//   - '?' matches any single non-'/' character
//   - '[abc]' matches a character class
//   - Exact strings match literally
//
// Examples:
//
//	Rule{Pattern: "vehicle/speed",  Allow: PermRead}         // exact match, read-only
//	Rule{Pattern: "vehicle/*",      Allow: PermReadWrite}     // any single-segment child
//	Rule{Pattern: "*",              Allow: PermRead}          // any top-level topic
type Rule struct {
	Pattern string
	Allow   Permission
}

// AccessPolicy enforces topic-level read/write permissions.
// Rules are evaluated in order; the first matching rule wins.
// A topic that matches no rule is denied all access.
type AccessPolicy struct {
	rules []Rule
}

// NewAccessPolicy creates an AccessPolicy from the given rules.
// Rules are evaluated in declaration order — the first match wins.
func NewAccessPolicy(rules ...Rule) *AccessPolicy {
	r := make([]Rule, len(rules))
	copy(r, rules)
	return &AccessPolicy{rules: r}
}

// CanRead returns true if any rule grants PermRead on topic.
func (p *AccessPolicy) CanRead(topic string) bool {
	return p.allows(topic, PermRead)
}

// CanWrite returns true if any rule grants PermWrite on topic.
func (p *AccessPolicy) CanWrite(topic string) bool {
	return p.allows(topic, PermWrite)
}

func (p *AccessPolicy) allows(topic string, perm Permission) bool {
	for _, r := range p.rules {
		matched, err := path.Match(r.Pattern, topic)
		if err != nil {
			continue // malformed pattern — skip
		}
		if matched {
			return r.Allow&perm != 0
		}
	}
	return false
}
