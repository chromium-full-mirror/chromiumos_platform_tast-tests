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
		ptsFlags = "NO_DOWNLOAD_CACHE=TRUE NO_EXTERNAL_DEPENDENCIES=TRUE NO_FILE_HASH_CHECKS=TRUE"
		// ptsBatchRun is the batch mode test command which has no user interaction.
		ptsBatchRun = "/phoronix-test-suite/phoronix-test-suite batch-benchmark"
	)
	r.PtsWorldShell(ctx, s, fmt.Sprintf("%s %s %s", ptsFlags, ptsBatchRun, suiteName))

	// TODO(darrenwu): Add test results xml format convert to results-chart.json
}
