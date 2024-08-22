// Copyright 2019 The ChromiumOS Authors
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
		Func:         CCAUISmoke,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Smoke test for ChromeOS Camera App",
		Contacts: []string{
			"chromeos-camera-app-eng@google.com",
			"pihsun@chromium.org",
			"shik@chromium.org",
		},
		BugComponent: "b:978428", // ChromeOS > Platform > Technologies > Camera > App & Framework
		Attr:         []string{"group:mainline"},
		SoftwareDeps: []string{"camera_app", "chrome", "proprietary_codecs"},
		Params: []testing.Param{{
			Name:              "real",
			ExtraSoftwareDeps: []string{caps.BuiltinCamera},
			Fixture:           "ccaLaunched",
			ExtraAttr:         []string{"informational", "group:criticalstaging", "group:camera-libcamera"},
		}, {
			Name:              "vivid",
			ExtraSoftwareDeps: []string{caps.VividCamera},
			Fixture:           "ccaLaunched",
			ExtraAttr:         []string{"group:camera-postsubmit", "informational", "group:criticalstaging"},
		}, {
			Name:      "fake_vcd",
			Fixture:   "ccaLaunchedWithFakeVCDCamera",
			ExtraAttr: []string{"informational", "group:criticalstaging"},
		}, {
			Name:      "fake_hal",
			Fixture:   "ccaLaunchedWithFakeHALCamera",
			ExtraAttr: []string{"informational", "group:cq-medium", "group:criticalstaging"},
		}},
	})
}

func CCAUISmoke(ctx context.Context, s *testing.State) {
	app := s.FixtValue().(cca.FixtureData).App()
	s.FixtValue().(cca.FixtureData).SetDebugParams(cca.DebugParams{SaveCameraFolderWhenFail: true})

	// Switch to photo mode and take a photo.
	if err := app.SwitchMode(ctx, cca.Photo); err != nil {
		s.Error("Failed to switch to photo mode: ", err)
	}
	if _, err := app.TakeSinglePhoto(ctx, cca.TimerOff); err != nil {
		s.Error("Failed to take photo: ", err)
	}

	// Switch to video mode and record a 3s video.
	if err := app.SwitchMode(ctx, cca.Video); err != nil {
		s.Error("Failed to switch to video mode: ", err)
	}
	if _, err := app.RecordVideo(ctx, cca.TimerOff, 3*time.Second); err != nil {
		s.Error("Failed to record video: ", err)
	}
}
