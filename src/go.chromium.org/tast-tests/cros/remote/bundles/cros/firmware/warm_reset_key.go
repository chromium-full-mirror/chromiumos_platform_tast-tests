// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         WarmResetKey,
		Desc:         "Test to verify that a warm reset can be performed by key combination: Alt+Volume Up+r (sysrq_r)",
		Contacts:     []string{"digehlot@google.com", "chromeos-firmware@google.com"},
		BugComponent: "b:281859363", // ChromeOS > Platform > System > Firmware > AP
		Attr:         []string{"group:firmware", "firmware_unstable"},
		Fixture:      fixture.NormalMode,
	})
}

func apResetCount(ctx context.Context, s *testing.State) int {
	h := s.FixtValue().(*fixture.Value).Helper
	ec := firmware.NewECTool(h.DUT, firmware.ECToolNameMain)
	apResetCount, err := ec.GetAPResetCount(ctx)
	if err != nil {
		s.Fatal("Failed to get ec reset count: ", err)
	}
	testing.ContextLog(ctx, "apResetCount = ", apResetCount)
	return apResetCount
}

// WarmResetKey sends the servo command sysrq_r, which initiates the warm reset using
// 'Alt+Volume Up+r' key combination. Then it compares the boot IDs before and
// after the warm reset to verify that the warm reset was successful.
func WarmResetKey(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}

	// Get the starting AP reset count, so we can validate it only increases by 1.
	startingApResetCount := apResetCount(ctx, s)

	// Get initial boot ID.
	initialBootID, err := h.Reporter.BootID(ctx)
	if err != nil {
		s.Fatal("Failed to get boot id: ", err)
	}

	s.Log("Initiating warm reset using key combination 'Alt+Volume Up+r'")
	if err := h.Servo.KeypressWithDuration(ctx, servo.SysRQR, servo.DurTab); err != nil {
		s.Fatal("Warm Reset Key test failed")
	}

	// Wait for DUT to reconnect.
	if err := h.WaitConnect(ctx); err != nil {
		s.Fatal("Failed to reconnect to DUT: ", err)
	}

	// Get boot ID post warm reset.
	currBootID, err := h.Reporter.BootID(ctx)
	if err != nil {
		s.Fatal("Failed to get boot id: ", err)
	}

	// The boot ID should have changed after the warm reset.
	if initialBootID == currBootID {
		s.Fatal("Warm reset failed, the boot id remains same")
	}

	expectECReboot := false
	// EC Reboot is expected with warm reset for `kukui` and `jacuzzi` board.
	if h.Config.Platform == "kukui" || h.Config.Platform == "jacuzzi" {
		expectECReboot = true
	}

	currentApResetCount := apResetCount(ctx, s)

	// The test is forcing an AP reboot, so we expect the AP reset count to
	// increase by exactly 1. Any other value indicates an error.
	totalApResetCount := currentApResetCount - startingApResetCount
	if !expectECReboot && totalApResetCount <= 0 {
		// AP reset count was cleared, which indicates the EC rebooted (and
		// cleared the counter).
		s.Fatal("Unexpected EC reboot")
	} else if totalApResetCount > 1 {
		// If the reboot count is >1 then the AP rebooted too many times from
		// a single 'Alt+Volume Up+r' press.
		s.Fatal("Unexpected multiple AP reboots")
	}
}
