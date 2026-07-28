// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package ros2 makes a go-DDS participant a first-class participant in a
// ROS 2 graph (ROADMAP.md Milestone 17, "ROS 2 / rmw Compatibility").
//
// NewROS2Participant wraps an rtps.Participant — the same wire-compatible
// RTPS implementation the interop package already proves against a live
// CycloneDDS peer — with the three additional conventions a ROS 2 rmw
// implementation (rmw_fastrtps, rmw_cyclonedds) expects of any DDS
// participant that wants to appear as a ROS 2 node:
//
//  1. Topic name mangling: a ROS 2 topic's DDS wire name is "rt" prefixed
//     onto its fully-qualified name (see ToDDSTopicName/FromDDSTopicName),
//     and its DDS type name follows the "pkg::msg::dds_::Type_" TypeSupport
//     convention (see TypeSupportName) — both are exactly what
//     rmw_fastrtps_cpp / rmw_cyclonedds_cpp emit today, unchanged since
//     ROS 2 Dashing and still current in Jazzy/Rolling.
//  2. Graph introspection: every conformant rmw publishes a
//     rmw_dds_common/msg/ParticipantEntitiesInfo sample, TRANSIENT_LOCAL +
//     RELIABLE, on the well-known "ros_discovery_info" topic whenever its
//     local node/endpoint set changes (see graph.go). Nodes/Topics decode
//     every peer's copy of that message the same way `ros2 node list` /
//     `ros2 topic list` do, so a go-DDS process shows up in — and can
//     itself observe — a real ROS 2 graph without a separate bridge
//     process.
//  3. Naming validation: ValidNodeName/ValidNamespace enforce the same
//     node/namespace token rules `rclcpp`/`rclpy` enforce client-side, so a
//     misconfigured go-DDS node fails fast instead of silently publishing
//     a name no ROS 2 tool will resolve correctly.
//
// # Honest scope
//
// This package targets wire-level compatibility with what ROS 2's rmw
// layer actually puts on the network — topic/type naming and the
// ros_discovery_info graph protocol — not the rclcpp/rclpy client-library
// API surface (parameters, actions, lifecycle nodes, etc.), and not the
// rosidl code generator: go-DDS has no .msg/.srv compiler, so callers
// supply their own type name and (for TypeHash) field descriptor. See
// typehash.go's doc comment for the one area — content-based type hashing
// — where this package's own hash function is a deliberate, clearly-marked
// stand-in for ROS 2's RIHS algorithm rather than a bit-compatible
// reimplementation of it.
package ros2
