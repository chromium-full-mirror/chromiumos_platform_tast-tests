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
		Func:         DiagnosticsPassV2,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests that the cros_healthd diagnostic routines V2 can pass",
		Contacts:     []string{"cros-tdm-tpe-eng@google.com"},
		BugComponent: "b:982097", // ChromeOS > Platform > Enablement > Health
		SoftwareDeps: []string{"chrome", "diagnostics"},
		Attr:         []string{"group:mainline"},
		Fixture:      "crosHealthdRunning",
		// TODO(b/277548688): Monitor test results and promote stable tests to critical.
		Params: []testing.Param{{
			Name:      "memory_v2",
			Val:       croshealthd.RoutineParamsV2{Routine: croshealthd.RoutineMemoryV2},
			ExtraAttr: []string{"informational", "group:criticalstaging"},
		}, {
			Name:      "cpu_stress_v2",
			Val:       croshealthd.RoutineParamsV2{Routine: croshealthd.RoutineCPUStressV2},
			Timeout:   5 * time.Minute,
			ExtraAttr: []string{"informational", "group:criticalstaging"},
		}, {
			Name:      "audio_driver",
			Val:       croshealthd.RoutineParamsV2{Routine: croshealthd.RoutineAudioDriver},
			ExtraAttr: []string{"informational", "group:criticalstaging"},
		}, {
			Name:      "cpu_cache_v2",
			Val:       croshealthd.RoutineParamsV2{Routine: croshealthd.RoutineCPUCacheV2},
			Timeout:   5 * time.Minute,
			ExtraAttr: []string{"informational", "group:criticalstaging"},
		}, {
			Name:              "ufs_lifetime",
			Val:               croshealthd.RoutineParamsV2{Routine: croshealthd.RoutineUFSLifetime},
			ExtraAttr:         []string{"informational", "group:criticalstaging"},
			ExtraHardwareDeps: hwdep.D(hwdep.Ufs()),
		}, {
			Name:      "prime_search_v2",
			Val:       croshealthd.RoutineParamsV2{Routine: croshealthd.RoutinePrimeSearchV2},
			Timeout:   5 * time.Minute,
			ExtraAttr: []string{"informational", "group:criticalstaging"},
		}, {
			Name:      "volume_button",
			Val:       croshealthd.RoutineParamsV2{Routine: croshealthd.RoutineVolumeButton},
			ExtraAttr: []string{"informational", "group:criticalstaging"},
		}, {
			Name:              "led_lit_up",
			Val:               croshealthd.RoutineParamsV2{Routine: croshealthd.RoutineLedLitUp},
			ExtraHardwareDeps: hwdep.D(hwdep.ChromeEC()),
			ExtraAttr:         []string{"informational", "group:criticalstaging"},
		}}})
}

// DiagnosticsPassV2 is a paramaterized test that runs supported diagnostic
// routines using the V2 API through cros_healthd. The purpose of this test is
// to ensure that the routines can pass, which is a stricter version of
// DiagnosticsRunV2.* test.
func DiagnosticsPassV2(ctx context.Context, s *testing.State) {
	params := s.Param().(croshealthd.RoutineParamsV2)
	routine := params.Routine
	s.Logf("Running routine: %s", routine)
	result, err := croshealthd.RunDiagRoutineV2(ctx, params)
	if err != nil {
		s.Fatalf("Unable to run %s routine: %s", routine, err)
	}

	if result.Status != croshealthd.StatusPassed {
		s.Fatalf("Unexpected routine status for %q: got %q; want Passed;",
			routine, result.Status)
	}

	// Check to see that if the routine was run, the progress is 100%.
	if result.Progress != 100 {
		s.Fatalf("Unexpected progress value for %q routine with status %q: got %d; want 100;",
			routine, result.Status, result.Progress)
	}
}
