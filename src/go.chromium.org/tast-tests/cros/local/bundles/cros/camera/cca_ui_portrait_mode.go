// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"time"

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
		Data:         []string{"pink-nature-1920x1080.jpg", "portrait_4096x3072.jpg"},
		Fixture:      "ccaLaunchedWithFakeHALCamera",
	})
}

// CCAUIPortraitMode tests that portrait mode works expectedly.
func CCAUIPortraitMode(ctx context.Context, s *testing.State) {
	switchScene := s.FixtValue().(cca.FixtureData).SwitchScene
	s.FixtValue().(cca.FixtureData).SetDebugParams(cca.DebugParams{SaveScreenshotWhenFail: true})

	for _, tc := range []struct {
		scenePath         string
		hasHumanFace      bool
		expectedNumOutput int
	}{
		{"pink-nature-1920x1080.jpg", false, 1},
		{"portrait_4096x3072.jpg", true, 2},
	} {
		if err := switchScene(ctx, cca.SceneData{Path: s.DataPath(tc.scenePath)}); err != nil {
			s.Fatal("Failed to prepare portrait scene: ", err)
		}

		// TODO(b/309572841): Remove the temporary Sleep() call after verifying that the scene is updated after switchScene() is called.
		// GoBigSleepLint: Wait for 2 second for the test scene to be switched.
		if err := testing.Sleep(ctx, 2*time.Second); err != nil {
			s.Fatal("Failed to sleep: ", err)
		}

		app := s.FixtValue().(cca.FixtureData).App()

		if err := app.SwitchMode(ctx, cca.Portrait); err != nil {
			s.Fatal("Failed to switch to portrait mode: ", err)
		}

		outputFiles, err := app.TakePortraitPhoto(ctx, cca.TimerOff, tc.hasHumanFace)
		if err != nil {
			s.Fatal("Failed to take portrait photo: ", err)
		}

		if len(outputFiles) != tc.expectedNumOutput {
			s.Fatalf("Expected %d output files, but got %d", tc.expectedNumOutput, len(outputFiles))
		}
	}
}
