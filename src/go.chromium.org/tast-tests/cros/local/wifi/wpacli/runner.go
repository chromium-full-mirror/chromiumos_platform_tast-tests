// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package wpacli contains utility functions to wrap around the wpacli program.
package wpacli

import (
	"go.chromium.org/tast-tests/cros/common/wifi/wpacli"
	"go.chromium.org/tast-tests/cros/local/network/cmd"
)

// Runner is an alias for common wpacli Runner but only for local execution.
type Runner = wpacli.Runner

// NewLocalRunner creates an wpacli runner for local execution.
func NewLocalRunner() *Runner {
	return wpacli.NewRunner(&cmd.LocalCmdRunner{})
}

// NewLocalRunnerOnIface creates an wpacli runner for local execution in the specific interface context.
func NewLocalRunnerOnIface(i string) *Runner {
	return wpacli.NewRunnerOnIface(&cmd.LocalCmdRunner{}, i)
}
