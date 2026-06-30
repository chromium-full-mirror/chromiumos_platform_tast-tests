// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: ECConsole,
		Desc: "Verifies that EC console is working",
		Contacts: []string{
			"chromeos-faft@google.com",
			"jbettis@chromium.org",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Attr:         []string{"group:firmware", "firmware_ec", "firmware_bringup", "firmware_stressed", "firmware_meets_kpi", "firmware_enabled", "firmware_ec_ro", "firmware_ec_rw"},
		Vars:         []string{"servo"},
		TestBedDeps:  tbdep.ServoPresentAndWorking,
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
	})
}

// ECConsole opens the EC console and runs the version command.
func ECConsole(ctx context.Context, s *testing.State) {
	servoSpec, _ := s.Var("servo")
	h := firmware.NewHelperWithoutDUT("", servoSpec, s.KeyFile(), s.KeyDir())
	defer func() {
		if err := h.Close(ctx); err != nil {
			s.Fatal("Closing helper: ", err)
		}
	}()

	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to require servo: ", err)
	}

	strings, err := h.Servo.RunECCommandGetOutput(ctx, "version", []string{`Chip:\s*([^\n]*)\n`})
	if err != nil {
		s.Fatal("ec console version command: ", err)
	}
	s.Logf("EC Chip: %s", strings[0][1])
}
