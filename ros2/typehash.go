// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package ros2

import (
	"crypto/sha256"
	"encoding/hex"
)

// TypeHashPrefix mirrors the version tag ROS 2's own RIHS (ROS Interface
// Hashing Scheme, REP 2011) uses for its type-hash strings ("rihs01_...").
// go-DDS does not have access to a message's full rosidl field-description
// tree — only whatever type name and (optionally) a caller-supplied field
// descriptor string it is given — so TypeHash below is NOT bit-for-bit
// identical to a real ROS 2 toolchain's RIHS hash of the same message type;
// it is go-DDS's own best-effort content hash, scoped honestly the same way
// this milestone's other cross-vendor gaps are (see the ros2 package doc
// comment). Two go-DDS ros2.Participants publishing/subscribing the same
// (typeName, fieldDescriptor) pair always agree, which is what this
// library can independently verify; agreement with a specific upstream
// rosidl-generated hash for the same nominal message is not.
const TypeHashPrefix = "gorihs01_"

// TypeHash computes a stable content hash for a DDS/ROS 2 type, given its
// type name (see TypeSupportName) and an optional field descriptor — a
// caller-defined canonical string describing the message's fields (e.g.
// "int32 x;int32 y;string label"). An empty fieldDescriptor still yields a
// stable, type-name-only hash — useful when the caller has no field
// description available, at the cost of not detecting a same-named type
// whose fields changed. The result is always TypeHashPrefix followed by 64
// lowercase hex characters (SHA-256).
func TypeHash(typeName, fieldDescriptor string) string {
	h := sha256.Sum256([]byte(typeName + "\x00" + fieldDescriptor))
	return TypeHashPrefix + hex.EncodeToString(h[:])
}
