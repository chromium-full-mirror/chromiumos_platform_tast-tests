// Copyright 2018 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"os"
	"time"

	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/camera/getusermedia"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros"
	"go.chromium.org/tast-tests/cros/local/media/pre"
	"go.chromium.org/tast-tests/cros/local/media/vm"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         GetUserMedia,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Verifies that getUserMedia captures video",
		Contacts:     []string{"chromeos-camera-eng@google.com", "shik@chromium.org"},
		BugComponent: "b:978428", // ChromeOS > Platform > Technologies > Camera > App & Framework
		Attr:         []string{"group:mainline", "group:camera-libcamera"},
		SoftwareDeps: []string{"chrome"},
		Data:         append(getusermedia.DataFiles(), "web_api.html"),
		Params: []testing.Param{
			{
				Name:              "real",
				Fixture:           "chromeVideo",
				ExtraAttr:         []string{"informational"},
				ExtraSoftwareDeps: []string{caps.BuiltinCamera},
				Val:               browser.TypeAsh,
			},
			{
				Name:              "vivid",
				Fixture:           "chromeVideo",
				ExtraAttr:         []string{"informational"},
				ExtraSoftwareDeps: []string{caps.VividCamera},
				Val:               browser.TypeAsh,
			},
			{
				Name:      "fake",
				Fixture:   "chromeVideoWithFakeWebcam",
				ExtraAttr: []string{"informational"},
				Val:       browser.TypeAsh,
			},
			{
				Name:              "lacros",
				Fixture:           pre.ChromeVideoLacrosFixture(pre.FakeMediaStreamUI),
				ExtraAttr:         []string{"informational"},
				ExtraSoftwareDeps: []string{caps.BuiltinOrVividCamera, "lacros"},
				Timeout:           7 * time.Minute, // A lenient limit for launching Lacros Chrome.
				Val:               browser.TypeLacros,
			},
			{
				Name:    "lacros_fake_vcd",
				Fixture: "chromeVideoLacrosWithFakeWebcam",
				// TODO(b/283215565): Promote to critical.
				ExtraAttr:         []string{"informational", "group:hw_agnostic"},
				ExtraSoftwareDeps: []string{"lacros"},
				Timeout:           7 * time.Minute, // A lenient limit for launching Lacros Chrome.
				Val:               browser.TypeLacros,
			},
			{
				Name:              "vcd_utility",
				Fixture:           "chromeVideoWithVCDInUtilityProcess",
				ExtraAttr:         []string{"informational"},
				ExtraSoftwareDeps: []string{caps.BuiltinCamera},
				Val:               browser.TypeAsh,
			},
			{
				Name:              "lacros_vcd_utility",
				Fixture:           pre.ChromeVideoLacrosFixture(pre.FakeMediaStreamUI, pre.VCDInUtilityProcess),
				ExtraAttr:         []string{"informational"},
				ExtraSoftwareDeps: []string{caps.BuiltinCamera, "lacros"},
				Timeout:           7 * time.Minute, // A lenient limit for launching Lacros Chrome.
				Val:               browser.TypeLacros,
			},
		},
	})
}

// GetUserMedia calls getUserMedia call and renders the camera's media stream
// in a video tag. It will test VGA and 720p and check if the gUM call succeeds.
// This test will fail when an error occurs or too many frames are broken.
//
// GetUserMedia performs video capturing for 3 seconds with 480p and 720p.
// (It's 10 seconds in case it runs under QEMU.) This a short version of
// camera.GetUserMediaPerf.
func GetUserMedia(ctx context.Context, s *testing.State) {
	// Ensure camera service running to avoid bad state from previous tests.
	if err := upstart.EnsureJobRunning(ctx, "cros-camera"); err != nil {
		s.Fatal("Failed to start cros-camera: ", err)
	}

	duration := 3 * time.Second
	// Since we use vivid on VM and it's slower than real cameras,
	// we use a longer time limit: https://crbug.com/929537
	if vm.IsRunningOnVM() {
		duration = 10 * time.Second
	}

	var ci getusermedia.ChromeInterface
	if s.Param().(browser.Type) == browser.TypeLacros {
		tconn, err := s.FixtValue().(chrome.HasChrome).Chrome().TestAPIConn(ctx)
		if err != nil {
			s.Fatal("Failed to connect to test API: ", err)
		}

		ci, err = lacros.Launch(ctx, tconn)
		if err != nil {
			s.Fatal("Failed to launch lacros-chrome: ", err)
		}
		defer ci.Close(ctx)
	} else {
		ci = s.FixtValue().(chrome.HasChrome).Chrome()
	}

	_, err := os.ReadFile(s.DataPath("third_party/ssim.js"))
	if err != nil {
		s.Fatal("Failed to read third_party/ssim.js: ", err)
	}

	// Run tests for 480p and 720p.
	if _, err := getusermedia.RunGetUserMedia(ctx, s.DataFileSystem(), ci, duration, nil, getusermedia.VerboseLogging); err != nil {
		s.Fatal("Failed to call getUserMedia(): ", err)
	}
}
