// Copyright 2021 The ChromiumOS Authors
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
		Func:         DiagnosticsRun,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests that the cros_healthd diagnostic routines can be run without errors",
		Contacts:     []string{"cros-tdm-tpe-eng@google.com"},
		BugComponent: "b:982097", // ChromeOS > Platform > Enablement > Health
		SoftwareDeps: []string{"chrome", "diagnostics"},
		Attr:         []string{"group:mainline"},
		Fixture:      "crosHealthdRunning",
		Params: []testing.Param{{
			Name:              "battery_capacity",
			Val:               croshealthd.NewRoutineParams(croshealthd.RoutineBatteryCapacity),
			ExtraHardwareDeps: hwdep.D(hwdep.Battery()),
		}, {
			Name:              "battery_health",
			Val:               croshealthd.NewRoutineParams(croshealthd.RoutineBatteryHealth),
			ExtraHardwareDeps: hwdep.D(hwdep.Battery()),
		}, {
			Name: "urandom",
			Val:  croshealthd.NewRoutineParams(croshealthd.RoutineURandom),
		}, {
			Name:              "smartctl_check",
			Val:               croshealthd.NewRoutineParams(croshealthd.RoutineSmartctlCheck),
			ExtraSoftwareDeps: []string{"smartctl"},
		}, {
			Name: "cpu_cache",
			Val:  croshealthd.NewRoutineParams(croshealthd.RoutineCPUCache),
		}, {
			Name: "cpu_stress",
			Val:  croshealthd.NewRoutineParams(croshealthd.RoutineCPUStress),
		}, {
			Name: "floating_point_accuracy",
			Val:  croshealthd.NewRoutineParams(croshealthd.RoutineFloatingPointAccurary),
		}, {
			Name:              "nvme_self_test",
			Val:               croshealthd.NewRoutineParams(croshealthd.RoutineNVMESelfTest),
			Timeout:           3 * time.Minute,
			ExtraSoftwareDeps: []string{"nvme"},
			ExtraHardwareDeps: hwdep.D(hwdep.Nvme(), hwdep.NvmeSelfTest()),
		}, {
			Name: "nvme_wear_level",
			Val:  croshealthd.NewRoutineParams(croshealthd.RoutineNVMEWearLevel),
			// nvme_wear_level requires specific offsets in the nvme log that
			// are only currently defined for wilco devices.
			ExtraSoftwareDeps: []string{"nvme", "wilco"},
			ExtraHardwareDeps: hwdep.D(hwdep.Nvme()),
		}, {
			Name: "prime_search",
			Val:  croshealthd.NewRoutineParams(croshealthd.RoutinePrimeSearch),
		}, {
			// Cannot be added to DiagnosticsPass.* since the result would be
			// "Not run" in lab's network. See b/286497166.
			Name: "signal_strength",
			Val:  croshealthd.NewRoutineParams(croshealthd.RoutineSignalStrength),
		}, {
			Name: "gateway_can_be_pinged",
			Val:  croshealthd.NewRoutineParams(croshealthd.RoutineGatewayCanBePinged),
		}, {
			// Cannot be added to DiagnosticsPass.* since the result would be
			// "Not run" in lab's network. See b/286497147.
			Name: "has_secure_wifi_connection",
			Val:  croshealthd.NewRoutineParams(croshealthd.RoutineHasSecureWifiConnection),
		}, {
			// Cannot be added to DiagnosticsPass.* since that requires the
			// routine to be run in a good network environment. The
			// DiagnosticsPass.* counterpart will be flaky in a normal lab.
			Name: "dns_latency",
			Val:  croshealthd.NewRoutineParams(croshealthd.RoutineDNSLatency),
		}, {
			Name: "http_firewall",
			Val:  croshealthd.NewRoutineParams(croshealthd.RoutineHTTPFirewall),
			// TODO(b/281464322): Promote to critical.
			ExtraAttr: []string{"informational"},
		}, {
			Name: "https_firewall",
			Val:  croshealthd.NewRoutineParams(croshealthd.RoutineHTTPSFirewall),
			// TODO(b/281464322): Promote to critical.
			ExtraAttr: []string{"informational"},
		}, {
			// Cannot be added to DiagnosticsPass.* since that requires the
			// routine to be run in a good network environment. The
			// DiagnosticsPass.* counterpart will be flaky in a normal lab.
			Name: "https_latency",
			Val:  croshealthd.NewRoutineParams(croshealthd.RoutineHTTPSLatency),
		}, {
			Name: "memory",
			Val:  croshealthd.NewRoutineParams(croshealthd.RoutineMemory),
			// TODO(b/279849842): Promote to critical.
			ExtraAttr: []string{"informational"},
		}, {
			Name: "sensitive_sensor",
			Val:  croshealthd.NewRoutineParams(croshealthd.RoutineSensitiveSensor),
			// TODO(b/280388091): Promote to critical.
			ExtraAttr: []string{"informational", "group:criticalstaging"},
		}, {
			Name: "fingerprint",
			Val:  croshealthd.NewRoutineParams(croshealthd.RoutineFingerprint),
			// Enabling this routine needs to configure the
			// cros_config. At this moment, only jinlon and drobit
			// are enabled.
			ExtraHardwareDeps: hwdep.D(hwdep.Model("jinlon", "drobit")),
			// TODO(b/279374234): Promote to critical.
			ExtraAttr: []string{"informational"},
		}, {
			Name: "fingerprint_alive",
			Val:  croshealthd.NewRoutineParams(croshealthd.RoutineFingerprintAlive),
			// Enabling this routine needs to configure the
			// cros_config. At this moment, only jinlon and drobit
			// are enabled.
			ExtraHardwareDeps: hwdep.D(hwdep.Model("jinlon", "drobit")),
			// TODO(b/279374234): Promote to critical.
			ExtraAttr: []string{"informational"},
		}, {
			Name:              "emmc_lifetime",
			Val:               croshealthd.NewRoutineParams(croshealthd.RoutineEMMCLifetime),
			ExtraHardwareDeps: hwdep.D(hwdep.Emmc()),
			// TODO(b/279707249): Promote to critical.
			ExtraAttr: []string{"informational", "group:criticalstaging"},
		}, {
			Name: "power_button",
			Val:  croshealthd.NewRoutineParams(croshealthd.RoutinePowerButton),
		}},
	})
}

