// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"time"

	fwCommon "go.chromium.org/tast-tests/cros/common/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: DevToSecureMode,
		Desc: "Boots device from dev to secure(normal) mode and verifies that the device is up",
		Contacts: []string{
			"peep-fleet-infra-sw@google.com",
		},
		BugComponent: "b:1032353", // Chrome Operations > Fleet > Software > OS Fleet Automation
		Attr:         []string{"group:labqual_informational"},
		Params: []testing.Param{
			{
				Name:    "dev_mode",
				Val:     fixture.DevMode,
				Fixture: fixture.DevMode,
			},
		},
		VarDeps:      []string{"servo"},
		LacrosStatus: testing.LacrosVariantUnneeded,
	})
}

// DevToSecureMode boots device in dev mode, switches to secure(normal) mode, restarts device and verifies that the device is up.
func DevToSecureMode(ctx context.Context, s *testing.State) {
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

	pv := s.FixtValue().(*fixture.Value)
	h := pv.Helper

	if err := h.RequireServo(ctx); err != nil {
		s.Fatalf("Failed to init servo: %s ", err)
	}

	ms, err := firmware.NewModeSwitcher(ctx, h)
	if err != nil {
		s.Fatalf("Creating mode switcher: %s", err)
	}

	// Report ModeSwitcherType, for debugging.
	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Requiring config")
	}
	s.Logf("Mode switcher type: %s", h.Config.ModeSwitcherType)

	if err := h.RequireServo(ctx); err != nil {
		s.Fatalf("Error opening servo: %s", err)
	}

	// Double-check that DUT starts in the right mode.
	curr, err := h.Reporter.CurrentBootMode(ctx)
	if err != nil {
		s.Fatalf("Checking boot mode at beginning of test: %s", err)
	}
	if curr != pv.BootMode {
		s.Logf("DUT started in boot mode %s. Setting up %s", curr, pv.BootMode)
		if err = ms.RebootToMode(ctx, pv.BootMode); err != nil {
			s.Fatalf("Failed to set up %s mode: %s", pv.BootMode, err)
		}
	}

	// update gbb flags to turn off dev mode
	dutcmd := s.DUT().Conn().CommandContext(ctx, "futility", "gbb", "--set", "--flash", "--flags", "0x0")

	out, err := dutcmd.CombinedOutput()
	if err != nil {
		dutcmd.DumpLog(ctx)
		s.Fatalf("Failed to reset gbb flags, command output: %s", string(out))
	}

	// Disable dev mode
	s.Log("Disabling dev mode with crossystem command")
	dutcmd = s.DUT().Conn().CommandContext(ctx, "crossystem", "disable_dev_request=1")
	out, err = dutcmd.CombinedOutput()
	if err != nil {
		dutcmd.DumpLog(ctx)
		s.Fatalf("Cros switch to secure mode failed, command output: %s", string(out))
	}

	// Verify the boot mode and then reboot to normal.
	if curr, err := h.Reporter.CurrentBootMode(ctx); err != nil {
		s.Fatalf("Failed to determine DUT boot mode: %s", err)
	} else if curr != fwCommon.BootModeNormal {
		s.Logf("Transitioning back from %s to normal mode after disabling dev mode", curr)
		// Ensure DUT is back online within required timeout period
		waitCtx, cancel := context.WithTimeout(ctx, 150*time.Second)
		defer cancel()

		if err = ms.RebootToMode(waitCtx, fwCommon.BootModeNormal); err != nil {
			s.Fatalf("Error returning from %s to %s: %+v", curr, fwCommon.BootModeNormal, err)
		}
		testing.ContextLog(ctx, "Reconnected to DUT")
		s.Log("Transition to normal mode after diasbling dev mode completed successfully")
	}
}
