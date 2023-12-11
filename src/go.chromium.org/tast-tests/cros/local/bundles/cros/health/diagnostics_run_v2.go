// Copyright 2023 The ChromiumOS Authors
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

func init() {
	testing.AddTest(&testing.Test{
		Func:         DiagnosticsRunV2,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests that the cros_healthd diagnostic routines V2 can be run without errors",
		Contacts:     []string{"cros-tdm-tpe-eng@google.com"},
		BugComponent: "b:982097", // ChromeOS > Platform > Enablement > Health
		SoftwareDeps: []string{"diagnostics"},
		Attr:         []string{"group:mainline"},
		Params: []testing.Param{{
			// Contact: kerker@google.com
			Name:    "audio_driver",
			Val:     croshealthd.RoutineParamsV2{Routine: croshealthd.RoutineAudioDriver},
			Fixture: "crosHealthdRunning",
		}}})
}

// DiagnosticsRunV2 is a paramaterized test that runs supported diagnostic
// routines using the V2 API through cros_healthd. The purpose of this test is
// to ensure that the routines can be run without errors, and not to check if
// the routines pass or fail.
func DiagnosticsRunV2(ctx context.Context, s *testing.State) {
	params := s.Param().(croshealthd.RoutineParamsV2)
	config := croshealthd.RoutineTestingConfigV2{
		ArgsBuilder:    croshealthd.CreateLegacyRoutineV2ArgsBuilder(params),
		RoutineRunner:  croshealthd.RunDiagV2,
		ResultVerifier: croshealthd.VerifyRoutineFinishedV2,
	}
	if err := croshealthd.TestDiagRoutineV2(ctx, config); err != nil {
		s.Fatal("Routine verification failed: ", err)
	}
}
