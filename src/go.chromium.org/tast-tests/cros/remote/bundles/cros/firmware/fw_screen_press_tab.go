// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"path/filepath"
	"time"

	fwCommon "go.chromium.org/tast-tests/cros/common/firmware"
	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         FWScreenPressTab,
		Desc:         "Verify that pressing the tab key on a firmware screen shows debug info",
		LacrosStatus: testing.LacrosVariantUnneeded,
		Contacts: []string{
			"chromeos-faft@google.com",
			"cienet-firmware@cienet.corp-partner.google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Attr:         []string{"group:firmware", "firmware_bios", "firmware_level2"},
		Requirements: []string{"sys-fw-0021-v01", "sys-fw-0024-v01", "sys-fw-0025-v01"},
		Fixture:      fixture.DevMode,
		Params: []testing.Param{{
			Name:              "chromebox",
			ExtraHardwareDeps: hwdep.D(hwdep.FormFactor(hwdep.Chromebox), hwdep.ExternalDisplay()),
		}, {
			Name:              "dev",
			ExtraHardwareDeps: hwdep.D(hwdep.SkipOnFormFactor(hwdep.Chromebox)),
		}},
		Timeout: 10 * time.Minute,
	})
}

func FWScreenPressTab(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}

	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to create config: ", err)
	}

	// Save the firmware log file for upload to Testhaus at the end of the test.
	defer func() {
		if err := h.EnsureDUTBooted(ctx); err != nil {
			s.Fatal("Failed to ensure dut has booted: ", err)
		}
		logPath := filepath.Join(s.OutDir(), "firmware.log")
		if err := h.SaveCBMEMLogs(ctx, logPath); err != nil {
			s.Fatal("Failed to save firmware log: ", err)
		}
	}()

	s.Log("Rebooting to the developer screen")
	if err := h.Servo.SetPowerState(ctx, servo.PowerStateWarmReset); err != nil {
		s.Fatal("Failed to warm reset the DUT: ", err)
	}

	s.Logf("Sleeping for %s (FirmwareScreen) ", h.Config.FirmwareScreen)
	// GoBigSleepLint: Delay to wait for the firmware screen during boot-up.
	if err := testing.Sleep(ctx, h.Config.FirmwareScreen); err != nil {
		s.Fatalf("Failed to sleep for %s: %v", h.Config.FirmwareScreen, err)
	}

	s.Log("Triggering debug info separately on two firmware screens")
	expFwScreens, err := triggerDebugInfoOnFwScreens(ctx, h)
	if err != nil {
		s.Fatal("Failed to invoke debug info on firmware screen: ", err)
	}
	ms, err := firmware.NewModeSwitcher(ctx, h)
	if err != nil {
		s.Fatal("Failed to create mode switcher: ", err)
	}
	devModeBypasserParams := firmware.RunBypasser{
		BypasserMethod:        ms.BypassDevMode,
		RepeatBypasser:        true,
		WaitUntilDUTConnected: h.Config.DelayRebootToPing,
	}
	if err := ms.RunBypasserUntilDUTConnected(ctx, devModeBypasserParams); err != nil {
		s.Fatal("Failed to boot through dev mode: ", err)
	}
	found, err := h.Reporter.GetDisplayedFWScreens(ctx)
	if err != nil {
		s.Fatal("Failed to get firmware screens: ", err)
	}
	if err := checkDisplayedScreensInSeq(ctx, h, found, expFwScreens); err != nil {
		s.Fatal("Failed to verify firmware screens displayed: ", err)
	}
}

func selectMenuTopmost(ctx context.Context, h *firmware.Helper) error {
	navigator, err := firmware.NewMenuNavigator(ctx, h)
	if err != nil {
		return errors.Wrap(err, "failed to create a new menu navigator")
	}
	// Perform the greatest distance of traverse possible.
	if err := firmware.MoveTo(ctx, h, navigator, firmware.MaxMenuTraverseDistance, 0); err != nil {
		return err
	}
	if err := navigator.SelectOption(ctx); err != nil {
		return err
	}
	return nil
}

