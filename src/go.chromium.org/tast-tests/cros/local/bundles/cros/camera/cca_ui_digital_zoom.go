// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/local/camera/cca"
	"go.chromium.org/tast-tests/cros/local/input"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CCAUIDigitalZoom,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies photo taking and video recording when digital zoom is active in CCA",
		Contacts:     []string{"chromeos-camera-eng@google.com", "kamchonlathorn@chromium.org", "julianachang@chromium.org"},
		BugComponent: "b:167281", // ChromeOS > Platform > Technologies > Camera
		Attr:         []string{"group:mainline", "informational", "group:camera-libcamera"},
		// TODO(b/330110878): Remove the hardware deps once the crash on strongbad is resolved.
		HardwareDeps: hwdep.D(hwdep.SkipOnPlatform("strongbad")),
		SoftwareDeps: []string{"camera_app", "chrome", caps.BuiltinCamera},
		Fixture:      "ccaLaunchedWithDigitalZoomAndSuperRes",
		Params: []testing.Param{{
			Name:              "photo",
			ExtraSoftwareDeps: []string{"no_camera_feature_super_res"},
			Val:               cca.Photo,
		}, {
			Name:              "photo_super_resolution",
			ExtraSoftwareDeps: []string{"camera_feature_super_res"},
			Val:               cca.Photo,
		}, {
			Name: "video",
			Val:  cca.Video,
		}},
	})
}

func CCAUIDigitalZoom(ctx context.Context, s *testing.State) {
	app := s.FixtValue().(cca.FixtureData).App()
	testMode := s.Param().(cca.Mode)

	if err := app.SwitchMode(ctx, testMode); err != nil {
		s.Fatalf("Failed to switch to %v mode", testMode)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	// Create keyboard to close the PTZ panel.
	keyboard, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to create a keyboard")
	}
	defer keyboard.Close(cleanupCtx)

	if err := app.RunThroughCameras(ctx, func(facing cca.Facing) error {
		testing.ContextLog(ctx, "Running subtest on facing: ", facing)

		// Perform zoom.
		if err := app.Click(ctx, cca.OpenPTZPanelButton); err != nil {
			return errors.Wrap(err, "failed to open the PTZ panel")
		}

		ptz, err := app.ClickPTZButtonAndWaitSettingsUpdate(ctx, cca.ZoomInButton)
		if err != nil {
			return errors.Wrap(err, "failed to click the zoom-in button")
		}
		testing.ContextLogf(ctx, "Zoom ratio is updated to %.1fx", ptz.Zoom)

		if err := keyboard.Accel(ctx, "Esc"); err != nil {
			return errors.Wrap(err, "failed to close the PTZ panel")
		}

		if err = app.WaitForState(ctx, "view-ptz-panel", false); err != nil {
			return errors.Wrap(err, "failed to wait for PTZ panel to close")
		}

		// Perform the operation based on the test mode.
		if testMode == cca.Photo {
			if _, err := app.TakeSinglePhoto(ctx, cca.TimerOff); err != nil {
				return errors.Wrap(err, "failed to take a photo")
			}
		} else if testMode == cca.Video {
			if _, err := app.RecordVideo(ctx, cca.TimerOff, 3*time.Second); err != nil {
				return errors.Wrap(err, "failed to record a video")
			}
		}

		return nil
	}); err != nil {
		s.Error("Failed to pass the test on the current facing: ", err)
	}
}
