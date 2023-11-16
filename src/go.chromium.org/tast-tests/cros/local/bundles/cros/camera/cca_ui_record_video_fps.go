// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"math"
	"time"

	"go.chromium.org/tast-tests/cros/local/camera/cca"
	"go.chromium.org/tast-tests/cros/local/camera/testutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CCAUIRecordVideoFPS,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Record video and check the FPS of the video",
		Contacts:     []string{"chromeos-camera-eng@google.com", "chuhsuan@chromium.org"},
		Attr:         []string{"group:crosbolt", "crosbolt_perbuild"},
		SoftwareDeps: []string{"camera_app", "chrome"},
		Timeout:      5 * time.Minute,
		Fixture:      "ccaTestBridgeReadyWithFakeHALCamera",
		BugComponent: "b:978428",
	})
}

// CCAUIRecordVideoFPS records video and checks the saved video's FPS.
func CCAUIRecordVideoFPS(ctx context.Context, s *testing.State) {
	runTestWithApp := s.FixtValue().(cca.FixtureData).RunTestWithApp
	s.FixtValue().(cca.FixtureData).SetDebugParams(cca.DebugParams{SaveCameraFolderWhenFail: true})

	if err := testutil.WriteFakeHALConfig(ctx, testutil.FakeHALConfig{
		Cameras: []testutil.FakeCameraConfig{
			{ID: 1, Connected: true, SupportedFormats: []*testutil.FakeCameraFormatsConfig{
				{
					Width:      1920,
					Height:     1080,
					FrameRates: []int{30, 60},
				},
				// TODO(b/311148660): We need at least two resolutions to set FPS from UI.
				{
					Width:      640,
					Height:     360,
					FrameRates: []int{30},
				},
			}},
		},
	}); err != nil {
		s.Fatal("Failed to write fake HAL config: ", err)
	}

	if err := runTestWithApp(ctx, testRecordVideoFPS, cca.TestWithAppParams{}); err != nil {
		s.Fatal("Failed when recording videos with different FPS: ", err)
	}

}

func testRecordVideoFPS(ctx context.Context, app *cca.App) error {
	if err := app.SwitchMode(ctx, cca.Video); err != nil {
		return errors.Wrap(err, "failed to switch to video mode")
	}

	fpsTolerance := 3.0

	if err := recordVideoWithFPS(ctx, app, 30.0, fpsTolerance); err != nil {
		return errors.Wrap(err, "failed to record video with 30 FPS")
	}

	if err := app.SwitchTo60FPS(ctx); err != nil {
		return errors.Wrap(err, "failed to switch to 60 FPS")
	}

	if err := recordVideoWithFPS(ctx, app, 60.0, fpsTolerance); err != nil {
		return errors.Wrap(err, "failed to record video with 60 FPS")
	}

	return nil
}

// recordVideoWithFPS verifies that the video is recorded with the specified FPS.
func recordVideoWithFPS(ctx context.Context, app *cca.App, targetFPS, fpsTolerance float64) error {
	file, err := app.RecordVideo(ctx, cca.TimerOff, time.Second*3)
	if err != nil {
		return errors.Wrap(err, "failed to record video")
	}

	path, err := app.FilePathInSavedDir(ctx, file.Name())
	if err != nil {
		return errors.Wrap(err, "failed to get file path")
	}

	fps, err := cca.BaseVideoFPS(ctx, path)
	if err != nil {
		return errors.Wrap(err, "failed to get FPS")
	} else if math.Abs(fps-targetFPS) > fpsTolerance {
		return errors.Errorf("unexpected video FPS of %v, want %v with tolerance of %v", fps, targetFPS, fpsTolerance)
	}
	return nil
}