func showDebugInfo(ctx context.Context, h *firmware.Helper) error {
	testing.ContextLog(ctx, "Pressing <tab>")
	if err := h.Servo.PressKey(ctx, "<tab>", servo.DurTab); err != nil {
		return errors.Wrap(err, "failed to press tab")
	}
	return nil
}

func hideDebugInfo(ctx context.Context, h *firmware.Helper) error {
	testing.ContextLog(ctx, "Pressing <esc>")
	if err := h.Servo.PressKey(ctx, "<esc>", servo.DurTab); err != nil {
		return errors.Wrap(err, "failed to press esc")
	}
	return nil
}

func hitSpaceForToNormScreen(ctx context.Context, h *firmware.Helper) error {
	if h.Config.ModeSwitcherType != firmware.KeyboardDevSwitcher {
		return errors.New("hitting space to trigger TO_NORM only works for KeyboardDevSwitcher")
	}
	testing.ContextLog(ctx, "Pressing SPACE")
	if err := h.Servo.PressKey(ctx, " ", servo.DurTab); err != nil {
		return errors.Wrap(err, "failed to press SPACE")
	}
	return nil
}

func triggerDebugInfoOnFwScreens(ctx context.Context, h *firmware.Helper) ([]fwCommon.FwScreenID, error) {
	var fwScreensOperations []func(ctx context.Context, h *firmware.Helper) error
	var expFwScreens []fwCommon.FwScreenID
	switch h.Config.ModeSwitcherType {
	case firmware.TabletDetachableSwitcher:
		fwScreensOperations = []func(ctx context.Context, h *firmware.Helper) error{
			showDebugInfo, hideDebugInfo, selectMenuTopmost, showDebugInfo, hideDebugInfo}
		expFwScreens = []fwCommon.FwScreenID{
			fwCommon.LegacyDeveloperWarningMenu,
			fwCommon.LegacyDebugInfo,
			fwCommon.LegacyDeveloperMenu,
			fwCommon.LegacyDebugInfo,
		}
	case firmware.KeyboardDevSwitcher:
		fwScreensOperations = []func(ctx context.Context, h *firmware.Helper) error{
			showDebugInfo, hideDebugInfo, hitSpaceForToNormScreen, showDebugInfo, hideDebugInfo}
		expFwScreens = []fwCommon.FwScreenID{
			fwCommon.LegacyDeveloperWarning,
			fwCommon.LegacyDebugInfo,
			fwCommon.LegacyDeveloperToNorm,
			fwCommon.LegacyDebugInfo,
		}
	case firmware.MenuSwitcher:
		fwScreensOperations = []func(ctx context.Context, h *firmware.Helper) error{
			showDebugInfo, hideDebugInfo, selectMenuTopmost, showDebugInfo, hideDebugInfo}
		expFwScreens = []fwCommon.FwScreenID{
			fwCommon.DeveloperMode,
			fwCommon.DebugInfo,
			fwCommon.LanguageSelect,
			fwCommon.DebugInfo,
		}
	default:
		return nil, errors.Errorf("found unsupported mode switcher type %s", h.Config.ModeSwitcherType)
	}
	for _, opt := range fwScreensOperations {
		if err := opt(ctx, h); err != nil {
			return nil, err
		}
		// GoBigSleepLint: Simulate a specific speed of key press.
		if err := testing.Sleep(ctx, h.Config.KeypressDelay); err != nil {
			return nil, errors.Wrapf(err, "failed to sleep for %s", h.Config.KeypressDelay)
		}
	}
	return expFwScreens, nil
}

func checkDisplayedScreensInSeq(ctx context.Context, h *firmware.Helper, source, expected []fwCommon.FwScreenID) error {
	for index := 0; len(expected) > 0 && index < len(source); index++ {
		if source[index] == expected[0] {
			expected = expected[1:]
		}
	}
	if len(expected) != 0 {
		testing.ContextLogf(ctx, "Found firmware screens: %x", source)
		return errors.Errorf("search ended because firmware screen with id %x was not found in the log", expected[0])
	}
	return nil
}
