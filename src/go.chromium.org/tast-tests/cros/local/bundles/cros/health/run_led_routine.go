// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package health tests the system daemon cros_healthd to ensure that telemetry
// and diagnostics calls can be completed successfully.
package health

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/croshealthd"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/shutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         RunLEDRoutine,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that cros_healthd can run LED routine",
		Contacts: []string{
			"cros-tdm-tpe-eng@google.com",
			"weiluanwang@google.com",
		},
		BugComponent: "b:982097", // ChromeOS > Platform > Enablement > Health
		SoftwareDeps: []string{"diagnostics"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		Attr:         []string{"group:mainline"},
		Fixture:      "crosHealthdRunning",
	})
}

func buildLEDRoutineArgs(ctx context.Context) ([]string, error) {
	re := regexp.MustCompile(`([^:]+): 0x([a-fA-F0-9]+)`)
	possibleLEDColor := map[string]struct{}{
		"red":    {},
		"green":  {},
		"blue":   {},
		"yellow": {},
		"white":  {},
		"amber":  {},
	}

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
			// Use an arbitrary supported LED and color for testing.
			if match[2] != "0" {
				return []string{"led_lit_up", fmt.Sprintf("--led_name=%s", ledName), fmt.Sprintf("--led_color=%s", colorName)}, nil
			}
		}
	}
	return []string{}, errors.New("no supported LEDs")
}

// runLEDDiag runs the LED routine while simulating the user's input.
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

func RunLEDRoutine(ctx context.Context, s *testing.State) {
	config := croshealthd.RoutineTestingConfigV2{
		ArgsBuilder:    buildLEDRoutineArgs,
		RoutineRunner:  runLEDDiag,
		ResultVerifier: croshealthd.VerifyRoutinePassedV2,
	}
	if err := croshealthd.TestDiagRoutineV2(ctx, config); err != nil {
		s.Fatal("Routine verification failed: ", err)
	}
}
