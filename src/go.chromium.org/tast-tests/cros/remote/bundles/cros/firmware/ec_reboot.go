// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: ECReboot,
		Desc: "Checks that device will reboot when EC gets the remote requests via UART",
		Contacts: []string{
			"chromeos-faft@google.com",
			"js@semihalf.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		// TODO: When stable, change firmware_unstable to a different attr.
		Attr:         []string{"group:firmware", "firmware_unstable"},
		Fixture:      fixture.NormalMode,
		Timeout:      12 * time.Minute,
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
	})
}

func ECReboot(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to connect to servod")
	}
	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to get configs")
	}

	type rebootTestCase struct {
		rebootName    string
		rebootCommand string
		shouldBeOn    bool
		collectBootID bool
	}

	for _, tc := range []rebootTestCase{
		{"EC reboot", "reboot", true, true},
		{"EC hard reboot", "reboot hard", true, true},
		{"EC AP-off", "reboot ap-off", false, false},
		// The last "reboot test case" is actually telling the EC to reboot
		// the machine which will eventually lead to the machine being on
		// and returning from AP-off (or any other) state no matter what.
		// We won't collect boot ID here as the previous reboot put the
		// machine in AP-off mode so it wasn't able to grab new boot ID.
		{"EC reboot to power-up", "reboot", true, false},
	} {
		var (
			oldBootID string
			newBootID string
			err       error
		)

		if tc.collectBootID {
			if oldBootID, err = h.Reporter.BootID(ctx); err != nil {
				s.Fatal("Failed to fetch current boot ID: ", err)
			}

			if err := h.DUT.Conn().CommandContext(ctx, "sync").Run(); err != nil {
				s.Fatalf("Failed to sync before %s: %s", tc.rebootName, err)
			}
		}

		s.Logf("Rebooting via %s", tc.rebootName)
		if err := h.Servo.RunECCommand(ctx, tc.rebootCommand); err != nil {
			s.Fatalf("Failed to reboot via %s: %s", tc.rebootName, err)
		}

		if tc.shouldBeOn {
			s.Log("Reestablishing connection to DUT")
			waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, h.Config.DelayRebootToPing)
			defer cancelWaitConnect()
			if err := h.DUT.WaitConnect(waitConnectCtx); err != nil {
				s.Fatalf("Failed to reconnect to DUT after rebooting via %s: %s", tc.rebootName, err)
			}
		} else {
			if err := h.WaitForPowerStates(ctx, 1*time.Second, 3*time.Minute, "G3"); err != nil {
				s.Fatalf("Failed to put system off after rebooting via %s: %s", tc.rebootName, err)
			}
		}

		if tc.collectBootID {
			if newBootID, err = h.Reporter.BootID(ctx); err != nil {
				s.Fatal("Failed to fetch current boot ID: ", err)
			}
			if newBootID == oldBootID {
				s.Fatalf("Failed to reboot via %s, old boot ID is the same as new boot ID", tc.rebootName)
			}
		}

	}
}
