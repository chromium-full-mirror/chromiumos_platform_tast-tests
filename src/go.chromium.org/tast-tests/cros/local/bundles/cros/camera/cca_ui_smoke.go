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

type testFunctionality int

const (
	none testFunctionality = iota
	photoTaking
	videoRecoridng
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CCAUISmoke,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Smoke test for Chrome Camera App",
		Contacts: []string{
			"chromeos-camera-eng@google.com",
			"pihsun@chromium.org",
			"shik@chromium.org",
			"cros-exp-wg+testresults@google.com", // for fieldtrial_testing_config
		},
		BugComponent: "b:978428", // ChromeOS > Platform > Technologies > Camera > App & Framework
		Attr:         []string{"group:mainline", "group:camera-libcamera"},
		SoftwareDeps: []string{"camera_app", "chrome"},
		Params: []testing.Param{{
			Name:              "real",
			ExtraSoftwareDeps: []string{caps.BuiltinCamera},
			Fixture:           "ccaLaunched",
			ExtraAttr:         []string{"informational"},
			Val:               none,
		}, {
			Name:              "vivid",
			ExtraSoftwareDeps: []string{caps.VividCamera},
			Fixture:           "ccaLaunched",
			// TODO(b/209833758): Removed from CQ due to flake in VM.
			ExtraAttr: []string{"group:camera-postsubmit", "informational"},
			Val:       none,
		}, {
			Name:      "fake_vcd_fieldtrial_testing_config_on",
			Fixture:   "ccaLaunchedWithFakeVCDCameraFieldTrialConfigOn",
			ExtraAttr: []string{"group:cq-medium"},
			Val:       none,
		}, {
			Name:      "fake_vcd_fieldtrial_testing_config_off",
			Fixture:   "ccaLaunchedWithFakeVCDCameraFieldTrialConfigOff",
			ExtraAttr: []string{"group:cq-medium"},
			Val:       none,
		}, {
			Name:    "photo_fake_vcd",
			Fixture: "ccaLaunchedWithFakeVCDCamera",
			Val:     photoTaking,
		}, {
			Name:              "video_fake_vcd",
			Fixture:           "ccaLaunchedWithFakeVCDCamera",
			ExtraSoftwareDeps: []string{"proprietary_codecs"},
			Val:               videoRecoridng,
		}, {
			Name:    "fake_hal",
			Fixture: "ccaLaunchedWithFakeHALCamera",
			// Marked as information because of flakiness.
			// TODO(b/275270567): Reenable on CQ once flakiness is gone.
			ExtraAttr: []string{"informational", "group:cq-medium"},
			Val:       none,
		}, {
			Name:    "photo_fake_hal",
			Fixture: "ccaLaunchedWithFakeHALCamera",
			// Marked as information because of flakiness.
			// TODO(b/275270567): Reenable on CQ once flakiness is gone.
			ExtraAttr: []string{"informational", "group:cq-medium"},
			Val:       photoTaking,
		}, {
			Name:    "video_fake_hal",
			Fixture: "ccaLaunchedWithFakeHALCamera",
			// Marked as information because of flakiness.
			// TODO(b/275270567): Reenable on CQ once flakiness is gone.
			ExtraAttr:         []string{"informational", "group:cq-medium"},
			ExtraSoftwareDeps: []string{"proprietary_codecs"},
			Val:               videoRecoridng,
		}},
	})
}

func CCAUISmoke(ctx context.Context, s *testing.State) {
	app := s.FixtValue().(cca.FixtureData).App()
	testFunction := s.Param().(testFunctionality)
	s.FixtValue().(cca.FixtureData).SetDebugParams(cca.DebugParams{SaveCameraFolderWhenFail: true})

	if testFunction == photoTaking {
		if err := app.SwitchMode(ctx, cca.Photo); err != nil {
			s.Error("Failed to switch to photo mode: ", err)
		}
		if _, err := app.TakeSinglePhoto(ctx, cca.TimerOff); err != nil {
			s.Error("Failed to take photo: ", err)
		}
	} else if testFunction == videoRecoridng {
		if err := app.SwitchMode(ctx, cca.Video); err != nil {
			s.Error("Failed to switch to video mode: ", err)
		}
		if _, err := app.RecordVideo(ctx, cca.TimerOff, 3*time.Second); err != nil {
			s.Error("Failed to record video: ", err)
		}
	}
}
