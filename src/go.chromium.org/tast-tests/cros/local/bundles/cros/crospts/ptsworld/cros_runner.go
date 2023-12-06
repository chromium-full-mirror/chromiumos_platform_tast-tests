// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ptsworld

import (
	"bufio"
	"context"
	"fmt"
	"io"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/testing"
)

// CrosRunner is a crospts runner for cros.
type CrosRunner struct {
	// ptsType is the type of crospts.
	ptsType PtsType
}

// NewCrosRunner creates a new CrosRunner.
func NewCrosRunner() *CrosRunner {
	return &CrosRunner{
		ptsType: TypeCros,
	}
}

// PtsWorldShell runs the given command in the PTSWorld.
func (r *CrosRunner) PtsWorldShell(ctx context.Context, s *testing.State, cmd string) {
	testing.ContextLog(ctx, "Running chroot shell: ", cmd)
	ptsWorldCmd := testexec.CommandContext(ctx, "env", "-i", "/usr/bin/chroot", CrosChroot, "/usr/bin/bash", "-c", cmd)
	stdout, err := ptsWorldCmd.StdoutPipe()
	if err != nil {
		s.Fatal("Failed to get stdout pipe: ", err)
	}
	stderr, err := ptsWorldCmd.StderrPipe()
	if err != nil {
		s.Fatal("Failed to get stderr pipe: ", err)
	}

	if err := ptsWorldCmd.Start(); err != nil {
		s.Fatal("Failed to start PTSWorld shell command: ", err)
	}

	// Log stdout and stderr in real-time.
	multi := io.MultiReader(stdout, stderr)
	in := bufio.NewScanner(multi)
	for in.Scan() {
		s.Log(in.Text())
	}
	if err := in.Err(); err != nil {
		s.Fatal("Failed to scan stdout and stderr: ", err)
	}

	if err := ptsWorldCmd.Wait(); err != nil {
		s.Fatal("Failed to wait for PTSWorld shell command: ", err)
	}
}

// RunTestSuite runs the given test suite in PTSWorld.
func (r *CrosRunner) RunTestSuite(ctx context.Context, s *testing.State, suiteName string) {
	const (
		// NO_DOWNLOAD_CACHE, NO_EXTERNAL_DEPENDENCIES and NO_FILE_HASH_CHECKS are
		// set for running the test by local installed tests.

		// The benchmark tool may be reinstalled which caused by PTS to check
		// the system_id_string. The system_id_string is composed by cpu model,
		// operating-system and compiler. See
		// https://github.com/phoronix-test-suite/phoronix-test-suite/blob/v10.8.4/pts-core/objects/phodevi/phodevi.php#L461
		// The reinstallation frequently failed due the permission denied error
		// which caused by PTS compiler mask.
		// NO_COMPILER_MASK is to disable PTS compiler mask and workaround the
		// installation error. The PTS compiler mask is used to log the compiler
		// logs. It finds the system compiler, i.e. CC, CXX...etc, and creates a
		// wrapper script file in /tmp/<benchmark_compiler_mask_temp_dir>/cc,
		// but the wrapper file sometime not set to right permission or did not
		// find the CC path causes the installation error.
		// Added the flag will use the benchmark tool installer without PTS
		// compiler mask.
		ptsFlags = "NO_DOWNLOAD_CACHE=TRUE NO_EXTERNAL_DEPENDENCIES=TRUE NO_FILE_HASH_CHECKS=TRUE NO_COMPILER_MASK=TRUE"
		// ptsBatchRun is the batch mode test command which has no user interaction.
		ptsBatchRun = "/phoronix-test-suite/phoronix-test-suite batch-benchmark"
	)
	r.PtsWorldShell(ctx, s, fmt.Sprintf("%s %s %s", ptsFlags, ptsBatchRun, suiteName))
}
