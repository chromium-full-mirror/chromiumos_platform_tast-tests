// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	fwUtils "go.chromium.org/tast-tests/cros/remote/bundles/cros/firmware/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: WriteProtectCrossystem,
		Desc: "Verify that enabled and disabled hardware write protect is reflected in crossystem wpsw_cur",
		Contacts: []string{
			"cros-flashrom-team@google.com",
		},
		BugComponent: "b:750299",
		Attr:         []string{"group:mainline", "informational", "group:firmware", "firmware_unstable"},
		SoftwareDeps: []string{"crossystem", "flashrom"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		Timeout:      5 * time.Minute,
		Fixture:      fixture.NormalMode,
		LacrosStatus: testing.LacrosVariantUnneeded,
	})
}

func WriteProtectCrossystem(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}

	ms, err := firmware.NewModeSwitcher(ctx, h)
	if err != nil {
		s.Fatal("Creating mode switcher: ", err)
	}

	if err := h.Servo.SetFWWPState(ctx, servo.FWWPStateOff); err != nil {
		s.Fatal("Failed to disable WP: ", err)
	}

	s.Log("Performing mode aware reboot to setting hw wp")
	if err := ms.ModeAwareReboot(ctx, firmware.ColdReset); err != nil {
		s.Fatal("Failed to reboot: ", err)
	}

	if err := fwUtils.CheckCrossystemWPSW(ctx, h, 0); err != nil {
		s.Fatal("Failed to confirm WP is off: ", err)
	}

	if err := h.Servo.SetFWWPState(ctx, servo.FWWPStateOn); err != nil {
		s.Fatal("Failed to enable WP: ", err)
	}

	s.Log("Performing mode aware reboot to setting hw wp")
	if err := ms.ModeAwareReboot(ctx, firmware.ColdReset); err != nil {
		s.Fatal("Failed to reboot: ", err)
	}

	if err := fwUtils.CheckCrossystemWPSW(ctx, h, 1); err != nil {
		s.Fatal("Failed to confirm WP is on: ", err)
	}

	// Reset FWWP state to off before test end.
	if err := h.Servo.SetFWWPState(ctx, servo.FWWPStateOff); err != nil {
		s.Fatal("Failed to disable WP: ", err)
	}

	s.Log("Performing mode aware reboot to setting hw wp")
	if err := ms.ModeAwareReboot(ctx, firmware.ColdReset); err != nil {
		s.Fatal("Failed to reboot: ", err)
	}
}
