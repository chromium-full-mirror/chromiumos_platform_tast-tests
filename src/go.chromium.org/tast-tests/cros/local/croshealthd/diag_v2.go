// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package croshealthd

import (
	"bytes"
	"context"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/shutil"
	"go.chromium.org/tast/core/testing"
)

// List of cros_healthd diagnostic routines.
const (
	RoutineMemoryV2      string = "memory_v2"
	RoutineCPUStressV2   string = "cpu_stress_v2"
	RoutineAudioDriver   string = "audio_driver"
	RoutineCPUCacheV2    string = "cpu_cache_v2"
	RoutineUFSLifetime   string = "ufs_lifetime"
	RoutinePrimeSearchV2 string = "prime_search_v2"
	RoutineVolumeButton  string = "volume_button"
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
	diagParams := []string{params.Routine, "--single_line_json"}
	switch r := params.Routine; r {
	case RoutineMemoryV2:
		// 15000 KiB runs for about 3 seconds on a volteer machine.
		diagParams = append(diagParams, "--max_testing_mem_kib=15000")
	case RoutineCPUStressV2:
		// Runs the routine for 1 second.
		diagParams = append(diagParams, "--length_seconds=1")
	case RoutineCPUCacheV2:
		// Runs the routine for 1 second.
		diagParams = append(diagParams, "--length_seconds=1")
	case RoutinePrimeSearchV2:
		// Runs the routine for 1 second.
		diagParams = append(diagParams, "--length_seconds=1")
	case RoutineVolumeButton:
		// Runs the routine for 5 second.
		diagParams = append(diagParams, "--length_seconds=5")
		diagParams = append(diagParams, "--button_type=up")
	default:
		// No extra parameters required for the following routines:
		//   - RoutineAudioDriver
		//   - RoutineUFSLifetime
	}
	var output string
	var err error
	if params.Routine == RoutineVolumeButton {
		output, err = runVolumeButtonDiag(ctx, diagParams)
	} else {
		output, err = runDiagV2(ctx, diagParams)
	}
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
	stdout, stderr, err := cmd.SeparatedOutput()
	if err != nil {
		cmd.DumpLog(ctx)
		return "", errors.Wrapf(err, "command failed with stdout: %q, stderr: %q", string(stdout), string(stderr))
	}
	return string(stdout), nil
}

// runVolumeButtonDiag is a helper function similar to `runDiagV2` while simulating the
// volume button event for volume button routine.
func runVolumeButtonDiag(ctx context.Context, args []string) (string, error) {
	kb, err := input.VirtualKeyboard(ctx)
	if err != nil {
		return "", errors.Wrap(err, "failed to open the keyboard")
	}
	defer kb.Close(ctx)

	// Start cros_healthd routine.
	var stdoutBuf bytes.Buffer
	args = append([]string{"diag"}, args...)
	runRoutineCmd := testexec.CommandContext(ctx, "cros-health-tool", args...)
	runRoutineCmd.Stdout = &stdoutBuf
	if err := runRoutineCmd.Start(); err != nil {
		testing.ContextLogf(ctx, "stdout of command: %q", stdoutBuf.String())
		runRoutineCmd.DumpLog(ctx)
		return "", errors.Wrapf(err, "failed to run %q", shutil.EscapeSlice(runRoutineCmd.Args))
	}
	testing.ContextLogf(ctx, "Running %q", shutil.EscapeSlice(runRoutineCmd.Args))

	// Press the volume button repeatedly until the routine finishes.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if err = kb.Accel(ctx, "volumeup"); err != nil {
			return errors.Wrap(err, "failed to press VolumeUp")
		}
		if strings.Contains(stdoutBuf.String(), "Status: ") {
			return nil
		}
		return errors.New("routine not finished")
	}, &testing.PollOptions{Interval: 1 * time.Second, Timeout: 6 * time.Second}); err != nil {
		return "", errors.Wrap(err, "routine timeout")
	}

	if err := runRoutineCmd.Wait(); err != nil {
		return "", errors.Wrap(err, "failed to wait command")
	}
	return stdoutBuf.String(), nil
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
