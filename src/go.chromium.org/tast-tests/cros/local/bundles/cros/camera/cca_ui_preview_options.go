// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/camera/cca"
	"go.chromium.org/tast-tests/cros/local/camera/testutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CCAUIPreviewOptions,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Opens CCA and verifies the use cases of preview options like mirror",
		Contacts:     []string{"chromeos-camera-eng@google.com", "chuhsuan@chromium.org"},
		BugComponent: "b:978428", // ChromeOS > Platform > Technologies > Camera > App & Framework
		Attr:         []string{"group:mainline", "informational", "group:camera-libcamera"},
		SoftwareDeps: []string{"camera_app", "chrome"},
		Fixture:      "ccaTestBridgeReadyWithFakeHALCamera",
	})
}

// CCAUIPreviewOptions verifies preview options like mirror works.
func CCAUIPreviewOptions(ctx context.Context, s *testing.State) {
	runTestWithApp := s.FixtValue().(cca.FixtureData).RunTestWithApp

	// setup multiple fake cameras for testing camera switching behavior.
	if err := testutil.WriteFakeHALConfig(ctx, testutil.FakeHALConfig{
		Cameras: []testutil.FakeCameraConfig{
			{ID: 1, Connected: true},
			{ID: 2, Connected: true},
		},
	}); err != nil {
		s.Fatal("Failed to write fake HAL config: ", err)
	}

	subTestTimeout := 30 * time.Second
	for _, tst := range []struct {
		name     string
		testFunc func(context.Context, *cca.App) error
	}{{
		"testMirrorOption",
		testMirrorOption,
	}, {
		"testGridOption",
		testGridOption,
	}, {
		"testTimerOption",
		testTimerOption,
	}} {
		subTestCtx, cancel := context.WithTimeout(ctx, subTestTimeout)
		s.Run(subTestCtx, tst.name, func(ctx context.Context, s *testing.State) {
			if err := runTestWithApp(ctx, tst.testFunc, cca.TestWithAppParams{}); err != nil {
				s.Errorf("Failed to pass %v subtest: %v", tst.name, err)
			}
		})
		cancel()
	}
}

// testMirrorOption tests the default mirror button state is expected on all
// cameras according to their facing, and also ensures the mirror state is
// preserved after switching cameras.
func testMirrorOption(ctx context.Context, app *cca.App) error {
	if err := app.CheckVisible(ctx, cca.OpenMirrorPanelButton, true); err != nil {
		return errors.Wrap(err, "failed to check mirroring button visibility state")
	}
	// Check mirror for default camera.
	if err := checkMirror(ctx, app); err != nil {
		return errors.Wrap(err, "failed to check mirror state")
	}

	numCameras, err := app.GetNumOfCameras(ctx)
	if err != nil {
		return errors.Wrap(err, "can't get number of cameras")
	}
	if numCameras > 1 {
		testing.ContextLog(ctx, "Checking the mirror state is preserved after switching cameras")
		firstCameraDefaultMirror, err := app.Mirrored(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get mirror state")
		}
		if err := toggleMirrorState(ctx, app); err != nil {
			return errors.Wrap(err, "failed to toggle mirror state")
		}
		for i := 1; i < numCameras; i++ {
			// Switch camera.
			if err := app.SwitchCamera(ctx); err != nil {
				return errors.Wrap(err, "switching camera failed")
			}

			// Check default mirrored.
			if err := checkMirror(ctx, app); err != nil {
				return errors.Wrap(err, "failed to check mirror state")
			}
		}

		// Switch back to the first camera.
		if err := app.SwitchCamera(ctx); err != nil {
			return errors.Wrap(err, "switching camera failed")
		}

		// Mirror state should persist for each camera respectively. Since the
		// mirror state of first camera is toggled, the state should be different
		// from the default one.
		if mirrored, err := app.Mirrored(ctx); err != nil {
			return errors.Wrap(err, "failed to get mirrored state")
		} else if mirrored == firstCameraDefaultMirror {
			return errors.Wrap(err, "mirroring does not persist correctly")
		}
	}
	return nil
}

