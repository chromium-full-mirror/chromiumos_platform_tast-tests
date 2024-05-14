// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package health tests the system daemon cros_healthd to ensure that telemetry
// and diagnostics calls can be completed successfully.
package health

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/camera/cca"
	"go.chromium.org/tast-tests/cros/local/camera/testutil"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/croshealthd"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
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
		Attr:         []string{"group:mainline", "informational", "group:criticalstaging", "group:camera_dependent"},
		Fixture:      "crosHealthdRunning",
	})
}

func buildCameraFrameAnalysisRoutineArgs(ctx context.Context) ([]string, error) {
	return []string{"camera_frame_analysis"}, nil
}

func RunCameraFrameAnalysisRoutine(ctx context.Context, s *testing.State) {
	// Camera diagnostics is installed in test image only. We need to enable it manually in tests.
	enableCameraDiagCmd := testexec.CommandContext(ctx, "bash", "/usr/local/bin/enable_camera_diagnostics.sh")
	if err := enableCameraDiagCmd.Run(); err != nil {
		s.Fatal("Failed to enable camera diagnostics: ", err)
	}

	cr, err := chrome.New(ctx, chrome.GuestLogin())
	if err != nil {
		s.Fatal("Failed to start chrome: ", err)
	}

	ctxForCleanUpTb := ctx
	ctx, cancelCleanUpTb := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancelCleanUpTb()

	tb, err := testutil.NewTestBridge(ctx, cr, testutil.UseRealCamera)
	if err != nil {
		s.Fatal("Failed to construct camera test bridge: ", err)
	}
	defer tb.TearDown(ctxForCleanUpTb)

	ctxForCleanUpApp := ctx
	ctx, cancelCleanUpApp := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancelCleanUpApp()

	app, err := cca.New(ctx, cr, s.OutDir(), tb)
	if err != nil {
		s.Fatal("Failed to start CCA: ", err)
	}
	defer app.Close(ctxForCleanUpApp)

	config := croshealthd.RoutineTestingConfigV2{
		ArgsBuilder:    buildCameraFrameAnalysisRoutineArgs,
		RoutineRunner:  croshealthd.RunDiagV2,
		ResultVerifier: croshealthd.VerifyRoutineV2PassedOrUnsupported,
	}
	if err := croshealthd.TestDiagRoutineV2(ctx, config); err != nil {
		s.Fatal("Routine verification failed: ", err)
	}
}
