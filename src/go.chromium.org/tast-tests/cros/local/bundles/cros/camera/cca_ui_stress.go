// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/local/camera/cca"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/display"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	defaultIterations = 20
	defaultSeed       = 1
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CCAUIStress,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Opens CCA and stress testing common functions randomly",
		Contacts:     []string{"chromeos-camera-eng@google.com", "shik@chromium.org", "pihsun@chromium.org"},
		SoftwareDeps: []string{"camera_app", "chrome", caps.BuiltinOrVividCamera},
		Vars: []string{
			// Number of iterations to test.
			"iterations",
			// Skip first skip_iterations iterations for reproducing failures faster.
			"skip_iterations",
			// The seed for deterministically generating the random action sequence.
			"seed",
			// The action filter regular expression. Only action names match
			// the filter will be stressed.
			"action_filter",
			// The list of comma separated actions(more than 1 action) that will be stressed.
			// In a single iteration, these actions will be stressed in the same order as given.
			"action_sequence",
			// Optional. Expecting "tablet".
			"mode",
		},
		// TODO(b/192846832): Change the fixture from ccaLaunched to ccaTestBridgeReady.
		Params: []testing.Param{{
			Name:              "real",
			ExtraSoftwareDeps: []string{caps.BuiltinCamera},
			ExtraAttr:         []string{"group:mainline", "informational", "group:camera-libcamera", "group:intel-nda", "group:intel-convertible"},
			Fixture:           "ccaTestBridgeReady",
			Timeout:           5 * time.Minute,
		}, {
			Name:              "vivid",
			ExtraSoftwareDeps: []string{caps.VividCamera},
			ExtraAttr:         []string{"group:mainline", "informational", "group:camera-libcamera"},
			Fixture:           "ccaTestBridgeReady",
			Timeout:           5 * time.Minute,
		}, {
			Name:      "fake_vcd",
			ExtraAttr: []string{"group:mainline", "informational", "group:camera-libcamera"},
			Fixture:   "ccaTestBridgeReadyWithFakeCameraWithoutFakeScene",
			Timeout:   5 * time.Minute,
		}, {
			Name:      "fake_hal",
			ExtraAttr: []string{"group:mainline", "informational", "group:camera-libcamera", "group:criticalstaging"},
			Fixture:   "ccaTestBridgeReadyWithFakeHALCamera",
			Timeout:   5 * time.Minute,
		}, {
			// For stress testing manually with real camera and longer timeout.
			Name:              "manual",
			ExtraSoftwareDeps: []string{caps.BuiltinCamera},
			Fixture:           "ccaTestBridgeReady",
			Timeout:           30 * 24 * time.Hour,
		}},
		BugComponent: "b:978428",
	})
}

type stressAction struct {
	name    string
	perform func(context.Context) error
}

func intVar(s *testing.State, name string, defaultValue int) int {
	str, ok := s.Var(name)
	if !ok {
		return defaultValue
	}

	val, err := strconv.Atoi(str)
	if err != nil {
		s.Fatalf("Failed to parse integer variable %v: %v", name, err)
	}

	return val
}

func stringVar(s *testing.State, name, defaultValue string) string {
	str, ok := s.Var(name)
	if !ok {
		return defaultValue
	}
	return str
}

// switchToRearCamera checks if the current camera is Rear camera or not. If user facing camera is open, it will switch to rear camera.
func switchToRearCamera(ctx context.Context, app cca.App) error {
	facing, err := app.GetFacing(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get camera facing")
	}
	if facing == cca.FacingBack {
		return nil
	}
	if err := app.SwitchCamera(ctx); err != nil {
		return errors.Wrap(err, "failed to switch camera")
	}
	if err := app.CheckFacing(ctx, cca.FacingBack); err != nil {
		return errors.Wrap(err, "failed to verify camera facing back")
	}
	return nil
}

