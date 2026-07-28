// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package interop contains RTPS wire-compatibility tests that require a live
// peer: a CycloneDDS container (interop_test.go) or a ROS 2 Jazzy/Rolling
// container (ros2_test.go, Milestone 17 "ROS 2 / rmw Compatibility"). Tests
// are gated behind the "interop" build tag so they are not included in the
// normal CI run.
//
// See interop_test.go and ros2_test.go for prerequisites and quick-start
// instructions.
package interop
