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
	"go.chromium.org/tast-tests/cros/local/camera/testutil"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         GetUserMediaPostVCSCrash,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies that getUserMedia works after the video capture service crashed",
		Contacts:     []string{"chromeos-camera-eng@google.com", "seannli@google.com"},
		BugComponent: "b:978428", // ChromeOS > Platform > Technologies > Camera > App & Framework
		Attr:         []string{"group:mainline", "group:camera-libcamera", "informational"},
		SoftwareDeps: []string{"chrome", caps.BuiltinCamera},
		Data:         append(getusermedia.DataFiles(), "web_api.html"),
		Fixture:      "chromeVideoWithVCDInUtilityProcess",
	})
}

// GetUserMediaPostVCSCrash calls getUserMedia and renders the camera's media stream
// in a video tag before/after video capture service crashes.
func GetUserMediaPostVCSCrash(ctx context.Context, s *testing.State) {
	// Restart ui to ensure that the following tests are not affected
	// if the video capture service process fails to restart.
	defer func() {
		if err := upstart.RestartJob(ctx, "ui"); err != nil {
			s.Error("Failed to restart ui: ", err)
		}
	}()

	// Ensure camera service running to avoid bad state from previous tests.
	if err := upstart.EnsureJobRunning(ctx, "cros-camera"); err != nil {
		s.Fatal("Failed to start cros-camera: ", err)
	}

	duration := 1 * time.Second

	ci := s.FixtValue().(chrome.HasChrome).Chrome()

	_, err := os.ReadFile(s.DataPath("third_party/ssim.js"))
	if err != nil {
		s.Fatal("Failed to read third_party/ssim.js: ", err)
	}

	// Run tests for 480p and 720p.
	if _, err := getusermedia.RunGetUserMedia(ctx, s.DataFileSystem(), ci, duration, nil, getusermedia.VerboseLogging); err != nil {
		s.Fatal("Failed to call getUserMedia() before killing video capture service process: ", err)
	}

	if err := testutil.KillVideoCaptureServiceProcess(ctx); err != nil {
		s.Fatal("Failed to relaunch a video capture service process: ", err)
	}

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if err := getusermedia.RunEnumerateDevices(ctx, s.DataFileSystem(), ci, getusermedia.VerboseLogging); err != nil {
			return err
		}
		return nil
	}, &testing.PollOptions{Timeout: 5 * time.Second}); err != nil {
		s.Fatal("Failed to find video input devices: ", err)
	}

	// Run tests for 480p and 720p.
	if _, err := getusermedia.RunGetUserMedia(ctx, s.DataFileSystem(), ci, duration, nil, getusermedia.VerboseLogging); err != nil {
		s.Fatal("Failed to call getUserMedia() after killing video capture service process: ", err)
	}
}