func clickPhotoShutterContinuously(ctx context.Context, s *testing.State, app *cca.App) error {
	const timerState = cca.TimerOff
	iterations := intVar(s, "iterations", defaultIterations)
	seed := intVar(s, "seed", defaultSeed)
	rand.Seed(int64(seed))

	if err := app.SwitchMode(ctx, cca.Photo); err != nil {
		return err
	}

	if err := app.SetTimerOption(ctx, timerState); err != nil {
		return err
	}

	testing.ContextLog(ctx, "Start to take photos")
	var startTime = time.Now()
	if err := clickShutterContinuously(ctx, app, iterations); err != nil {
		return err
	}

	if err := app.WaitForState(ctx, "taking", false); err != nil {
		return errors.Wrap(err, "capturing hasn't ended")
	}
	dir, err := app.SavedDir(ctx)
	if err != nil {
		return err
	}
	info, err := app.WaitForFileSaved(ctx, dir, cca.PhotoPattern, startTime)
	if err != nil {
		return errors.Wrapf(err, "can't find any result pictures with regexp: %v", cca.PhotoPattern)
	}
	if info.Size() == 0 {
		return errors.Errorf("saved file %v is empty", info.Name())
	}
	if err := app.CheckNoTemporalFile(ctx, dir, cca.PhotoPattern, startTime, []string{}); err != nil {
		return err
	}
	return nil
}

func clickVideoShutterContinuously(ctx context.Context, s *testing.State, app *cca.App) error {
	iterations := intVar(s, "iterations", defaultIterations)
	seed := intVar(s, "seed", defaultSeed)
	rand.Seed(int64(seed))

	if err := app.SwitchMode(ctx, cca.Video); err != nil {
		return err
	}
	var startTime = time.Now()
	if err := clickShutterContinuously(ctx, app, iterations); err != nil {
		return err
	}
	if err := app.Close(ctx); err != nil {
		return err
	}

	dir, err := app.SavedDir(ctx)
	if err != nil {
		return err
	}
	exceptions := []string{".Temp"}
	if err := app.CheckNoTemporalFile(ctx, dir, cca.VideoPattern, startTime, exceptions); err != nil {
		return err
	}

	return nil
}

func clickShutterContinuously(ctx context.Context, app *cca.App, iterations int) error {
	for i := 0; i < iterations; i++ {
		if err := app.ClickShutter(ctx); err != nil {
			return err
		}
		if i == iterations-1 {
			break
		}
		var interval = time.Duration(rand.Intn(500)) * time.Millisecond
		// GoBigSleepLint: Wait for a random time interval (0-500ms) before clicking shutter button again.
		if err := testing.Sleep(ctx, interval); err != nil {
			return err
		}
	}
	return nil
}

