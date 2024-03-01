// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package intel

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/local/camera/cca"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

type action int

const (
	userFacingImageCapture action = iota
	envFacingImageCapture
	userFacingSwitchMode
	envFacingSwitchMode
	switchCamera
	restartCamera
)

type ccaTestParams struct {
	action     action
	iter       int
	tabletMode bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         CCAUIEssential,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Opens CCA and stress testing common functions",
		Contacts:     []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		SoftwareDeps: []string{"camera_app", "chrome", caps.BuiltinCamera},
		Fixture:      "ccaTestBridgeReady",
		Attr:         []string{"group:camera_dependent"},
		Params: []testing.Param{{
			Name:      "user_facing_image_clamshell",
			ExtraAttr: []string{"group:intel-nda"},
			Val:       ccaTestParams{userFacingImageCapture, 5, false},
			Timeout:   5 * time.Minute,
		}, {
			Name:      "user_facing_image_tablet",
			ExtraAttr: []string{"group:intel-convertible"},
			Val:       ccaTestParams{userFacingImageCapture, 5, true},
			Timeout:   5 * time.Minute,
		}, {
			Name:      "env_facing_image_clamshell",
			ExtraAttr: []string{"group:intel-nda"},
			Val:       ccaTestParams{envFacingImageCapture, 5, false},
			Timeout:   5 * time.Minute,
		}, {
			Name:      "env_facing_image_tablet",
			ExtraAttr: []string{"group:intel-convertible"},
			Val:       ccaTestParams{envFacingImageCapture, 5, true},
			Timeout:   5 * time.Minute,
		}, {
			Name:      "user_facing_switch_mode",
			ExtraAttr: []string{"group:intel-nda"},
			Val:       ccaTestParams{userFacingSwitchMode, 10, false},
			Timeout:   5 * time.Minute,
		}, {
			Name:      "env_facing_switch_mode",
			ExtraAttr: []string{"group:intel-nda"},
			Val:       ccaTestParams{envFacingSwitchMode, 10, false},
			Timeout:   5 * time.Minute,
		}, {
			Name:      "switch_camera_clamshell",
			ExtraAttr: []string{"group:intel-nda"},
			Val:       ccaTestParams{switchCamera, 5, false},
			Timeout:   5 * time.Minute,
		}, {
			Name:      "switch_camera_clamshell_stress",
			ExtraAttr: []string{"group:intel-nda"},
			Val:       ccaTestParams{switchCamera, 10, false},
			Timeout:   5 * time.Minute,
		}, {
			Name:      "switch_camera_tablet",
			ExtraAttr: []string{"group:intel-convertible"},
			Val:       ccaTestParams{switchCamera, 1, true},
			Timeout:   5 * time.Minute,
		}, {
			Name:      "consecutive_image_stress",
			ExtraAttr: []string{"group:intel-nda"},
			Val:       ccaTestParams{restartCamera, 360, false},
			Timeout:   15 * time.Minute,
		}},
		BugComponent: "b:157291", // ChromeOS > External > Intel
	})
}

