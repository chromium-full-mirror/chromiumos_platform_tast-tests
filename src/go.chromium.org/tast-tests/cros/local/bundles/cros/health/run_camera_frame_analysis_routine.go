// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package health tests the system daemon cros_healthd to ensure that telemetry
// and diagnostics calls can be completed successfully.
package health

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/local/camera/testpage"
	"go.chromium.org/tast-tests/cros/local/camera/testutil"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/crosconfig"
	"go.chromium.org/tast-tests/cros/local/croshealthd"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         RunCameraFrameAnalysisRoutine,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that cros_healthd can run camera frame analysis routine",
		Contacts: []string{
			"cros-tdm-tpe-eng@google.com",
			"weiluanwang@google.com",
		},
		BugComponent: "b:982097", // ChromeOS > Platform > Enablement > Health
		SoftwareDeps: []string{"diagnostics", "chrome"},
		HardwareDeps: hwdep.D(hwdep.CameraEnumerated()),
		Data:         []string{"camera_page.html", "camera_page.js"},
		Attr:         []string{"group:mainline", "informational", "group:camera_dependent", "group:criticalstaging"},
		Fixture:      "crosHealthdRunning",
	})
}

func getCameraCount(ctx context.Context) (int, error) {
	str, err := crosconfig.Get(ctx, "/camera", "count")
	if err != nil {
		if crosconfig.IsNotFound(err) {
			return 0, nil
		}
		return 0, err
	}
	return strconv.Atoi(str)
}

func buildCameraFrameAnalysisRoutineArgs(ctx context.Context) ([]string, error) {
	return []string{"camera_frame_analysis"}, nil
}

func RunCameraFrameAnalysisRoutine(ctx context.Context, s *testing.State) {
	cameraCount, err := getCameraCount(ctx)
	if err != nil {
		s.Fatal("Failed to get camera count: ", err)
	}

	if cameraCount != 0 {
		if err := upstart.EnsureJobRunning(ctx, "cros-camera-diagnostics"); err != nil {
			s.Fatal("Failed to ensure the cros-camera-diagnostics service is running: ", err)
		}

		cr, err := chrome.New(
			ctx,
			chrome.GuestLogin(),
			chrome.ExtraArgs("--use-fake-ui-for-media-stream"), // Bypass permission.
		)
		if err != nil {
			s.Fatal("Failed to start chrome: ", err)
		}

		cleanupCtx := ctx
		var cancel context.CancelFunc
		ctx, cancel = ctxutil.Shorten(ctx, 10*time.Second)
		defer cancel()

		// Use a fake camera to avoid issues about the real camera. Fake cameras
		// should be sufficient to catch issues about the integration between
		// cros_healthd and camera_diagnostics_service.
		if err := testutil.SetupTestConfig(ctx, testutil.UseFakeHALCamera); err != nil {
			s.Fatal("Failed to setup test config: ", err)
		}
		defer testutil.RemoveTestConfig(cleanupCtx)

		if err := testutil.SetupFakeHALConfig(ctx); err != nil {
			s.Fatal("Failed to setup fake hal config: ", err)
		}
		defer testutil.RemoveFakeHALConfig(cleanupCtx)

		if err := upstart.RestartJob(ctx, "cros-camera"); err != nil {
			s.Fatal("Failed to restart cros-camera after test config setup: ", err)
		}

		server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
		defer server.Close()

		webPage := testpage.New(server.URL)
		if err := webPage.Open(ctx, cr); err != nil {
			s.Fatal("Failed to open web page: ", err)
		}
		defer webPage.Close(cleanupCtx)
	} else {
		testing.ContextLog(ctx, "Skip opening cameras due to no builtin cameras")
	}

	config := croshealthd.RoutineTestingConfigV2{
		ArgsBuilder:    buildCameraFrameAnalysisRoutineArgs,
		RoutineRunner:  croshealthd.RunDiagV2,
		ResultVerifier: croshealthd.VerifyRoutineV2PassedOrUnsupported,
	}
	if err := croshealthd.TestDiagRoutineV2(ctx, config); err != nil {
		s.Fatal("Routine verification failed: ", err)
	}
}
