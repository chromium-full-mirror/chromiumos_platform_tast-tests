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
		Func:         DiagnosticsPassV2,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests that the cros_healthd diagnostic routines V2 can pass",
		Contacts:     []string{"cros-tdm-tpe-eng@google.com"},
		BugComponent: "b:982097", // ChromeOS > Platform > Enablement > Health
		SoftwareDeps: []string{"diagnostics"},
		Attr:         []string{"group:mainline"},
		Params: []testing.Param{{
			// Contact: yycheng@google.com
			Name:    "memory_v2",
			Val:     croshealthd.RoutineParamsV2{Routine: croshealthd.RoutineMemoryV2},
			Fixture: "crosHealthdRunningAndRebootDUT",
		}, {
			// Contact: yycheng@google.com
			Name:    "cpu_stress_v2",
			Val:     croshealthd.RoutineParamsV2{Routine: croshealthd.RoutineCPUStressV2},
			Fixture: "crosHealthdRunningAndRebootDUT",
		}, {
			// Contact: kerker@google.com
			Name:    "audio_driver",
			Val:     croshealthd.RoutineParamsV2{Routine: croshealthd.RoutineAudioDriver},
			Fixture: "crosHealthdRunning",
			// TODO(b/295499944): Promote tast to critical
			ExtraAttr: []string{"informational"},
		}, {
			// Contact: yycheng@google.com
			Name:    "cpu_cache_v2",
			Val:     croshealthd.RoutineParamsV2{Routine: croshealthd.RoutineCPUCacheV2},
			Fixture: "crosHealthdRunningAndRebootDUT",
		}, {
			// Contact: yycheng@google.com
			Name:    "prime_search_v2",
			Val:     croshealthd.RoutineParamsV2{Routine: croshealthd.RoutinePrimeSearchV2},
			Fixture: "crosHealthdRunning",
		}, {
			// Contact: yycheng@google.com
			Name:    "floating_point_v2",
			Val:     croshealthd.RoutineParamsV2{Routine: croshealthd.RoutineFloatingPointV2},
			Fixture: "crosHealthdRunning",
		}, {
			// Contact: byronlee@google.com
			Name: "bluetooth_power",
			Val:  croshealthd.RoutineParamsV2{Routine: croshealthd.RoutineBluetoothPowerV2},
			// Bluetooth v2 routines are only supported when Floss is enabled.
			Fixture: "crosHealthdRunningAndBluetoothEnabledWithFloss",
			// TODO(b/303370425): Promote tast to critical
			ExtraAttr:         []string{"informational"},
			ExtraSoftwareDeps: []string{"bluetooth_floss"},
		}, {
			// Contact: byronlee@google.com
			Name: "bluetooth_discovery",
			Val:  croshealthd.RoutineParamsV2{Routine: croshealthd.RoutineBluetoothDiscoveryV2},
			// Bluetooth v2 routines are only supported when Floss is enabled.
			Fixture: "crosHealthdRunningAndBluetoothEnabledWithFloss",
			// TODO(b/303370425): Promote tast to critical
			ExtraAttr:         []string{"informational"},
			ExtraSoftwareDeps: []string{"bluetooth_floss"},
		}, {
			// Contact: byronlee@google.com
			Name: "bluetooth_scanning",
			Val:  croshealthd.RoutineParamsV2{Routine: croshealthd.RoutineBluetoothScanningV2},
			// Bluetooth v2 routines are only supported when Floss is enabled.
			Fixture: "crosHealthdRunningAndBluetoothEnabledWithFloss",
			// TODO(b/303370425): Promote tast to critical
			ExtraAttr:         []string{"informational"},
			ExtraSoftwareDeps: []string{"bluetooth_floss"},
		}}})
}

// DiagnosticsPassV2 is a paramaterized test that runs supported diagnostic
// routines using the V2 API through cros_healthd. The purpose of this test is
// to ensure that the routines can pass, which is a stricter version of
// DiagnosticsRunV2.* test.
func DiagnosticsPassV2(ctx context.Context, s *testing.State) {
	params := s.Param().(croshealthd.RoutineParamsV2)
	config := croshealthd.RoutineTestingConfigV2{
		ArgsBuilder:    croshealthd.CreateLegacyRoutineV2ArgsBuilder(params),
		RoutineRunner:  croshealthd.RunDiagV2,
		ResultVerifier: croshealthd.VerifyRoutinePassedV2,
	}
	if err := croshealthd.TestDiagRoutineV2(ctx, config); err != nil {
		s.Fatal("Routine verification failed: ", err)
	}
}
