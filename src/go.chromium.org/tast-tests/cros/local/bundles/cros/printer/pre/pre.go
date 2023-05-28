// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package pre contains preconditions for printing tests.
package pre

import (
	"go.chromium.org/tast/core/testing/hwdep"
)

var unstableModels = []string{
	"yaviks", // Problems with debugd.  TODO(b/257070388): Remove when migration to printscanmgr is complete.
	"nereid", // Problems with debugd.  TODO(b/257070388): Remove when migration to printscanmgr is complete.
}

// PrinterSkipUnstableModels contains a list of models that should be skipped for
// printer testing due to various stability issues.
var PrinterSkipUnstableModels = hwdep.SkipOnModel(unstableModels...)
