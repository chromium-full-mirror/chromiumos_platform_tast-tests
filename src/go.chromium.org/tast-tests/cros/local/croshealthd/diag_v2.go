// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package croshealthd

import (
	"context"
	"regexp"
	"strconv"
	"strings"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/shutil"
	"go.chromium.org/tast/core/testing"
)

// List of cros_healthd diagnostic routines.
const (
	RoutineMemoryV2             string = "memory_v2"
	RoutineCPUStressV2          string = "cpu_stress_v2"
	RoutineAudioDriver          string = "audio_driver"
	RoutineCPUCacheV2           string = "cpu_cache_v2"
	RoutineUFSLifetime          string = "ufs_lifetime"
	RoutinePrimeSearchV2        string = "prime_search_v2"
	RoutineFloatingPointV2      string = "floating_point_v2"
	Fan                         string = "fan"
	RoutineBluetoothPowerV2     string = "bluetooth_power_v2"
	RoutineBluetoothDiscoveryV2 string = "bluetooth_discovery_v2"
	RoutineBluetoothScanningV2  string = "bluetooth_scanning_v2"
)

// RoutineResultV2 contains the progress of the routine as a percentage and
// the routine status.
type RoutineResultV2 struct {
	Progress int
	Status   string
	Output   string
}

// VerifyRoutinePassedV2 returns nil if the routine status is passed and the progress is 100. Returns an error otherwise.
func VerifyRoutinePassedV2(result RoutineResultV2) error {
	if result.Status != StatusPassed {
		return errors.Errorf("unexpected status: got %q, want %q; output = %q", result.Status, StatusPassed, result.Output)
	}
	if result.Progress != 100 {
		return errors.Errorf("unexpected progress: got %d, want 100; output = %q", result.Progress, result.Output)
	}
	return nil
}

// VerifyRoutineFinishedV2 returns nil if both the following conditions hold
// (1) the routine status is either passed or failed,
// (2) the routine progress is 100.
// Returns an error otherwise.
//
// It verifies the routine completes successfully without crashing or throwing
// errors. For example, some lab machines might have old batteries that would
// fail the battery diagnostic routines, but this is regarded as a "successful
// run".
func VerifyRoutineFinishedV2(result RoutineResultV2) error {
	if result.Status != StatusPassed && result.Status != StatusFailed {
		return errors.Errorf("unexpected status: got %q, want %q or %q; output = %q",
			result.Status, StatusPassed, StatusFailed, result.Output)
	}
	if result.Progress != 100 {
		return errors.Errorf("unexpected progress: got %d, want 100; status = %q; output = %q",
			result.Progress, result.Status, result.Output)
	}
	return nil
}

// RoutineTestingConfigV2 contains the necessary functions to run and verify
// the result of a diagnostic routine.
type RoutineTestingConfigV2 struct {
	// ArgsBuilder returns a slice of arguments for the command
	// `cros-health-tool diag`. The arguments are used by `RoutineRunner`.
	ArgsBuilder func(context.Context) ([]string, error)
	// RoutineRunner runs a routine with the given `args` and returns the stdout
	// of `cros-health-tool diag`.
	RoutineRunner func(ctx context.Context, args []string) (string, error)
	// ResultVerifier returns an error if the result is not expected. Otherwise,
	// it returns nil.
	ResultVerifier func(RoutineResultV2) error
}

// TestDiagRoutineV2 runs a diagnostic routine and verifies the result. It
// returns the error found during the process.
func TestDiagRoutineV2(ctx context.Context, r RoutineTestingConfigV2) error {
	args, err := r.ArgsBuilder(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to build routine arguments")
	}
	args = append(args, "--single_line_json")

	output, err := r.RoutineRunner(ctx, args)
	if err != nil {
		return errors.Wrap(err, "failed to run routine")
	}

	result, err := parseDiagOutputV2(ctx, output)
	if err != nil {
		return errors.Wrap(err, "failed to parse output")
	}
	return r.ResultVerifier(result)
}

// RoutineParamsV2 are different configuration options for running a diagnostic
// routine.
type RoutineParamsV2 struct {
	Routine string // The name of the routine to run.
}

// RunDiagRoutineV2 runs the specified routine based on `params`. Returns a
// RoutineResult on success or an error.
func RunDiagRoutineV2(ctx context.Context, params RoutineParamsV2) (RoutineResultV2, error) {
	testing.ContextLogf(ctx, "Running routine: %s", params.Routine)
	diagParams := []string{params.Routine, "--single_line_json"}
	switch params.Routine {
	case RoutineMemoryV2:
		// 15000 KiB runs for about 3 seconds on a volteer machine.
		diagParams = append(diagParams, "--max_testing_mem_kib=15000")
	case RoutineCPUStressV2, RoutineCPUCacheV2, RoutinePrimeSearchV2, RoutineFloatingPointV2:
		// Runs the CPU routine for 1 second.
		diagParams = append(diagParams, "--length_seconds=1")
	default:
		// No extra parameters required for the following routines:
		//   - RoutineAudioDriver
		//   - RoutineUFSLifetime
	}
	output, err := runDiagV2(ctx, diagParams)
	if err != nil {
		return RoutineResultV2{}, err
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

// parseDiagOutputV2 is a helper function that takes the `raw` output from running a
// diagnostic V2 routine and returns a RoutineResult on success, or an error.
//
// Some examples for `raw`:
// "\rRunning Progress: 0\rWaiting: kWaitingToBeScheduled\n\rRunning Progress: 0\rRunning Progress: 1\rRunning Progress: 2\rRunning Progress: 98\rRunning Progress: 99\rRunning Progress: 100\nStatus: Passed\n"
func parseDiagOutputV2(ctx context.Context, raw string) (RoutineResultV2, error) {
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
				return RoutineResultV2{}, errors.Wrapf(err, "Unable to parse Progress value %q as int", value)
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
	return RoutineResultV2{progress, status, output}, nil
}