// testGridOption checks the grid option can be successfully set and the state will be preserved after switching cameras.
func testGridOption(ctx context.Context, app *cca.App) error {
	if err := app.Click(ctx, cca.OpenGridPanelButton); err != nil {
		return errors.Wrap(err, "failed to open grid option panel")
	}
	if err := app.Click(ctx, cca.GridOptionGoldenRatio); err != nil {
		return errors.Wrap(err, "failed to click the golden-grid button")
	}
	if err := app.WaitForState(ctx, "grid-golden", true); err != nil {
		return errors.Wrap(err, "failed to wait for golden-grid type being active")
	}

	// The grid option should be preserved when switching cameras.
	numCameras, err := app.GetNumOfCameras(ctx)
	if err != nil {
		return errors.Wrap(err, "can't get number of cameras")
	}
	if numCameras > 1 {
		if err := app.SwitchCamera(ctx); err != nil {
			return errors.Wrap(err, "switching camera failed")
		}
		if state, err := app.State(ctx, "grid-golden"); err != nil {
			return errors.Wrap(err, "failed to get state of the grid")
		} else if state != true {
			return errors.Wrap(err, "failed to preserve the grid state after switching camera")
		}
	}
	return nil
}

// testTimerOption checks the timer option can be successfully set and the state will be preserved after switching cameras.
func testTimerOption(ctx context.Context, app *cca.App) error {
	if err := app.Click(ctx, cca.OpenTimerPanelButton); err != nil {
		return errors.Wrap(err, "failed to open timer option panel")
	}
	if err := app.Click(ctx, cca.TimerOption10Seconds); err != nil {
		return errors.Wrap(err, "failed to click the 10s timer timer button")
	}
	if err := app.WaitForState(ctx, "timer-10s", true); err != nil {
		return errors.Wrap(err, "failed to wait for 10s-timer being active")
	}

	// The timer option should be preserved when switching cameras.
	numCameras, err := app.GetNumOfCameras(ctx)
	if err != nil {
		return errors.Wrap(err, "can't get number of cameras")
	}
	if numCameras > 1 {
		if err := app.SwitchCamera(ctx); err != nil {
			return errors.Wrap(err, "switching camera failed")
		}
		if state, err := app.State(ctx, "timer-10s"); err != nil {
			return errors.Wrap(err, "failed to get state of the timer")
		} else if state != true {
			return errors.Wrap(err, "failed to preserve the timer state after switching camera")
		}
	}
	return nil
}

// checkMirror checks if the current mirror state is the default one according to current camera facing.
func checkMirror(ctx context.Context, app *cca.App) error {
	facing, err := app.GetFacing(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get camera facing")
	}
	// Mirror should be enabled for front / external camera and should be
	// disabled for back camera.
	if mirrored, err := app.Mirrored(ctx); err != nil {
		return errors.Wrap(err, "failed to get mirrored state")
	} else if mirrored != (facing != cca.FacingBack) {
		return errors.Wrapf(err, "mirroring state is unexpected: got %v, want %v", mirrored, facing != cca.FacingBack)
	}
	return nil
}

// toggleMirrorState toggles the mirror state for the current camera.
func toggleMirrorState(ctx context.Context, app *cca.App) error {
	if err := app.Click(ctx, cca.OpenMirrorPanelButton); err != nil {
		return errors.Wrap(err, "failed to open mirror panel")
	}
	targetOption := cca.MirrorOptionOn
	if mirrored, err := app.Mirrored(ctx); err != nil {
		return errors.Wrap(err, "failed to get mirrored state")
	} else if mirrored {
		targetOption = cca.MirrorOptionOff
	}
	if err := app.Click(ctx, targetOption); err != nil {
		return errors.Wrap(err, "failed to toggle mirror state")
	}
	return nil
}
