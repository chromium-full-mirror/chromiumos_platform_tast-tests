// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package health tests the system daemon cros_healthd to ensure that telemetry
// and diagnostics calls can be completed successfully.
package health

import (
	"bytes"
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/croshealthd"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/shutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         RunVolumeButtonRoutine,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that cros_healthd can run volume button routine",
		Contacts: []string{
			"cros-tdm-tpe-eng@google.com",
			"weiluanwang@google.com",
		},
		BugComponent: "b:982097", // ChromeOS > Platform > Enablement > Health
		SoftwareDeps: []string{"diagnostics"},
		Attr:         []string{"group:mainline"},
		Fixture:      "crosHealthdRunning",
	})
}

// runVolumeButtonDiag runs the volume button routine while simulating the
// volume button events.
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

func buildVolumeButtonRoutineArgs(ctx context.Context) ([]string, error) {
	return []string{"volume_button", "--button_type=up", "--length_seconds=5"}, nil
}

func RunVolumeButtonRoutine(ctx context.Context, s *testing.State) {
	config := croshealthd.RoutineTestingConfigV2{
		ArgsBuilder:    buildVolumeButtonRoutineArgs,
		RoutineRunner:  runVolumeButtonDiag,
		ResultVerifier: croshealthd.VerifyRoutineV2PassedOrUnsupported,
	}
	if err := croshealthd.TestDiagRoutineV2(ctx, config); err != nil {
		s.Fatal("Routine verification failed: ", err)
	}
}
