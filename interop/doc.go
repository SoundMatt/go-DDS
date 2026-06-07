// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package interop contains RTPS wire-compatibility tests that require a live
// CycloneDDS peer. Tests are gated behind the "interop" build tag so they
// are not included in the normal CI run.
//
// See interop_test.go for prerequisites and quick-start instructions.
package interop
