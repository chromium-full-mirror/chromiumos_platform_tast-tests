// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package intel

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast-tests/cros/remote/powercontrol"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         AltVolupRWarmReboot,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test if 'Alt + Vol Up + R' warm reboots the DUT successfully",
		Contacts:     []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		BugComponent: "b:157291", // ChromeOS > External > Intel
		ServiceDeps:  []string{"tast.cros.security.BootLockboxService"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      fixture.NormalMode,
		Timeout:      5 * time.Minute,
	})
}

func AltVolupRWarmReboot(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}

	// Perform a Chrome login.
	s.Log("Login to Chrome")
	if err := powercontrol.ChromeOSLogin(ctx, h.DUT, s.RPCHint()); err != nil {
		s.Fatal("Failed to login to chrome: ", err)
	}

	s.Log("Pressing and holding alt+vol up+r")
	if err := h.Servo.PressKeys(ctx, []string{"<alt_l>", "<f10>", "r"}, servo.DurTab); err != nil {
		s.Fatal("Failed to press keys: ", err)
	}

	s.Log(ctx, "Waiting for DUT to shutdown")
	sdCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := h.DUT.WaitUnreachable(sdCtx); err != nil {
		s.Fatal("Failed to shutdown DUT: ", err)
	}

	waitCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	if err := h.DUT.WaitConnect(waitCtx); err != nil {
		s.Fatal("Failed to wait connect DUT: ", err)
	}

	// Performing prev_sleep_state check.
	if err := powercontrol.ValidatePrevSleepState(ctx, h.DUT, 0); err != nil {
		s.Fatal("Failed to validate previous sleep state: ", err)
	}
}