// CCAUIEssential opens CCA and stress common operations.
func CCAUIEssential(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cr := s.FixtValue().(cca.FixtureData).Chrome
	startApp := s.FixtValue().(cca.FixtureData).StartApp
	stopApp := s.FixtValue().(cca.FixtureData).StopApp
	iterations := s.Param().(ccaTestParams).iter
	action := s.Param().(ccaTestParams).action
	tabletMode := s.Param().(ccaTestParams).tabletMode

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}
	if tabletMode {
		s.Log("Enabling Tablet mode")
		cleanup, err := ash.EnsureTabletModeEnabled(ctx, tconn, tabletMode)
		if err != nil {
			s.Fatal("Failed to enable tablet mode to ", tabletMode)
		}
		defer cleanup(cleanupCtx)
	}

	// Starting the CCA after enabling the Tablet mode.
	app, err := startApp(ctx)
	if err != nil {
		s.Fatal("Failed to open CCA: ", err)
	}
	defer func(ctx context.Context) {
		if err := stopApp(cleanupCtx, s.HasError()); err != nil {
			s.Fatal("Failed to close CCA: ", err)
		}
		cca.ClearSavedDir(ctx, cr)
	}(cleanupCtx)

	numCameras, err := app.GetNumOfCameras(ctx)
	if err != nil {
		s.Fatal("Can't get number of cameras: ", err)
	}
	s.Logf("No. of cameras: %d", numCameras)
	if (action != userFacingImageCapture && action != userFacingSwitchMode) && numCameras == 1 {
		s.Fatal("Failed as DUT does not have Environment Camera")
	}

	type facing struct {
		defaultFacing cca.Facing
		switchFacing  cca.Facing
	}
	// CCA should open back camera as default if the device is under tablet
	// mode and open front camera as default for clamshell mode.
	var isTabletMode bool
	if err := tconn.Eval(ctx,
		`tast.promisify(chrome.autotestPrivate.isTabletModeEnabled)()`,
		&isTabletMode); err != nil {
		s.Fatal("Failed to recognize device mode: ", err)
	}
	var current facing
	if isTabletMode && numCameras != 1 {
		current.defaultFacing = cca.FacingBack
		current.switchFacing = cca.FacingFront
	} else {
		current.defaultFacing = cca.FacingFront
		current.switchFacing = cca.FacingBack
	}
	if err := app.CheckCameraFacing(ctx, current.defaultFacing); err != nil {
		s.Fatal("Failed to open default camera facing: ", err)
	}

	tb := s.FixtValue().(cca.FixtureData).TestBridge()
	for i := 1; i <= iterations; i++ {
		s.Logf("Iteration: %d/%d", i, iterations)
		switch action {
		case switchCamera:
			if err := app.SwitchCamera(ctx); err != nil {
				s.Fatal("Failed to switch camera: ", err)
			}
			if i%2 == 0 {
				if err := app.CheckCameraFacing(ctx, current.defaultFacing); err != nil {
					s.Fatal("Failed to check camera after switching: ", err)
				}
			} else {
				if err := app.CheckCameraFacing(ctx, current.switchFacing); err != nil {
					s.Fatal("Failed to check camera after switching: ", err)
				}
			}
		case userFacingImageCapture, userFacingSwitchMode:
			if current.defaultFacing != cca.FacingFront {
				if err := app.SwitchCamera(ctx); err != nil {
					s.Fatal("Failed to switch camera: ", err)
				}
				current = facing{cca.FacingFront, cca.FacingBack}
			}
			if err := app.CheckCameraFacing(ctx, cca.FacingFront); err != nil {
				s.Fatal("Failed to check for user facing camera: ", err)
			}
			if action == userFacingImageCapture {
				_, err := app.TakeSinglePhoto(ctx, cca.TimerOff)
				if err != nil {
					s.Fatal("Failed to capture image: ", err)
				}
			} else {
				if err := app.SwitchMode(ctx, cca.Video); err != nil {
					s.Fatal("Failed to switch to Video mode: ", err)
				}
				if err := app.SwitchMode(ctx, cca.Photo); err != nil {
					s.Fatal("Failed to switch to Photo mode: ", err)
				}
			}
		case envFacingImageCapture, envFacingSwitchMode:
			if current.defaultFacing != cca.FacingBack {
				if err := app.SwitchCamera(ctx); err != nil {
					s.Fatal("Failed to switch camera: ", err)
				}
				current = facing{cca.FacingBack, cca.FacingFront}
			}
			if err := app.CheckCameraFacing(ctx, cca.FacingBack); err != nil {
				s.Fatal("Failed to check for environment facing camera: ", err)
			}
			if action == envFacingImageCapture {
				_, err := app.TakeSinglePhoto(ctx, cca.TimerOff)
				if err != nil {
					s.Fatal("Failed to capture image: ", err)
				}
			} else {
				if err := app.SwitchMode(ctx, cca.Video); err != nil {
					s.Fatal("Failed to switch to Video mode: ", err)
				}
				if err := app.SwitchMode(ctx, cca.Photo); err != nil {
					s.Fatal("Failed to switch to Photo mode: ", err)
				}
			}
		case restartCamera:
			if err := app.Restart(ctx, tb); err != nil {
				s.Fatal("Failed to switch camera: ", err)
			}
			_, err := app.TakeSinglePhoto(ctx, cca.TimerOff)
			if err != nil {
				s.Fatal("Failed to capture image: ", err)
			}
		}
	}
}
