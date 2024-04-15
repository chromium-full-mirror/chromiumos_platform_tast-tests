// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/local/camera/cca"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CCAUIDigitalZoom,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies photo taking and video recording when digital zoom is active in CCA",
		Contacts:     []string{"chromeos-camera-eng@google.com", "kamchonlathorn@chromium.org", "julianachang@chromium.org"},
		BugComponent: "b:167281", // ChromeOS > Platform > Technologies > Camera
		Attr:         []string{"group:mainline", "informational", "group:camera-libcamera"},
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

	if err := app.RunThroughCameras(ctx, func(facing cca.Facing) error {
		testing.ContextLog(ctx, "Running subtest on facing: ", facing)

		if err := app.ZoomInFromPTZPanel(ctx); err != nil {
			return errors.Wrap(err, "failed to zoom in from PTZ panel")
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
