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
		Func: H1Console,
		Desc: "Verifies that H1 console is working",
		Contacts: []string{
			"chromeos-faft@google.com",
			"jbettis@chromium.org",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Attr:         []string{"group:firmware", "firmware_cr50", "firmware_bringup"},
		Vars:         []string{"servo"},
		TestBedDeps:  tbdep.ServoPresentAndWorking,
		HardwareDeps: hwdep.D(hwdep.GSCUART()),
	})
}

// H1Console opens the H1 (cr50) console and runs the sysinfo command.
func H1Console(ctx context.Context, s *testing.State) {
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

	strings, err := h.Servo.RunGSCCommandGetOutput(ctx, "sysinfo", []string{`Chip:\s*([^\n]*)\n`})
	if err != nil {
		s.Fatal("cr50 console sysinfo command: ", err)
	}
	s.Logf("H1 Chip: %s", strings[0][1])
}
