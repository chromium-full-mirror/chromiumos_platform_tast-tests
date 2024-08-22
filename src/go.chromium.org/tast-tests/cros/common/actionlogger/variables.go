// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package actionlogger contains the variables to be used by local and remote.
package actionlogger

import "go.chromium.org/tast/core/testing"

// ShouldRun is the used for the actionlogger hook and library.
var ShouldRun = testing.RegisterVarString(
	"actionlogger.shouldRun",
	"",
	"A variable to decide whether the action logger should run",
)
