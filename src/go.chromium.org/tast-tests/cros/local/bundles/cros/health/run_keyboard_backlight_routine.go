// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package health tests the system daemon cros_healthd to ensure that telemetry
// and diagnostics calls can be completed successfully.
package health

import (
	"context"
	"io"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/croshealthd"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/shutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: RunKeyboardBacklightRoutine,
		Desc: "Checks that cros_healthd can run keyboard backlight routine",
		Contacts: []string{
			"cros-tdm-tpe-eng@google.com",
			"weiluanwang@google.com",
		},
		// ChromeOS > Platform > Enablement > Serviceability > Diagnostic & Health
		BugComponent: "b:982097",
		SoftwareDeps: []string{"diagnostics"},
		Attr:         []string{"group:mainline"},
		Fixture:      "crosHealthdRunning",
		Timeout:      1 * time.Minute,
	})
}

func buildKeyboardBacklightRoutineArgs(ctx context.Context) ([]string, error) {
	return []string{"keyboard_backlight"}, nil
}

// runKeyboardBacklightDiag runs the keyboard backlight routine while simulating
// user's input.
func runKeyboardBacklightDiag(ctx context.Context, args []string) (string, error) {
	args = append([]string{"diag"}, args...)
	cmd := testexec.CommandContext(ctx, "cros-health-tool", args...)
	testing.ContextLogf(ctx, "Running %q", shutil.EscapeSlice(cmd.Args))

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return "", errors.Wrap(err, "failed to get cmd.StdinPipe()")
	}

	go func() {
		defer stdin.Close()
		// Input `y` to proceed. The `y` indicates that the LED is working.
		io.WriteString(stdin, "y")
	}()

	stdout, stderr, err := cmd.SeparatedOutput(testexec.DumpLogOnError)
	if err != nil {
		return "", errors.Wrapf(err, "command failed with stdout: %q, stderr: %q", string(stdout), string(stderr))
	}
	return string(stdout), nil
}

func RunKeyboardBacklightRoutine(ctx context.Context, s *testing.State) {
	config := croshealthd.RoutineTestingConfigV2{
		ArgsBuilder:    buildKeyboardBacklightRoutineArgs,
		RoutineRunner:  runKeyboardBacklightDiag,
		ResultVerifier: croshealthd.VerifyRoutineV2PassedOrUnsupported,
	}
	if err := croshealthd.TestDiagRoutineV2(ctx, config); err != nil {
		s.Fatal("Routine verification failed: ", err)
	}
}
