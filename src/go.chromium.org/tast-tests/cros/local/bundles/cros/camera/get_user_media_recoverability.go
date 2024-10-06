// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"net/http"
	"os"
	"time"

	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/camera/getusermedia"
	"go.chromium.org/tast-tests/cros/local/camera/testutil"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         GetUserMediaRecoverability,
		Desc:         "Verifies that getUserMedia works after the video capture service crashed",
		Contacts:     []string{"chromeos-camera-app-eng@google.com", "seannli@google.com"},
		BugComponent: "b:978428", // ChromeOS > Platform > Technologies > Camera > App & Framework
		Attr:         []string{"group:mainline", "group:camera-libcamera", "informational"},
		SoftwareDeps: []string{"chrome", caps.BuiltinCamera},
		Data:         append(getusermedia.DataFiles(), "web_api.html"),
		Fixture:      "chromeVideoWithVCDInUtilityProcess",
	})
}

// GetUserMediaRecoverability verifies the recoverability of camera functionality after the
// video capture service process and GPU process crash. It calls getUserMedia and renders
// the media stream before and after each crash.
func GetUserMediaRecoverability(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Restart ui to ensure that the following tests are not affected
	// if the video capture service process fails to restart.
	defer func() {
		if err := upstart.RestartJob(cleanupCtx, "ui"); err != nil {
			s.Error("Failed to restart ui: ", err)
		}
	}()

	// Setup fake camera HAL.
	if err := setupFakeCameraHAL(ctx); err != nil {
		s.Error("Failed to setup Fake HAL camera: ", err)
	}
	defer func() {
		if err := resetFakeCameraHAL(cleanupCtx); err != nil {
			s.Error("Failed to reset Fake HAL camera: ", err)
		}
	}()

	duration := 1 * time.Second

	ci := s.FixtValue().(chrome.HasChrome).Chrome()

	_, err := os.ReadFile(s.DataPath("third_party/ssim.js"))
	if err != nil {
		s.Fatal("Failed to read third_party/ssim.js: ", err)
	}

	if err := checkVCSUtilityProcessReadiness(ctx, ci, s.DataFileSystem()); err != nil {
		s.Fatal("VCS is not ready before killing video capture service process: ", err)
	}

	// Run tests for 480p and 720p.
	if _, err := getusermedia.RunGetUserMedia(ctx, s.DataFileSystem(), ci, duration, nil, getusermedia.VerboseLogging); err != nil {
		s.Fatal("Failed to call getUserMedia() before killing video capture service process: ", err)
	}

	if err := testutil.KillVideoCaptureServiceProcess(ctx); err != nil {
		s.Fatal("Failed to relaunch a video capture service process: ", err)
	}

	if err := checkVCSUtilityProcessReadiness(ctx, ci, s.DataFileSystem()); err != nil {
		s.Fatal("VCS is not ready after killing video capture service process: ", err)
	}

	// Run tests for 480p and 720p.
	if _, err := getusermedia.RunGetUserMedia(ctx, s.DataFileSystem(), ci, duration, nil, getusermedia.VerboseLogging); err != nil {
		s.Fatal("Failed to call getUserMedia() after killing video capture service process: ", err)
	}

	if err := testutil.KillGPUProcess(ctx); err != nil {
		s.Fatal("Failed to relaunch a GPU process: ", err)
	}

	// Run tests for 480p and 720p.
	if _, err := getusermedia.RunGetUserMedia(ctx, s.DataFileSystem(), ci, duration, nil, getusermedia.VerboseLogging); err != nil {
		s.Fatal("Failed to call getUserMedia() after killing GPU process: ", err)
	}
}

func setupFakeCameraHAL(ctx context.Context) error {
	if err := testutil.SetupTestConfig(ctx, testutil.UseFakeHALCamera); err != nil {
		return errors.Wrap(err, "failed to set up camera test config")
	}
	if err := testutil.SetupFakeHALConfig(ctx); err != nil {
		return errors.Wrap(err, "failed to setup Fake HAL config")
	}
	if err := upstart.RestartJob(ctx, "cros-camera"); err != nil {
		return errors.Wrap(err, "failed to restart cros-camera after fake camera HAL setup")
	}
	return nil
}

func resetFakeCameraHAL(ctx context.Context) error {
	if err := testutil.RemoveFakeHALConfig(ctx); err != nil {
		return errors.Wrap(err, "failed to remove fake HAL config")
	}
	if err := testutil.RemoveTestConfig(ctx); err != nil {
		return errors.Wrap(err, "failed to remove camera test config")
	}
	if err := upstart.RestartJob(ctx, "cros-camera"); err != nil {
		return errors.Wrap(err, "failed to restart cros-camera after fake camera HAL removal")
	}
	return nil
}

func checkVCSUtilityProcessReadiness(ctx context.Context, ci *chrome.Chrome, fileSystem http.FileSystem) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		return getusermedia.RunEnumerateDevices(ctx, fileSystem, ci, getusermedia.VerboseLogging)
	}, &testing.PollOptions{Timeout: 10 * time.Second})
}
