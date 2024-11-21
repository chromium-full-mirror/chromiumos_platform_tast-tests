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
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: RunCameraAvailabilityRoutine,
		Desc: "Checks that cros_healthd can run camera availability routine",
		Contacts: []string{
			"cros-tdm-tpe-eng@google.com",
			"weiluanwang@google.com",
		},
		// ChromeOS > Platform > Enablement > Serviceability > Diagnostic & Health
		BugComponent: "b:982097",
		SoftwareDeps: []string{"diagnostics"},
		HardwareDeps: hwdep.D(hwdep.CameraEnumerated()),
		Attr:         []string{"group:mainline"},
		Fixture:      "crosHealthdRunning",
	})
}

func buildCameraAvailabilityRoutineArgs(ctx context.Context) ([]string, error) {
	return []string{"camera_availability"}, nil
}

func RunCameraAvailabilityRoutine(ctx context.Context, s *testing.State) {
	if err := upstart.EnsureJobRunning(ctx, "cros-camera"); err != nil {
		s.Fatal("Failed to ensure the cros-camera service is running: ", err)
	}

	config := croshealthd.RoutineTestingConfigV2{
		ArgsBuilder:    buildCameraAvailabilityRoutineArgs,
		RoutineRunner:  croshealthd.RunDiagV2,
		ResultVerifier: croshealthd.VerifyRoutineV2PassedOrUnsupported,
	}
	if err := croshealthd.TestDiagRoutineV2(ctx, config); err != nil {
		s.Fatal("Routine verification failed: ", err)
	}
}
