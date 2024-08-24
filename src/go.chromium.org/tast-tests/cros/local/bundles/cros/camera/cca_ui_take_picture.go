// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/camera/cca"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type takePictureSubtest struct {
	name     string
	testFunc func(context.Context, *cca.App) error
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         CCAUITakePicture,
		Desc:         "Opens CCA and verifies photo taking related use cases",
		Contacts:     []string{"chromeos-camera-app-eng@google.com", "wtlee@chromium.org", "kamchonlathorn@chromium.org"},
		BugComponent: "b:978428", // ChromeOS > Platform > Technologies > Camera > App & Framework
		Attr:         []string{
			"group:mainline",
			"informational",
			"group:intel-gating",
			"group:intel-nda",
			"group:release-health",
			"release-health_camera",
		},
		SoftwareDeps: []string{"camera_app", "chrome"},
		Params: []testing.Param{
			{
				Name:    "fake_hal",
				Fixture: "ccaLaunchedWithFakeHALCamera",
				Val: []takePictureSubtest{
					{"testTakeSinglePhoto", testTakeSinglePhoto},
					{"testTakeSinglePhotoWithTimer", testTakeSinglePhotoWithTimer},
					{"testCancelTimer", testCancelTimer},
				},
			},
			{
				Name:              "real",
				Fixture:           "ccaLaunched",
				ExtraAttr:         []string{"group:camera-libcamera"},
				ExtraHardwareDeps: hwdep.D(hwdep.CameraEnumerated()),
				Val: []takePictureSubtest{
					{"testTakeSinglePhoto", testTakeSinglePhoto},
					{"testTakeZoomedPhoto", testTakeZoomedPhoto},
				},
			},
		},
	})
}

// CCAUITakePicture verifies photo taking related functionalities works.
func CCAUITakePicture(ctx context.Context, s *testing.State) {
	app := s.FixtValue().(cca.FixtureData).App()

	subTestTimeout := 30 * time.Second
	subtests := s.Param().([]takePictureSubtest)
	for _, tst := range subtests {
		subTestCtx, cancel := context.WithTimeout(ctx, subTestTimeout)
		s.Run(subTestCtx, tst.name, func(ctx context.Context, s *testing.State) {
			if err := app.RunThroughCameras(ctx, func(_ cca.Facing) error {
				return tst.testFunc(ctx, app)
			}); err != nil {
				s.Errorf("Failed to pass %v subtest: %v", tst.name, err)
			}
		})
		cancel()
	}
}

func testTakeSinglePhoto(ctx context.Context, app *cca.App) error {
	_, err := app.TakeSinglePhoto(ctx, cca.TimerOff)
	return err
}

func testTakeZoomedPhoto(ctx context.Context, app *cca.App) error {
	if err := app.ZoomInFromPTZPanel(ctx); err != nil {
		return err
	}
	_, err := app.TakeSinglePhoto(ctx, cca.TimerOff)
	return err
}

func testTakeSinglePhotoWithTimer(ctx context.Context, app *cca.App) error {
	_, err := app.TakeSinglePhoto(ctx, cca.TimerOn)
	return err
}

func testCancelTimer(ctx context.Context, app *cca.App) error {
	if err := app.SetTimerOption(ctx, true); err != nil {
		return err
	}

	testing.ContextLog(ctx, "Click on start shutter")
	if err := app.ClickShutter(ctx); err != nil {
		return err
	}
	// GoBigSleepLint: Wait for a second before canceling the shutter.
	if err := testing.Sleep(ctx, time.Second); err != nil {
		return err
	}

	start := time.Now()
	testing.ContextLog(ctx, "Click on cancel shutter")
	if err := app.ClickShutter(ctx); err != nil {
		return err
	}
	if err := app.WaitForState(ctx, "taking", false); err != nil {
		return err
	}

	dir, err := app.SavedDir(ctx)
	if err != nil {
		return err
	}

	// GoBigSleepLint: Wait until the 3 seconds timer expires
	if err := testing.Sleep(ctx, 3*time.Second); err != nil {
		return errors.Wrap(err, "failed to sleep for three seconds")
	}

	if _, err := app.WaitForFileSaved(ctx, dir, cca.PhotoPattern, start); err == nil {
		return errors.New("failed to cancel the timer, picture was saved")
	}

	return nil
}
