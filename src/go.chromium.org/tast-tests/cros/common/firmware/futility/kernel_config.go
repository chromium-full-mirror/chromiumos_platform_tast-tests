// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package futility

import (
	"context"
	"fmt"
	"strings"

	"go.chromium.org/tast/core/errors"
)

// DumpKernelConfig reads kernel config from selected partition.
//
// kernelPartition is required.
// kloadaddr is optional. Use ^uint64(0) as default, as zero is a valid value.
//
// On success returns kernel config as string. Also returns program output and error on failure.
func (i *Instance) DumpKernelConfig(ctx context.Context, kernelPartition string, kloadaddr uint64) (string, []byte, error) {
	if kernelPartition == "" {
		return "", nil, errors.New("cannot dump kernel config: kernelPartition is empty")
	}

	cmdArgs := append(i.futilityCmdArgs(), "dump_kernel_config")
	if kloadaddr != ^uint64(0) {
		cmdArgs = append(cmdArgs, "--kloadaddr", fmt.Sprintf("%#x", kloadaddr))
	}
	cmdArgs = append(cmdArgs, kernelPartition)

	stdout, stderr, err := i.runCommandLine(ctx, cmdArgs)
	if err != nil {
		return "", joinProgramOutputs(stdout, stderr), errors.Wrapf(err, "failed to dump kernel config with arguments: %v", cmdArgs)
	}

	return strings.TrimSpace(string(stdout)), joinProgramOutputs(stdout, stderr), nil
}
