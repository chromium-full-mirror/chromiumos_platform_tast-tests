// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package croshealthd

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"chromiumos/tast/common/testexec"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/shutil"
	"go.chromium.org/tast/core/testing"
)

// List of cros_healthd diagnostic routines.
const (
	RoutineMemoryV2    string = "memory_v2"
	RoutineCPUStressV2 string = "cpu_stress_v2"
)

// RoutineResultV2 contains the progress of the routine as a percentage and
// the routine status.
type RoutineResultV2 struct {
	Progress int
	Status   string
	Output   string
}

// RoutineParamsV2 are different configuration options for running a diagnostic
// routine.
type RoutineParamsV2 struct {
	Routine string // The name of the routine to run.
}

// RunDiagRoutineV2 runs the specified routine based on `params`. Returns a
// RoutineResult on success or an error.
func RunDiagRoutineV2(ctx context.Context, params RoutineParamsV2) (*RoutineResultV2, error) {
	diagParams := []string{"--action=run_routine", "--single_line_json", fmt.Sprintf("--routine=%s", params.Routine)}
	if params.Routine == RoutineMemoryV2 {
		// 15000 KiB runs for about 3 seconds on a volteer machine.
		diagParams = append(diagParams, "--max_testing_mem_kib=15000")
	} else if params.Routine == RoutineCPUStressV2 {
		// Runs the routine for 1 seconds.
		diagParams = append(diagParams, "--cpu_stress_length_seconds=1")
	}
	var output string
	var err error
	output, err = runDiagV2(ctx, diagParams)
	if err != nil {
		return nil, err
	}
	return parseDiagOutputV2(ctx, output)
}

// runDiagV2 is a helper function that runs the cros_healthd diag command and
// returns the raw stdout on success, or an error.
func runDiagV2(ctx context.Context, args []string) (string, error) {
	args = append([]string{"diag"}, args...)
	cmd := testexec.CommandContext(ctx, "cros-health-tool", args...)
	testing.ContextLogf(ctx, "Running %q", shutil.EscapeSlice(cmd.Args))
	out, err := cmd.Output()
	if err != nil {
		cmd.DumpLog(ctx)
		return "", errors.Wrapf(err, "failed to run %q", shutil.EscapeSlice(cmd.Args))
	}
	return string(out), nil
}

// parseDiagOutputV2 is a helper function that takes the `raw` output from running a
// diagnostic V2 routine and returns a RoutineResult on success, or an error.
//
// Some examples for `raw`:
// "\rRunning Progress: 0\rWaiting: kWaitingToBeScheduled\n\rRunning Progress: 0\rRunning Progress: 1\rRunning Progress: 2\rRunning Progress: 98\rRunning Progress: 99\rRunning Progress: 100\nStatus: Passed\n"
func parseDiagOutputV2(ctx context.Context, raw string) (*RoutineResultV2, error) {
	status := ""
	progress := 0
	output := ""
	re := regexp.MustCompile(`([^:]+): (.*)`)
	testing.ContextLog(ctx, raw)

	// Treat both \n and \r as separators since the purpose of \r is to make the
	// output more readable in a terminal.
	for _, line := range regexp.MustCompile(`(\n|\r)`).Split(raw, -1) {
		match := re.FindStringSubmatch(line)
		if match == nil {
			continue
		}

		key := match[1]
		value := strings.TrimSpace(match[2])
		switch key {
		case "Status":
			status = value
		case "Running Progress":
			i, err := strconv.Atoi(value)
			if err != nil {
				return nil, errors.Wrapf(err, "Unable to parse Progress value %q as int", value)
			}
			// Override the old value because only the last progress will be reported.
			progress = i
		case "Output":
			output = value
		case "Error":
			status = StatusError
			output = value
		}
	}
	return &RoutineResultV2{progress, status, output}, nil
}
