// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"time"

	uiperf "go.chromium.org/tast-tests/cros/local/bundles/cros/ui/perf"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/perfutil"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SystemTrayItemsPerf,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Measures animation smoothness of items in the system tray",
		Contacts:     []string{"cros-status-area-eng@google.com", "leandre@chromium.org", "chromeos-wmp@google.com", "chromeos-sw-engprod@google.com"},
		BugComponent: "b:1246070", // ChromeOS > Software > System UI Surfaces > Status Area
		Attr:         []string{"group:crosbolt", "crosbolt_perbuild"},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		Fixture:      "chromeLoggedIn",
		Timeout:      3 * time.Minute,
	})
}

func SystemTrayItemsPerf(ctx context.Context, s *testing.State) {
	// Ensure display on to record ui performance correctly.
	if err := power.TurnOnDisplay(ctx); err != nil {
		s.Fatal("Failed to turn on display: ", err)
	}

	cr := s.FixtValue().(*chrome.Chrome)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	ac := uiauto.New(tconn)

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to create a keyboard: ", err)
	}
	defer kb.Close(ctx)

	topRow, err := input.KeyboardTopRowLayout(ctx, kb)
	if err != nil {
		s.Fatal("Failed to load the top-row layout: ", err)
	}

	// Starts full screen recording via UI.
	screenRecordToggleButton := nodewith.HasClass("IconSliderButton").NameContaining("Screen record")
	recordFullscreenToggleButton := nodewith.HasClass("IconSliderButton").NameContaining("Record full screen")
	stopRecordButton := nodewith.HasClass("TrayBackgroundView").Name("Stop screen recording")
	recordTakenLabel := nodewith.HasClass("Label").Name("Screen recording taken")
	popupNotification := nodewith.Role(role.Window).HasClass("ash/message_center/MessagePopup")
	notificationCenterTray := nodewith.HasClass("NotificationCenterTray")

	// Ensure the notification center tray is visible by enabling caps lock. The caps lock notification gets a special icon in the notification center tray
	// (rather than the generic notification counter) - this is necessary for the next part of the test to work, since it measures the smoothness of the
	// notification counter's visibility animation, which would not occur if the notification counter was the only tray item visible in the notification center tray.
	err = kb.AccelAction("Search+Alt")(ctx)
	if err != nil {
		s.Fatal("Failed to enable caps lock via the 'Search+Alt' keyboard accelerator: ", err)
	}

	if err := perfutil.RunMultipleAndSave(ctx, s.OutDir(), cr.Browser(), uiperf.Run(s, perfutil.RunAndWaitAll(tconn, func(ctx context.Context) error {
		// Take video recording so that the shelf pod bounces up, then click on the shelf pod for it to fade out,
		// (at the same time notification counter tray item will do show animation), then we close the notification
		// for tray item to perform hide animation.
		return uiauto.Combine(
			"perform screen recording, then close the notification",
			// Ensure the notification center tray is visible.
			ac.WaitUntilExists(notificationCenterTray),
			// Enter screen capture mode.
			kb.AccelAction("Ctrl+Shift+"+topRow.SelectTask),
			ac.LeftClick(screenRecordToggleButton),
			ac.LeftClick(recordFullscreenToggleButton),
			kb.AccelAction("Enter"),
			// Records full screen for 2 seconds.
			uiauto.Sleep(2*time.Second),
			ac.LeftClick(stopRecordButton),
			// Checks if the screen record is taken. Waiting a bit increases the chances that the notification counter's show animation runs to completion.
			uiauto.Sleep(2*time.Second),
			ac.WaitUntilExists(recordTakenLabel),
			// Close the notification so that tray item performs hide animation.
			ac.LeftClick(popupNotification),
			// Opening too many "Files" app window might cause the system hang on some device.
			// Thus, we close that window on every iteration.
			kb.AccelAction("Ctrl+W"),
		)(ctx)
	},
		"Ash.StatusArea.TrayBackgroundView.BounceIn",
		"Ash.StatusArea.TrayBackgroundView.Hide",
		"Ash.StatusArea.TrayItemView.Show",
		"Ash.StatusArea.TrayItemView.Hide")),
		perfutil.StoreSmoothness,
		perfutil.RunnerOptions{IgnoreFirstRun: true, DropMinMaxValues: true},
	); err != nil {
		s.Fatal("Failed to run or save: ", err)
	}
}
