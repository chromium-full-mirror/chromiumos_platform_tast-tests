// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package iw contains utility functions to wrap around the iw program.
package iw

import (
	"chromiumos/tast/local/network/cmd"
	"go.chromium.org/tast-tests/cros/common/network/iw"
)

// Runner is an alias for common iw Runner but only for local execution.
type Runner = iw.Runner

// NewLocalRunner creates an iw runner for local execution.
func NewLocalRunner() *Runner {
	return iw.NewRunner(&cmd.LocalCmdRunner{})
}
