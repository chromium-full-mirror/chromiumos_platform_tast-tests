// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package vm

import (
	"bytes"
	"context"

	"go.chromium.org/tast-tests/cros/common/testexec"
)

const (
	bruschettaVMName  = "bru"
	fakeContainerName = "penguin"
)

// BruschettaVM implements vm.Guest
type BruschettaVM struct {
	VM *VM
}

// Command runs a command in a Bruschetta VM.
func (bru *BruschettaVM) Command(ctx context.Context, vshArgs ...string) *testexec.Cmd {
	args := append([]string{"--vm_name=" + bruschettaVMName, "--owner_id=" + bru.VM.Concierge.ownerID, "--target_container=" + fakeContainerName, "--"}, vshArgs...)
	cmd := testexec.CommandContext(ctx, "vsh", args...)
	// Add an empty buffer for stdin to force allocating a pipe. vsh uses
	// epoll internally and generates a warning (EPERM) if stdin is /dev/null.
	cmd.Stdin = &bytes.Buffer{}
	return cmd
}
