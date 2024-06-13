// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package health tests the system daemon cros_healthd to ensure that telemetry
// and diagnostics calls can be completed successfully.
package health

import (
	"context"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/local/camera/cca"
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
		Attr:         []string{"group:mainline", "informational", "group:camera_dependent"},
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

		cr, err := chrome.New(ctx, chrome.GuestLogin())
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
		tb, err := testutil.NewTestBridge(ctx, cr, testutil.UseFakeHALCamera)
		if err != nil {
			s.Fatal("Failed to construct camera test bridge: ", err)
		}
		defer tb.TearDown(cleanupCtx)

		app, err := cca.New(ctx, cr, s.OutDir(), tb)
		if err != nil {
			s.Fatal("Failed to start CCA: ", err)
		}
		defer app.Close(cleanupCtx)
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
