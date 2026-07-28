// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package ros2

import (
	"fmt"
	"regexp"
	"strings"
)

// nameSegmentRE matches a single valid ROS 2 node-name / namespace-segment
// token, per the "Topic and Service name mapping to DDS" design article:
// it must start with an alpha character or underscore, and contain only
// alphanumerics and underscores thereafter.
var nameSegmentRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// ValidNodeName reports whether name is a valid ROS 2 base node name (no
// slashes, matching nameSegmentRE).
func ValidNodeName(name string) bool {
	return nameSegmentRE.MatchString(name)
}

// ValidNamespace reports whether ns is a valid ROS 2 node namespace: either
// "" (equivalent to the root namespace "/") or a "/"-separated path of
// nameSegmentRE segments starting with "/" and not ending with "/" (except
// the root namespace "/" itself).
func ValidNamespace(ns string) bool {
	if ns == "" || ns == "/" {
		return true
	}
	if !strings.HasPrefix(ns, "/") || strings.HasSuffix(ns, "/") {
		return false
	}
	for _, seg := range strings.Split(ns[1:], "/") {
		if !nameSegmentRE.MatchString(seg) {
			return false
		}
	}
	return true
}

// FullyQualifiedName joins a ROS 2 namespace and a node (or topic) name
// into its fully-qualified form, e.g. FullyQualifiedName("/robot1", "camera")
// == "/robot1/camera", and FullyQualifiedName("/", "camera") == "/camera".
// If name is already absolute (starts with "/") it is returned unchanged —
// matching ROS 2's own name-resolution rule that an absolute topic/service
// name is never combined with the node's namespace. It does not itself
// validate ns or name — call ValidNamespace/ValidNodeName first if that
// matters to the caller.
func FullyQualifiedName(ns, name string) string {
	if strings.HasPrefix(name, "/") {
		return name
	}
	if ns == "" || ns == "/" {
		return "/" + name
	}
	return ns + "/" + name
}

// ROS 2's builtin discovery-graph topic name (rmw_dds_common). Unlike
// ordinary user topics, this name is never mangled with the "rt" prefix —
// every conformant rmw implementation (rmw_fastrtps, rmw_cyclonedds) uses
// it verbatim on the wire so that vendors' discovery/graph tooling
// interoperates regardless of which rmw produced it.
const DiscoveryTopicName = "ros_discovery_info"

// Wire-name prefixes ROS 2 prepends to a fully-qualified name before using
// it as the underlying DDS topic name (REP 2003 / the "Topic and Service
// name mapping to DDS" design doc). Only the plain-topic prefix is used by
// this milestone; the request/reply prefixes are recorded here for the
// Action Server Pattern sub-phase (ROADMAP.md Milestone 17) to reuse.
const (
	topicPrefix   = "rt"
	requestPrefix = "rq"
	replyPrefix   = "rr"
)

// ToDDSTopicName converts a fully-qualified ROS 2 topic name (e.g.
// "/chatter") to the mangled DDS wire topic name a ROS 2 rmw participant
// actually publishes/subscribes on (e.g. "rt/chatter"). rosTopic must be
// fully-qualified (start with "/"); relative names should be resolved with
// FullyQualifiedName first.
func ToDDSTopicName(rosTopic string) string {
	return topicPrefix + rosTopic
}

// FromDDSTopicName reverses ToDDSTopicName: given a raw DDS topic name seen
// over SEDP, it returns the fully-qualified ROS 2 topic name and true if
// ddsTopic carries the "rt" ROS 2 user-topic prefix, or ("", false)
// otherwise (e.g. a plain go-DDS topic with no ROS 2 participant behind
// it, or one of ROS 2's own "rq"/"rr" service-wire topics).
func FromDDSTopicName(ddsTopic string) (rosTopic string, ok bool) {
	if !strings.HasPrefix(ddsTopic, topicPrefix) {
		return "", false
	}
	rest := ddsTopic[len(topicPrefix):]
	if !strings.HasPrefix(rest, "/") {
		return "", false
	}
	return rest, true
}

// TypeSupportName builds the DDS type name a ROS 2 rmw implementation
// (rmw_fastrtps, rmw_cyclonedds) announces for a message type, e.g.
// TypeSupportName("std_msgs", "msg", "String") == "std_msgs::msg::dds_::String_".
// subfolder is conventionally "msg" or "srv". This is the exact
// "TypeSupport name" both rmw implementations have used since ROS 2
// Dashing, still current in Jazzy and Rolling.
func TypeSupportName(pkg, subfolder, msgType string) string {
	return fmt.Sprintf("%s::%s::dds_::%s_", pkg, subfolder, msgType)
}

// typeSupportNameRE parses the TypeSupportName format back into its parts.
var typeSupportNameRE = regexp.MustCompile(`^([^:]+)::([^:]+)::dds_::(.+)_$`)

// ParseTypeSupportName reverses TypeSupportName. ok is false if typeName
// does not match the "pkg::subfolder::dds_::Type_" shape (e.g. go-DDS's own
// default "CDR_BLOB" sentinel, or a non-ROS-2 vendor's type name).
func ParseTypeSupportName(typeName string) (pkg, subfolder, msgType string, ok bool) {
	m := typeSupportNameRE.FindStringSubmatch(typeName)
	if m == nil {
		return "", "", "", false
	}
	return m[1], m[2], m[3], true
}
