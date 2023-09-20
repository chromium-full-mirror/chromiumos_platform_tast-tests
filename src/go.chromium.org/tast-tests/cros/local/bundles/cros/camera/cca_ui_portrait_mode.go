// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/local/camera/cca"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CCAUIPortraitMode,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies that CCA can take portrait mode photo",
		Contacts:     []string{"chromeos-camera-eng@google.com", "wtlee@chromium.org"},
		BugComponent: "b:978428", // ChromeOS > Platform > Technologies > Camera > App & Framework
		Attr:         []string{"group:mainline", "informational", "group:camera_dependent"},
		SoftwareDeps: []string{"camera_app", "camera_feature_portrait_mode", "chrome", caps.BuiltinOrVividCamera},
		Data:         []string{"human_face_scene.jpg"},
		Fixture:      "ccaLaunchedWithFakeHALCamera",
	})
}

// CCAUIPortraitMode tests that portrait mode works expectedly.
func CCAUIPortraitMode(ctx context.Context, s *testing.State) {
	switchScene := s.FixtValue().(cca.FixtureData).SwitchScene
	s.FixtValue().(cca.FixtureData).SetDebugParams(cca.DebugParams{SaveScreenshotWhenFail: true})

	if err := switchScene(ctx, cca.SceneData{Path: s.DataPath("human_face_scene.jpg")}); err != nil {
		s.Fatal("Failed to prepare document scene: ", err)
	}

	app := s.FixtValue().(cca.FixtureData).App()

	if err := app.SwitchMode(ctx, cca.Portrait); err != nil {
		s.Fatal("Failed to switch to portrait mode: ", err)
	}
	if _, err := app.TakeSinglePhoto(ctx, cca.TimerOff); err != nil {
		s.Fatal("Failed to take portrait photo: ", err)
	}
}
