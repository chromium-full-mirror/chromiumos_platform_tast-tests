// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package health tests the system daemon cros_healthd to ensure that telemetry
// and diagnostics calls can be completed successfully.
package health

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/croshealthd"
	"go.chromium.org/tast/core/testing"
)

type sensitiveSensorRoutineTestParams struct {
	// If true, check if the routine is passed. Otherwise, check if the routine is
	// finished.
	CheckRoutinePassed bool
	// If true, check the v2 routine. Otherwise, check the v1 routine.
	CheckRoutineV2 bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         RunSensitiveSensorRoutine,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that cros_healthd can run sensitive sensor routine",
		Contacts: []string{
			"cros-tdm-tpe-eng@google.com",
			"byronlee@google.com",
		},
		BugComponent: "b:982097", // ChromeOS > Platform > Enablement > Health
		SoftwareDeps: []string{"diagnostics"},
		Attr:         []string{"group:mainline"},
		Fixture:      "crosHealthdRunning",
		Params: []testing.Param{{
			Name: "v1_finished",
			Val: sensitiveSensorRoutineTestParams{
				CheckRoutinePassed: false,
				CheckRoutineV2:     false,
			},
			// TODO(b/280388091): Promote tast to critical
			ExtraAttr: []string{"informational", "group:criticalstaging"},
		}, {
			Name: "v1_passed",
			Val: sensitiveSensorRoutineTestParams{
				CheckRoutinePassed: true,
				CheckRoutineV2:     false,
			},
			// TODO(b/280388091): Promote tast to critical
			ExtraAttr: []string{"informational", "group:criticalstaging"},
		}, {
			Name: "v2_finished",
			Val: sensitiveSensorRoutineTestParams{
				CheckRoutinePassed: false,
				CheckRoutineV2:     true,
			},
			// TODO(b/280388091): Promote tast to critical
			ExtraAttr: []string{"informational", "group:criticalstaging"},
		}, {
			Name: "v2_passed",
			Val: sensitiveSensorRoutineTestParams{
				CheckRoutinePassed: true,
				CheckRoutineV2:     true,
			},
			// TODO(b/280388091): Promote tast to critical
			ExtraAttr: []string{"informational", "group:criticalstaging"},
		}},
	})
}

func buildSensitiveSensorRoutineArgs(ctx context.Context) ([]string, error) {
	return []string{"sensitive_sensor_v2"}, nil
}

// RunSensitiveSensorRoutine runs the sensitive sensor routine.
func RunSensitiveSensorRoutine(ctx context.Context, s *testing.State) {
	param := s.Param().(sensitiveSensorRoutineTestParams)

	if param.CheckRoutineV2 {
		verifier := croshealthd.VerifyRoutineFinishedV2
		if param.CheckRoutinePassed {
			verifier = croshealthd.VerifyRoutinePassedV2
		}

		config := croshealthd.RoutineTestingConfigV2{
			ArgsBuilder:    buildSensitiveSensorRoutineArgs,
			RoutineRunner:  croshealthd.RunDiagV2,
			ResultVerifier: verifier,
		}
		if err := croshealthd.TestDiagRoutineV2(ctx, config); err != nil {
			s.Fatal("Routine verification failed: ", err)
		}
	} else {
		result, err := croshealthd.RunDiagRoutine(ctx,
			croshealthd.NewRoutineParams(croshealthd.RoutineSensitiveSensor))
		if err != nil {
			s.Fatalf("Unable to run routine: %s", err)
		}

		if param.CheckRoutinePassed {
			if err := result.VerifyPassed(); err != nil {
				s.Fatalf("Routine is not passed: %s", err)
			}
		} else {
			if err := result.VerifyFinished(); err != nil {
				s.Fatalf("Routine is not finished: %s", err)
			}
		}
	}
}
