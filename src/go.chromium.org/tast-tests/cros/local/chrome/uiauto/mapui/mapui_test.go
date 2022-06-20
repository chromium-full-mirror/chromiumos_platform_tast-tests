// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package mapui

import (
	"testing"
)

func TestImport(t *testing.T) {
	// The importing is the testing itself.
	// When the mapui package is imported, all the node definitions are loaded.
	// If there is an error in a node definition, mapui.nodeFromStringVar will panic.
	// It only checks the default node values, not the overwrites added as CLI flags.
}