// DiagnosticsRun is a paramaterized test that runs supported diagnostic
// routines through cros_healthd. The purpose of this test is to ensure that the
// routines can be run without errors, and not to check if the routines pass or
// fail.
func DiagnosticsRun(ctx context.Context, s *testing.State) {
	params := s.Param().(croshealthd.RoutineParams)
	routine := params.Routine
	s.Logf("Running routine: %s", routine)
	result, err := croshealthd.RunDiagRoutine(ctx, params)
	if err != nil {
		s.Fatalf("Unable to run %s routine: %s", routine, err)
	}

	// Test a given routine and ensure that it can complete successfully without
	// crashing or throwing errors. For example, some lab machines might have
	// old batteries that would fail the diagnostic routines, but this should
	// not fail the Tast test.
	if result.Status != croshealthd.StatusPassed &&
		result.Status != croshealthd.StatusFailed &&
		result.Status != croshealthd.StatusNotRun {
		s.Fatalf("Unexpected routine status for %q: got %q; want %q, %q, or %q; message = %q",
			routine, result.Status, croshealthd.StatusPassed, croshealthd.StatusFailed, croshealthd.StatusNotRun, result.StatusMessage)
	}

	// Check to see that if the routine was run, the progress is 100%
	if result.Progress != 100 && result.Status != croshealthd.StatusNotRun {
		s.Fatalf("Unexpected progress value for %q routine with status %q: got %d; want 100; message = %q",
			routine, result.Status, result.Progress, result.StatusMessage)
	}
}
