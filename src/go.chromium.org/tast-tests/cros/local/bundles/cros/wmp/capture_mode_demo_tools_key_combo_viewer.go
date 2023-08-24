// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wmp

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/wmp/wmputils"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/event"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/wmp"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CaptureModeDemoToolsKeyComboViewer,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies that the key combo viewer will display conditionally on key event if demo tools feature is enabled",
		Contacts: []string{
			"chromeos-wm-corexp@google.com",
			"chromeos-sw-engprod@google.com",
			"michelefan@chromium.org",
		},
		// ChromeOS > Software > ScreenCapture
		BugComponent: "b:1253115",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
	})
}

func CaptureModeDemoToolsKeyComboViewer(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 15*time.Second)
	defer cancel()

	// Start chrome and enable the demo tools feature.
	cr, err := chrome.New(ctx, chrome.EnableFeatures("CaptureModeDemoTools"))
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(ctx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to get user's Download path: ", err)
	}

	// Clear the downloads folder before starting a screen capture.
	if err := wmp.DeleteAllScreenCaptureFiles(downloadsPath, true /*deleteScreenRecording*/, true /*deleteScreenshots*/); err != nil {
		s.Fatal("Failed to delete all screenshots: ", err)
	}

	// Make sure to clear the downloads folder at the end of the test.
	defer func() {
		err := wmp.DeleteAllScreenCaptureFiles(downloadsPath, true /*deleteScreenRecording*/, true /*deleteScreenshots*/)
		if err != nil {
			s.Fatal("Failed to delete all screen capture files: ", err)
		}
	}()

	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	// Enter screen capture mode.
	if err := wmputils.EnsureCaptureModeActivated(tconn, true)(ctx); err != nil {
		s.Fatal("Failed to enable recording: ", err)
	}

	// Ensure case exit screen capture mode.
	defer wmputils.EnsureCaptureModeActivated(tconn, false)(cleanupCtx)

	var (
		screenRecordToggleButton     = nodewith.HasClass("IconButton").Name("Screen record")
		recordFullscreenToggleButton = nodewith.HasClass("IconButton").Name("Record full screen")
		captureModeSettingsButton    = nodewith.HasClass("IconButton").Name("Settings")
		captureSettingsWiget         = nodewith.HasClass("CaptureModeSettingsWidget")
		demoToolsToggleSwitch        = nodewith.HasClass("Switch").Name("Show clicks and keys")
		keyComboWidget               = nodewith.HasClass("KeyComboWidget")
	)

	ac := uiauto.New(tconn)

	if err := uiauto.Combine(
		"Enable demo tools from the settings menu",
		// The demo tools feature is only available in video recording mode.
		ac.DoDefault(screenRecordToggleButton),
		ac.DoDefault(recordFullscreenToggleButton),
		// Open settings menu.
		ac.DoDefault(captureModeSettingsButton),
		ac.WaitUntilExists(captureSettingsWiget),
		// Click on the demo tools toggle button to enable the feature.
		ac.LeftClick(demoToolsToggleSwitch),
	)(ctx); err != nil {
		s.Fatal("Failed to enable the demo tools feature the settings menu: ", err)
	}
	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to create a keyboard: ", err)
	}

	stopRecordButton := nodewith.HasClass("TrayBackgroundView").Name("Stop screen recording")
	if err = uiauto.Combine(
		"Enter video recording",
		kb.AccelAction("Enter"),
		// Ensure that the video recording starts by making sure that the stop recording button is stabilized in position.
		ac.WaitUntilExists(stopRecordButton),
		ac.WaitUntilNoEvent(stopRecordButton, event.LocationChanged),
	)(ctx); err != nil {
		s.Fatal("Failed to enter video recording mode: ", err)
	}

	// Press 'Ctrl + t' and the key combo widget will show up.
	if err = uiauto.Combine(
		"Showing key combo widget",
		kb.AccelPressAction("Ctrl+t"),
		ac.WaitUntilExists(keyComboWidget),
	)(ctx); err != nil {
		s.Fatal("Failed to show key combo viewer during video recording: ", err)
	}

	// On modifier key 'Ctrl' up, the key combo widget will disappear.
	if err = uiauto.Combine(
		"Releasing the modifier key 'Ctrl'",
		kb.AccelReleaseAction("Ctrl"),
		ac.WaitUntilGone(keyComboWidget),
	)(ctx); err != nil {
		s.Fatal("Key combo viewer failed to disappear: ", err)
	}

	// Ensure that all keys are released to avoid test flakiness.
	if err = kb.AccelReleaseAction("t")(ctx); err != nil {
		s.Fatal("Failed to release the non-modifier key 't': ", err)
	}

	recordTakenLabel := nodewith.HasClass("Label").Name("Screen recording taken")
	if err = uiauto.Combine(
		"Stop recording",
		ac.DoDefault(stopRecordButton),
		// Check if the screen record is taken.
		ac.WaitUntilExists(recordTakenLabel),
	)(ctx); err != nil {
		s.Fatal("Failed to stop video recording: ", err)
	}

	// Check if there is a screen record video file stored in Downloads folder.
	if err != nil {
		s.Fatal("Failed to get user's Download path: ", err)
	}
	has, err := wmputils.HasScreenRecord(ctx, downloadsPath)
	if err != nil {
		s.Fatal("Failed to check whether screen record is present: ", err)
	}
	if !has {
		s.Fatal("No screen record is stored in Downloads folder")
	}
}
