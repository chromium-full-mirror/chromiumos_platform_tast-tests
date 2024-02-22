// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/arc/gio"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/mouse"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         GameDashboard,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Installs the game controls application and verifies game dashboard screen capture",
		Contacts:     []string{"arc-gaming@google.com", "pjlee@google.com", "phshah@google.com", "cuicuiruan@google.com"},
		// ChromeOS > Software > ARC++ > Gaming
		BugComponent: "b:1373988",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome", "no_arc_userdebug", "android_vm"},
		HardwareDeps: hwdep.D(hwdep.FeatureLevel(1)),
		Fixture:      "arcBootedWithGameDashboard",
		Timeout:      chrome.LoginTimeout + arc.BootTimeout + 1*time.Minute,
	})
}

func GameDashboard(ctx context.Context, s *testing.State) {
	gio.SetupTestAppInFullscreen(ctx, s, func(params gio.TestParams) error {
		// Start up UIAutomator.
		ui := uiauto.New(params.TestConn).WithTimeout(time.Minute).WithInterval(500 * time.Millisecond)

		// Start up keyboard.
		kb, err := input.Keyboard(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to open keyboard")
		}
		defer kb.Close(ctx)

		startRecording := nodewith.Name("Start recording").HasClass("PillButton")
		recordGame := nodewith.Name("Record game").HasClass("FeatureTile")
		stopRecord := nodewith.Name("Stop screen recording").HasClass("StopRecordingButtonTray")
		if err := uiauto.Combine("find gaming input overlay UI elements",
			// Open game dashboard.
			kb.AccelAction("Search+g"),
			// Navigate to recording settings.
			ui.LeftClickUntil(recordGame, ui.Gone(recordGame)),
			ui.LeftClickUntil(nodewith.Name("Settings").HasClass("IconButton"),
				ui.Exists(nodewith.Name("Front Camera").HasClass("CaptureModeOption"))),
			// Toggle the camera preview.
			toggleCamera(ui),
			// Move the camera preview before recording starts.
			moveCameraPreview(ui, params.TestConn),
			// Click to start recording.
			ui.LeftClickUntil(startRecording, ui.Gone(startRecording)),
			// Recording will take place after a countdown of 3 seconds.
			action.Sleep(4*time.Second),
			// Move the camera preview after recording starts.
			moveCameraPreview(ui, params.TestConn),
			// Stop recording via shelf.
			ui.LeftClickUntil(stopRecord, ui.Gone(stopRecord)),
			// Files notification exists.
			ui.WaitUntilExists(nodewith.Name("Share to YouTube").HasClass("PillButton")),
		)(ctx); err != nil {
			return errors.Wrap(err, "failed to record game")
		}

		fileName := ""
		defer func(ctx context.Context) {
			// Do nothing if `fileName` is not set.
			if fileName == "" {
				return
			}
			// Delete file to clean up environment.
			if err := os.Remove(fileName); err != nil {
				testing.ContextLog(ctx, "Failed to delete the recording: ", err)
			}
		}(ctx)

		// Verify file exists.
		downloadsPath, err := cryptohome.DownloadsPath(ctx, params.Chrome.NormalizedUser())
		if err != nil {
			return errors.Wrap(err, "failed to get user's download path")
		}

		const recordingPattern = "Screen recording*.webm"
		matches, err := filepath.Glob(filepath.Join(downloadsPath, recordingPattern))
		if err != nil {
			return errors.Wrap(err, "failed to glob Downloads folder")
		}
		if matches == nil {
			return errors.New("failed to find any matches")
		}
		if len(matches) != 1 {
			return errors.Errorf("expected 1 match in Downloads folder, got %q matches", len(matches))
		}

		// Verify positive file size.
		fileName = matches[0]
		f, err := os.Stat(fileName)
		if err != nil {
			return errors.Wrap(err, "failed to open recording file")
		}
		if f.Size() <= 0 {
			return errors.Errorf("expected positive file size, got %q file size", f.Size())
		}

		return nil
	})
}

// toggleCamera toggles the camera from on to off and back to on, and verifies the camera preview state.
func toggleCamera(ui *uiauto.Context) action.Action {
	return uiauto.Combine("toggle front camera state",
		// Turn the camera preview off.
		ui.LeftClickUntil(nodewith.Name("Off").HasClass("CaptureModeOption").Nth(1),
			ui.Gone(nodewith.ClassName("CameraPreviewView"))),
		// Turn the camera preview on.
		ui.LeftClickUntil(nodewith.Name("Front Camera").HasClass("CaptureModeOption"),
			ui.Exists(nodewith.ClassName("CameraPreviewView"))),
	)
}

// moveCameraPreview moves the camera preview to the top left corner, and then back to the
// bottom right corner.
func moveCameraPreview(ui *uiauto.Context, tconn *chrome.TestConn) action.Action {
	return func(ctx context.Context) error {
		rect, err := ui.Location(ctx, nodewith.ClassName("CameraPreviewView"))
		if err != nil {
			return err
		}
		initialLoc := rect.CenterPoint()
		finalLoc := coords.NewPoint(rect.Width/2, rect.Height/2)
		return uiauto.Combine("move camera preview",
			// Drag the preview from the bottom right to the top left.
			mouse.Drag(tconn, initialLoc, finalLoc, 5*time.Second),
			// Sleep to wait for the magnetic animation.
			action.Sleep(1*time.Second),
			// Drag the preview from the top left to the bottom right.
			mouse.Drag(tconn, finalLoc, initialLoc, 5*time.Second),
		)(ctx)
	}
}
