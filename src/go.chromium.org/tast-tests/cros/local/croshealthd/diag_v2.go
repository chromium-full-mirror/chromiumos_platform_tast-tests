// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package croshealthd

import (
	"bytes"
	"context"
	"fmt"
	"io"
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
	RoutineMemoryV2        string = "memory_v2"
	RoutineCPUStressV2     string = "cpu_stress_v2"
	RoutineAudioDriver     string = "audio_driver"
	RoutineCPUCacheV2      string = "cpu_cache_v2"
	RoutineUFSLifetime     string = "ufs_lifetime"
	RoutinePrimeSearchV2   string = "prime_search_v2"
	RoutineVolumeButton    string = "volume_button"
	RoutineLedLitUp        string = "led_lit_up"
	RoutineFloatingPointV2 string = "floating_point_v2"
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
	switch params.Routine {
	case RoutineMemoryV2:
		// 15000 KiB runs for about 3 seconds on a volteer machine.
		diagParams = append(diagParams, "--max_testing_mem_kib=15000")
	case RoutineCPUStressV2, RoutineCPUCacheV2, RoutinePrimeSearchV2, RoutineFloatingPointV2:
		// Runs the CPU routine for 1 second.
		diagParams = append(diagParams, "--length_seconds=1")
	case RoutineVolumeButton:
		// Runs the routine for 5 second.
		diagParams = append(diagParams, "--length_seconds=5")
		diagParams = append(diagParams, "--button_type=up")
	case RoutineLedLitUp:
		// Use an arbitrary supported LED and color for testing. Here, we use
		// the first supported LED and its first supported color from `getSupportedLED`.
		supportedLED, err := getSupportedLED(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get supported LEDs")
		}
		if len(supportedLED) == 0 {
			return nil, errors.Wrap(err, "no supported LEDs")
		}
		for ledName, ledColors := range supportedLED {
			if len(ledColors) == 0 {
				return nil, errors.Wrap(err, "the list of supported colors should not be empty")
			}
			diagParams = append(diagParams, fmt.Sprintf("--led_name=%s", ledName), fmt.Sprintf("--led_color=%s", ledColors[0]))
			break
		}
	default:
		// No extra parameters required for the following routines:
		//   - RoutineAudioDriver
		//   - RoutineUFSLifetime
	}
	var output string
	var err error
	if params.Routine == RoutineVolumeButton {
		output, err = runVolumeButtonDiag(ctx, diagParams)
	} else if params.Routine == RoutineLedLitUp {
		output, err = runLEDDiag(ctx, diagParams)
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

// runLEDDiag is a helper function similar to `runDiag` while simulating the
// user input for LED routine.
func runLEDDiag(ctx context.Context, args []string) (string, error) {
	args = append([]string{"diag"}, args...)
	cmd := testexec.CommandContext(ctx, "cros-health-tool", args...)
	testing.ContextLogf(ctx, "Running %q", shutil.EscapeSlice(cmd.Args))

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return "", errors.Wrap(err, "failed to get cmd.StdinPipe()")
	}

	go func() {
		defer stdin.Close()
		// Input `y` to proceed. The `y` indicates that the color is correct.
		io.WriteString(stdin, "y")
	}()

	stdout, stderr, err := cmd.SeparatedOutput(testexec.DumpLogOnError)
	if err != nil {
		return "", errors.Wrapf(err, "command failed with stdout: %q, stderr: %q", string(stdout), string(stderr))
	}
	return string(stdout), nil
}

// getSupportedLED returns a map of the list of supported colors for each
// supported LED. For example, {"battery": ["red", "yellow", "green"], ...}.
func getSupportedLED(ctx context.Context) (map[string][]string, error) {
	re := regexp.MustCompile(`([^:]+): 0x([a-fA-F0-9]+)`)
	possibleLEDColor := map[string]bool{
		"red":    true,
		"green":  true,
		"blue":   true,
		"yellow": true,
		"white":  true,
		"amber":  true,
	}

	m := make(map[string][]string)
	for _, ledName := range []string{"battery", "power", "adapter", "left", "right"} {
		out, err := testexec.CommandContext(ctx, "ectool", "led", ledName, "query").Output()
		if err != nil {
			// The command will fail if this LED is not supported.
			testing.ContextLogf(ctx, "Failed to query brightness range for LED %q", ledName)
			continue
		}
		// Example output:
		// Brightness range for LED 0:
		//         red     : 0x1
		//         green   : 0x1
		//         blue    : 0x0
		//         yellow  : 0x0
		//         white   : 0x0
		//         amber   : 0x1
		for _, line := range strings.Split(string(out), "\n") {
			match := re.FindStringSubmatch(line)
			if match == nil {
				continue
			}

			colorName := strings.TrimSpace(match[1])
			if _, exists := possibleLEDColor[colorName]; !exists {
				testing.ContextLogf(ctx, "Invalid LED name: %q", colorName)
				continue
			}

			// Brightness range other than 0x0 means the color is supported.
			if match[2] != "0" {
				m[ledName] = append(m[ledName], colorName)
			}
		}
	}
	return m, nil
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
