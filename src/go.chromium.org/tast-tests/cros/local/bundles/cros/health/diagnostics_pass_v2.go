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
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DiagnosticsPassV2,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests that the cros_healthd diagnostic routines V2 can pass",
		Contacts:     []string{"cros-tdm-tpe-eng@google.com"},
		BugComponent: "b:982097", // ChromeOS > Platform > Enablement > Health
		SoftwareDeps: []string{"diagnostics"},
		Attr:         []string{"group:mainline"},
		Fixture:      "crosHealthdRunning",
		Params: []testing.Param{{
			// Contact: yycheng@google.com
			Name: "memory_v2",
			Val:  croshealthd.RoutineParamsV2{Routine: croshealthd.RoutineMemoryV2},
		}, {
			// Contact: yycheng@google.com
			Name: "cpu_stress_v2",
			Val:  croshealthd.RoutineParamsV2{Routine: croshealthd.RoutineCPUStressV2},
			// TODO(b/295497926): Promote tast to critical
			ExtraAttr: []string{"informational"},
		}, {
			// Contact: kerker@google.com
			Name: "audio_driver",
			Val:  croshealthd.RoutineParamsV2{Routine: croshealthd.RoutineAudioDriver},
			// TODO(b/295499944): Promote tast to critical
			ExtraAttr: []string{"informational"},
		}, {
			// Contact: yycheng@google.com
			Name: "cpu_cache_v2",
			Val:  croshealthd.RoutineParamsV2{Routine: croshealthd.RoutineCPUCacheV2},
			// TODO(b/281766836): Promote tast to critical
			ExtraAttr: []string{"informational"},
		}, {
			// Contact: dennyh@google.com
			Name:              "ufs_lifetime",
			Val:               croshealthd.RoutineParamsV2{Routine: croshealthd.RoutineUFSLifetime},
			ExtraHardwareDeps: hwdep.D(hwdep.Ufs()),
		}, {
			// Contact: yycheng@google.com
			Name: "prime_search_v2",
			Val:  croshealthd.RoutineParamsV2{Routine: croshealthd.RoutinePrimeSearchV2},
		}, {
			// Contact: weiluanwang@google.com
			Name: "volume_button",
			Val:  croshealthd.RoutineParamsV2{Routine: croshealthd.RoutineVolumeButton},
		}, {
			// Contact: weiluanwang@google.com
			Name:              "led_lit_up",
			Val:               croshealthd.RoutineParamsV2{Routine: croshealthd.RoutineLedLitUp},
			ExtraHardwareDeps: hwdep.D(hwdep.ChromeEC()),
		}, {
			// Contact: yycheng@google.com
			Name: "floating_point_v2",
			Val:  croshealthd.RoutineParamsV2{Routine: croshealthd.RoutineFloatingPointV2},
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
		s.Fatalf("Unexpected routine status for %q: got %q; want Passed; output: %s",
			routine, result.Status, result.Output)
	}

	// Check to see that if the routine was run, the progress is 100%.
	if result.Progress != 100 {
		s.Fatalf("Unexpected progress value for %q routine with status %q: got %d; want 100; output: %s",
			routine, result.Status, result.Progress, result.Output)
	}
}
