// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package tcpdump

import (
	"chromiumos/tast/local/network/cmd"
	"go.chromium.org/tast-tests/cros/common/network/tcpdump"
)

// Runner is an alias for common tcpdump Runner but only for local execution.
type Runner = tcpdump.Runner

// NewLocalRunner creates an tcpdump runner for local execution.
func NewLocalRunner() *Runner {
	return tcpdump.NewRunner(&cmd.LocalCmdRunner{})
}
