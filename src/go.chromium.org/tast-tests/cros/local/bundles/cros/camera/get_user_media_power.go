// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"os"
	"time"

	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/camera/getusermedia"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros"
	powersetup "go.chromium.org/tast-tests/cros/local/power/setup"
	"go.chromium.org/tast-tests/cros/local/upstart"

	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         GetUserMediaPower,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Verifies that getUserMedia captures video",
		Contacts:     []string{"chromeos-camera-eng@google.com", "seannli@google.com"},
		BugComponent: "b:978428", // ChromeOS > Platform > Technologies > Camera > App & Framework
		Attr:         []string{"group:mainline", "group:camera-libcamera", "informational"},
		SoftwareDeps: []string{"chrome", caps.BuiltinCamera},
		Data:         append(getusermedia.DataFiles(), "web_api.html"),
		Timeout:      7*time.Minute + power.RecorderTimeout,
		Params: []testing.Param{
			{
				Name:    "ash",
				Val:     getusermedia.Resolution{Width: 1280, Height: 720},
				Fixture: "chromeVideoPower",
			},
			{
				Name:              "lacros",
				Val:               getusermedia.Resolution{Width: 1280, Height: 720},
				Fixture:           "chromeVideoPowerLacros",
				ExtraSoftwareDeps: []string{"lacros"},
			},
			{
				Name:    "ash_vcd_utility",
				Val:     getusermedia.Resolution{Width: 1280, Height: 720},
				Fixture: "chromeVideoPowerWithVCDInUtilityProcess",
			},
			{
				Name:              "lacros_vcd_utility",
				Val:               getusermedia.Resolution{Width: 1280, Height: 720},
				Fixture:           "chromeVideoPowerLacrosWithVCDInUtilityProcess",
				ExtraSoftwareDeps: []string{"lacros"},
			},
		},
	})
}

// GetUserMediaPower calls getUserMedia and renders the camera's media stream in a
// video tag while measuring power consumption. It checks if the getUserMedia call
// with 720p succeeds. This test will fail when an error occurs, or power recording
// issues arise.
//
// GetUserMediaPower captures video for a longer duration to measure the sustained
// power impact of camera usage. It logs power metrics throughout the process.
func GetUserMediaPower(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	const interval = 5 * time.Second
	const duration = 60 * time.Second

	// Ensure camera service running to avoid bad state from previous tests.
	if err := upstart.EnsureJobRunning(ctx, "cros-camera"); err != nil {
		s.Fatal("Failed to start cros-camera: ", err)
	}

	_, err := os.ReadFile(s.DataPath("third_party/ssim.js"))
	if err != nil {
		s.Fatal("Failed to read third_party/ssim.js: ", err)
	}

	r := power.NewRecorder(ctx, interval, s.OutDir(), s.TestName())
	defer r.Close(cleanupCtx)

	if err := power.Cooldown(ctx); err != nil {
		s.Log("Failed to cooldown before the setup: ", err)
	}

	if err := r.Start(ctx); err != nil {
		s.Fatal("Cannot start collecting power metrics: ", err)
	}

	browserType := s.FixtValue().(powersetup.PowerUIFixtureData).Bt
	cr := s.FixtValue().(powersetup.PowerUIFixtureData).Cr

	var ci getusermedia.ChromeInterface
	if browserType == browser.TypeLacros {
		tconn, err := cr.TestAPIConn(ctx)
		if err != nil {
			s.Fatal("Failed to connect to test API: ", err)
		}

		ci, err = lacros.Launch(ctx, tconn)
		if err != nil {
			s.Fatal("Failed to launch lacros-chrome: ", err)
		}
		defer ci.Close(ctx)
	} else {
		ci = cr
	}

	// Run tests for 720p.
	resolution := s.Param().(getusermedia.Resolution)
	if _, err := getusermedia.RunGetUserMedia(ctx, s.DataFileSystem(), ci, duration, []getusermedia.Resolution{resolution}, getusermedia.VerboseLogging); err != nil {
		s.Fatal("Failed to call getUserMedia(): ", err)
	}

	if err := r.Finish(ctx); err != nil {
		s.Fatal("Cannot finish collecting power metrics: ", err)
	}

}
