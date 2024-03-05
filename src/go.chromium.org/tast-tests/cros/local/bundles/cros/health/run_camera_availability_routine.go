// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package health tests the system daemon cros_healthd to ensure that telemetry
// and diagnostics calls can be completed successfully.
package health

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/croshealthd"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/testing"
)

type cameraAvailabilityTestParams struct {
	runCameraDiagnosticServiceCheck bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         RunCameraAvailabilityRoutine,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that cros_healthd can run camera availability routine",
		Contacts: []string{
			"cros-tdm-tpe-eng@google.com",
			"weiluanwang@google.com",
		},
		BugComponent: "b:982097", // ChromeOS > Platform > Enablement > Health
		SoftwareDeps: []string{"diagnostics"},
		Attr:         []string{"group:mainline"},
		Fixture:      "crosHealthdRunning",
		Params: []testing.Param{{
			// TODO(b/304912538): Enable `runCameraDiagnosticServiceCheck` when
			// the camera diagnostic service is enabled.
			Name: "camera_diagnostic_service_check_disabled",
			Val: cameraAvailabilityTestParams{
				runCameraDiagnosticServiceCheck: false,
			},
			// TODO(b/315739688): Promote to critical.
			ExtraAttr: []string{"informational"},
		}},
	})
}

func buildCameraAvailabilityRoutineArgs(params cameraAvailabilityTestParams) func(context.Context) ([]string, error) {
	return func(context.Context) ([]string, error) {
		var runCameraDiagnosticServiceCheckArg string
		if params.runCameraDiagnosticServiceCheck {
			runCameraDiagnosticServiceCheckArg = "true"
		} else {
			runCameraDiagnosticServiceCheckArg = "false"
		}
		return []string{
			"camera_availability",
			"--run_camera_diagnostic_service_available_check=" + runCameraDiagnosticServiceCheckArg,
		}, nil
	}
}

func RunCameraAvailabilityRoutine(ctx context.Context, s *testing.State) {
	params := s.Param().(cameraAvailabilityTestParams)

	if err := upstart.EnsureJobRunning(ctx, "cros-camera"); err != nil {
		s.Fatal("Failed to ensure the cros-camera service is running: ", err)
	}

	config := croshealthd.RoutineTestingConfigV2{
		ArgsBuilder:    buildCameraAvailabilityRoutineArgs(params),
		RoutineRunner:  croshealthd.RunDiagV2,
		ResultVerifier: croshealthd.VerifyRoutineV2PassedOrUnsupported,
	}
	if err := croshealthd.TestDiagRoutineV2(ctx, config); err != nil {
		s.Fatal("Routine verification failed: ", err)
	}
}