func takeActionsRandomly(ctx context.Context, s *testing.State, app *cca.App) error {
	cr := s.FixtValue().(cca.FixtureData).Chrome
	tb := s.FixtValue().(cca.FixtureData).TestBridge()

	const defaultSkipIterations = 0
	const actionTimeout = 30 * time.Second
	const cleanupTimeout = 20 * time.Second

	iterations := intVar(s, "iterations", defaultIterations)
	skipIterations := intVar(s, "skip_iterations", defaultSkipIterations)
	actionFilter, err := regexp.Compile(stringVar(s, "action_filter", ".*"))
	if err != nil {
		return errors.New("failed to compile action_filter as a regexp")
	}
	actionSequences := strings.Split(stringVar(s, "action_sequence", ""), ",")

	seed := intVar(s, "seed", defaultSeed)
	rand.Seed(int64(seed))

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to connect to test API")
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()
	var tabletMode bool
	if mode, ok := s.Var("mode"); ok {
		tabletMode = mode == "tablet"
		cleanup, err := ash.EnsureTabletModeEnabled(ctx, tconn, tabletMode)
		if err != nil {
			return errors.Wrapf(err, "failed to enable tablet mode to %v", tabletMode)
		}
		defer cleanup(cleanupCtx)
	} else {
		// Use default screen mode of the DUT.
		tabletMode, err = ash.TabletModeEnabled(ctx, tconn)
		if err != nil {
			return errors.Wrap(err, "failed to get DUT default screen mode")
		}
	}
	s.Log("Running test with tablet mode: ", tabletMode)
	if tabletMode {
		cleanup, err := display.RotateToLandscape(ctx, tconn)
		if err != nil {
			return errors.Wrap(err, "failed to rotate display to landscape")
		}
		defer cleanup(cleanupCtx)
	}

	// TODO(b/182248415): Add variables to control per action parameters, like
	// how many photo should be taken consecutively or how long the video
	// recording should be.

	allActions := []stressAction{
		{
			name: "restart-app",
			perform: func(ctx context.Context) error {
				return app.Restart(ctx, tb)
			},
		},
		{
			name: "take-photo",
			perform: func(ctx context.Context) error {
				if err := app.SwitchMode(ctx, cca.Photo); err != nil {
					return err
				}
				_, err := app.TakeSinglePhoto(ctx, cca.TimerOff)
				return err
			},
		},
		{
			name: "record-video",
			perform: func(ctx context.Context) error {
				if err := app.SwitchMode(ctx, cca.Video); err != nil {
					return err
				}
				_, err := app.RecordVideo(ctx, cca.TimerOff, 3*time.Second)
				return err
			},
		},
		{
			name: "switch-photo",
			perform: func(c context.Context) error {
				return app.SwitchMode(ctx, cca.Photo)
			},
		},
		{
			name: "switch-video",
			perform: func(c context.Context) error {
				return app.SwitchMode(ctx, cca.Video)
			},
		},
	}

	numCameras, err := app.GetNumOfCameras(ctx)
	if err != nil {
		s.Fatal("Failed to get number of cameras: ", err)
	}
	if numCameras > 1 {
		allActions = append(allActions,
			stressAction{
				name: "switch-camera",
				perform: func(ctx context.Context) error {
					return app.SwitchCamera(ctx)
				},
			},
			stressAction{
				name: "switch-photo-rear",
				perform: func(ctx context.Context) error {
					if err := switchToRearCamera(ctx, *app); err != nil {
						return errors.Wrap(err, "failed to switch to rear camera")
					}
					return app.SwitchMode(ctx, cca.Photo)

				},
			},
			stressAction{
				name: "switch-video-rear",
				perform: func(ctx context.Context) error {
					if err := switchToRearCamera(ctx, *app); err != nil {
						return errors.Wrap(err, "failed to switch to rear camera")
					}
					return app.SwitchMode(ctx, cca.Video)
				},
			},
		)
	}

	var actions []stressAction

	if len(actionSequences) > 1 {
		for _, actionSeq := range actionSequences {
			for _, action := range allActions {
				if string(actionSeq) == action.name {
					actions = append(actions, action)
					break
				}
			}
		}
	} else {
		for _, action := range allActions {
			if actionFilter.MatchString(action.name) {
				actions = append(actions, action)
			}
		}
	}

	s.Logf("Start stressing for %v iterations with seed = %v, skipIterations = %v", iterations, seed, skipIterations)

	// TODO(b/182248415): Clear camera/ folder periodically, otherwise the disk
	// might be full after running many iterations.
	for i := 1; i <= iterations; i++ {
		if len(actionSequences) > 1 {
			for _, action := range actions {
				s.Logf("Iteration %d/%d: Performing action %s", i, iterations, action.name)
				if err := func() error {
					actionCtx, actionCancel := context.WithTimeout(ctx, actionTimeout)
					defer actionCancel()
					if err := action.perform(actionCtx); err != nil {
						return errors.Wrapf(err, "failed to perform action %v", action.name)
					}
					return nil
				}(); err != nil {
					return err
				}
			}
		} else {
			action := actions[rand.Intn(len(actions))]
			if i <= skipIterations {
				// We still need to call rand.Intn() to advance the internal state of PRNG.
				continue
			}
			s.Logf("Iteration %d/%d: Performing action %s", i, iterations, action.name)
			if err := func() error {
				actionCtx, actionCancel := context.WithTimeout(ctx, actionTimeout)
				defer actionCancel()
				if err := action.perform(actionCtx); err != nil {
					return errors.Wrapf(err, "failed to perform action %v", action.name)
				}
				return nil
			}(); err != nil {
				return err
			}
		}
	}
	return nil
}

// CCAUIStress opens CCA and stress common operations.
func CCAUIStress(ctx context.Context, s *testing.State) {
	runTestWithApp := s.FixtValue().(cca.FixtureData).RunTestWithApp
	s.FixtValue().(cca.FixtureData).SetDebugParams(cca.DebugParams{SaveScreenshotWhenFail: true, SaveCameraFolderWhenFail: true})
	subTestTimeout := 5 * time.Minute
	for _, tst := range []struct {
		name     string
		testFunc func(context.Context, *testing.State, *cca.App) error
	}{{
		"takeActionsRandomly",
		takeActionsRandomly,
	}, {
		"clickPhotoShutterContinuously",
		clickPhotoShutterContinuously,
	}, {
		"clickVideoShutterContinuously",
		clickVideoShutterContinuously,
	}} {
		subTestCtx, cancel := context.WithTimeout(ctx, subTestTimeout)
		s.Run(subTestCtx, tst.name, func(ctx context.Context, s *testing.State) {
			if err := runTestWithApp(ctx, func(ctx context.Context, app *cca.App) error {
				return tst.testFunc(ctx, s, app)
			}, cca.TestWithAppParams{}); err != nil {
				s.Fatalf("Test %v failed : %v", tst.name, err)
			}
		})
		defer cancel()
	}
}
