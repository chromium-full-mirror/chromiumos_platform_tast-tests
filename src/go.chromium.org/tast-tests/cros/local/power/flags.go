// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import "go.chromium.org/tast/core/testing"

// pdashNoteVar is a runtime variable that annotates results on power dashboard
var pdashNoteVar = testing.RegisterVarString(
	"power.pdash_note",
	"",
	`A string note to annotate results on power dashboard`,
)
