// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: ECReboot,
		Desc: "Checks that device will reboot when EC gets the remote requests via UART",
		Contacts: []string{
			"chromeos-faft@google.com",
			"tij@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		// TODO: When stable, change firmware_unstable to a different attr.
		Attr:         []string{"group:firmware", "firmware_unstable"},
		Fixture:      fixture.NormalMode,
		Timeout:      20 * time.Minute,
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		LacrosStatus: testing.LacrosVariantUnneeded,
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
	}
	for _, tc := range []rebootTestCase{
		{"EC reboot", "reboot", true},
		{"EC hard reboot", "reboot hard", true},
		{"EC AP-off", "reboot ap-off", false},
		// The last "reboot test case" is actually telling the EC to reboot
		// the machine which will eventually lead to the machine being on
		// and returning from AP-off (or any other) state no matter what.
		{"EC reboot to power-up", "reboot", true},
	} {
		var (
			oldBootID string
			newBootID string
			err       error
		)
		if oldBootID, err = h.Reporter.BootID(ctx); err != nil {
			s.Fatal("Failed to fetch current boot ID: ", err)
		}
		if err := h.DUT.Conn().CommandContext(ctx, "sync").Run(); err != nil {
			s.Fatalf("Failed to sync before %s: %s", tc.rebootName, err)
		}

		previousUptime, err := getECUptimeFromConsole(ctx, h)
		if err != nil {
			s.Fatal("Failed to get ec uptime: ", err)
		}

		s.Logf("Rebooting via %s", tc.rebootName)
		if err := h.Servo.RunECCommand(ctx, tc.rebootCommand); err != nil {
			s.Fatalf("Failed to reboot via %s: %s", tc.rebootName, err)
		}

		start := time.Now()

		if !tc.shouldBeOn {
			if err := h.WaitForPowerStates(ctx, firmware.PowerStateInterval, firmware.PowerStateTimeout, "G3", "S5"); err != nil {
				s.Fatal("Failed to get G3 or S5 powerstate: ", err)
			}
			testing.ContextLog(ctx, "Pressing power key to turn on DUT")
			if err := h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.Dur(h.Config.HoldPwrButtonPowerOn)); err != nil {
				s.Fatal("Failed to press power key on DUT: ", err)
			}
			testing.ContextLog(ctx, "Waiting for S0 powerstate")
			if err := h.WaitForPowerStates(ctx, firmware.PowerStateInterval, firmware.PowerStateTimeout, "S0"); err != nil {
				s.Fatal("Failed to get S0 powerstate: ", err)
			}
		}

		s.Log("Reestablishing connection to DUT")
		waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, h.Config.DelayRebootToPing)
		defer cancelWaitConnect()
		if err := h.DUT.WaitConnect(waitConnectCtx); err != nil {
			s.Fatalf("Failed to reconnect to DUT after rebooting via %s: %s", tc.rebootName, err)
		}

		end := time.Since(start).Seconds()

		newUptime, err := getECUptimeFromConsole(ctx, h)
		if err != nil {
			s.Fatal("Failed to get ec uptime: ", err)
		}

		if (newUptime - end) > previousUptime {
			s.Fatalf("Expected uptime to reset, but did not previous %.3f, new %.3f, duration of reset: %.3f", previousUptime, newUptime, end)
		}

		if newBootID, err = h.Reporter.BootID(ctx); err != nil {
			s.Fatal("Failed to fetch current boot ID: ", err)
		}
		if newBootID == oldBootID {
			s.Fatalf("Failed to reboot via %s, old boot ID (%s) is the same as new boot ID (%s)", tc.rebootName, oldBootID, newBootID)
		}
	}
}

func getECUptimeFromConsole(ctx context.Context, h *firmware.Helper) (float64, error) {
	testing.ContextLog(ctx, "Getting current ec uptime")
	// Try using ectool uptimeinfo to get uptime.
	uptime, err := firmware.NewECTool(h.DUT, firmware.ECToolNameMain).GetECUptime(ctx)
	if err == nil {
		return uptime, nil
	}
	testing.ContextLog(ctx, "Failed to get uptime from ectool, falling back to ec console cmd 'gettime'")

	var out [][]string
	// If ectool uptimeinfo didn't work, try parsing from the ec console with gettime cmd.
	out, err = h.Servo.RunECCommandGetOutput(ctx, "gettime", []string{`Time:\s*\S+\s*=\s*(\d+\.\d+)\s*s`})
	if err != nil {
		// Many commands in ec console don't log uptime, powerinfo seems to generally work and doesn't
		// have very long output to that might get interrupted so it's a reasonable choice for teritary backup.
		testing.ContextLog(ctx, "Failed to get uptime from 'gettime' cmd, falling back to ec console cmd 'powerinfo'")
		out, err = h.Servo.RunECCommandGetOutput(ctx, "powerinfo", []string{`(\d+\.\d+) power state`})
		if err != nil {
			return -1.0, errors.Wrap(err, "failed to get uptime from ec")
		}
	}

	uptime, err = strconv.ParseFloat(out[0][1], 64)
	if err != nil {
		return -1.0, errors.Wrapf(err, "failed to parse ec uptime to float, got %s", out[0][1])
	}

	return uptime, nil
}
