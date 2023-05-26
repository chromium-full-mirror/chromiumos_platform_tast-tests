// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package crostini

import (
	"context"

	"chromiumos/tast/local/vm"
	"go.chromium.org/tast-tests/cros/common/testexec"
)

// BasicCommandWorks executes a command in the container and returns
// an error if it fails.
func BasicCommandWorks(ctx context.Context, cont *vm.Container) error {
	return cont.Command(ctx, "pwd").Run(testexec.DumpLogOnError)
}
