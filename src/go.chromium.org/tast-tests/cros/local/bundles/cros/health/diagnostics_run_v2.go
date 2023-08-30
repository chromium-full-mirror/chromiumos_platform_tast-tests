// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package health tests the system daemon cros_healthd to ensure that telemetry
// and diagnostics calls can be completed successfully.
package health

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/croshealthd"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DiagnosticsRunV2,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests that the cros_healthd diagnostic routines V2 can be run without errors",
		Contacts:     []string{"cros-tdm-tpe-eng@google.com"},
		BugComponent: "b:982097", // ChromeOS > Platform > Enablement > Health
		SoftwareDeps: []string{"chrome", "diagnostics"},
		Attr:         []string{"group:mainline"},
		Fixture:      "crosHealthdRunning",
		Params: []testing.Param{{
			Name:    "cpu_stress_v2",
			Val:     croshealthd.RoutineParamsV2{Routine: croshealthd.RoutineCPUStressV2},
			Timeout: 5 * time.Minute,
			// TODO(b/295497926): Promote tast to critical
			ExtraAttr: []string{"informational", "group:criticalstaging"},
		}, {
			Name: "audio_driver",
			Val:  croshealthd.RoutineParamsV2{Routine: croshealthd.RoutineAudioDriver},
		}, {
			Name:    "cpu_cache_v2",
			Val:     croshealthd.RoutineParamsV2{Routine: croshealthd.RoutineCPUCacheV2},
			Timeout: 5 * time.Minute,
			// TODO(b/281766836): Promote tast to critical
			ExtraAttr: []string{"informational", "group:criticalstaging"},
		}, {
			Name:              "ufs_lifetime",
			Val:               croshealthd.RoutineParamsV2{Routine: croshealthd.RoutineUFSLifetime},
			ExtraHardwareDeps: hwdep.D(hwdep.Ufs()),
		}}})
}

// DiagnosticsRunV2 is a paramaterized test that runs supported diagnostic
// routines using the V2 API through cros_healthd. The purpose of this test is
// to ensure that the routines can be run without errors, and not to check if
// the routines pass or fail.
func DiagnosticsRunV2(ctx context.Context, s *testing.State) {
	params := s.Param().(croshealthd.RoutineParamsV2)
	routine := params.Routine
	s.Logf("Running routine: %s", routine)
	result, err := croshealthd.RunDiagRoutineV2(ctx, params)
	if err != nil {
		s.Fatalf("Unable to run %s routine: %s", routine, err)
	}

	// Test a given routine and ensure that it can complete successfully without
	// crashing or throwing errors. For example, some lab machines might have
	// old batteries that would fail the diagnostic routines, but this should
	// not fail the Tast test.
	if result.Status != croshealthd.StatusPassed &&
		result.Status != croshealthd.StatusFailed {
		s.Fatalf("Unexpected routine status for %q: got %q; want %q or %q; output: %s",
			routine, result.Status, croshealthd.StatusPassed, croshealthd.StatusFailed, result.Output)
	}

	// Check to see that if the routine was run, the progress is 100%.
	if result.Progress != 100 {
		s.Fatalf("Unexpected progress value for %q routine with status %q: got %d; want 100; output: %s",
			routine, result.Status, result.Progress, result.Output)
	}
}
