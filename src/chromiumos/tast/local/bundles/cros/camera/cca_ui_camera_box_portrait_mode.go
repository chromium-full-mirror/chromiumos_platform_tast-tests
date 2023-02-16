// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"

	"chromiumos/tast/common/media/caps"
	"chromiumos/tast/local/camera/cca"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CCAUICameraBoxPortraitMode,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies that CCA can take portrait mode photo via CameraBox",
		Contacts:     []string{"chromeos-camera-eng@google.com", "wtlee@chromium.org"},
		Attr:         []string{"group:camerabox"},
		SoftwareDeps: []string{"camera_app", "chrome", caps.BuiltinOrVividCamera},
		Data:         []string{"human_face_scene.jpg"},
		Vars:         []string{"chart"},
		Fixture:      "ccaLaunchedInCameraBox",
		Params: []testing.Param{{
			Name:      "back",
			ExtraAttr: []string{"camerabox_facing_back"},
			Val:       cca.FacingBack,
		}, {
			Name:      "front",
			ExtraAttr: []string{"camerabox_facing_front"},
			Val:       cca.FacingFront,
		}},
		BugComponent: "b:978428",
	})
}

// CCAUICameraBoxPortraitMode tests that portrait mode works expectedly.
func CCAUICameraBoxPortraitMode(ctx context.Context, s *testing.State) {
	prepareChart := s.FixtValue().(cca.FixtureData).PrepareChart
	chartHost, ok := s.Var("chart")
	if !ok {
		chartHost = ""
	}
	if err := prepareChart(ctx, chartHost, s.DataPath("human_face_scene.jpg")); err != nil {
		s.Fatal("Failed to prepare chart: ", err)
	}
	s.FixtValue().(cca.FixtureData).SetDebugParams(cca.DebugParams{SaveScreenshotWhenFail: true})

	app := s.FixtValue().(cca.FixtureData).App()
	facing := s.Param().(cca.Facing)

	if curFacing, err := app.GetFacing(ctx); err != nil {
		s.Fatal("Failed to get facing: ", err)
	} else if curFacing != facing {
		if err := app.SwitchCamera(ctx); err != nil {
			s.Fatal("Failed to switch camera: ", err)
		}
		if err := app.CheckFacing(ctx, facing); err != nil {
			s.Fatalf("Failed to switch to the target camera %v: %v", facing, err)
		}
	}

	if supported, err := app.PortraitModeSupported(ctx); err != nil {
		s.Fatal("Failed to determine whether portrait mode is supported: ", err)
	} else if supported {
		if err := app.SwitchMode(ctx, cca.Portrait); err != nil {
			s.Fatal("Failed to switch to portrait mode: ", err)
		}
		if _, err = app.TakeSinglePhoto(ctx, cca.TimerOff); err != nil {
			s.Fatal("Failed to take portrait photo: ", err)
		}
	}
}
