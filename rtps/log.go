// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package rtps

import (
	"fmt"
	"log/slog"
)

// plog wraps an optional *slog.Logger. When log is nil all methods are no-ops.
type plog struct{ l *slog.Logger }

func (p plog) debug(msg string, args ...any) {
	if p.l != nil {
		p.l.Debug(fmt.Sprintf(msg, args...))
	}
}

func (p plog) info(msg string, args ...any) {
	if p.l != nil {
		p.l.Info(fmt.Sprintf(msg, args...))
	}
}

func (p plog) warn(msg string, args ...any) {
	if p.l != nil {
		p.l.Warn(fmt.Sprintf(msg, args...))
	}
}
