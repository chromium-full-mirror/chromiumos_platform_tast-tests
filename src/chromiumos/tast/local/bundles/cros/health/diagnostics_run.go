// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package health tests the system daemon cros_healthd to ensure that telemetry
// and diagnostics calls can be completed successfully.
package health

import (
	"context"
	"time"

	"chromiumos/tast/local/croshealthd"

	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

// newRoutineParams creates and returns a diagnostic routine with default test
// parameters.
func newRoutineParams(routine string) croshealthd.RoutineParams {
	return croshealthd.RoutineParams{
		Routine:                       routine,
		Cancel:                        false,
		DefaultNVMEWearLevelThreshold: 50,
	}
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         DiagnosticsRun,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests that the cros_healthd diagnostic routines can be run without errors",
		Contacts:     []string{"cros-tdm-tpe-eng@google.com"},
		BugComponent: "b:982097",
		SoftwareDeps: []string{"chrome", "diagnostics"},
		Attr:         []string{"group:mainline"},
		Fixture:      "crosHealthdRunning",
		Params: []testing.Param{{
			Name:              "battery_capacity",
			Val:               newRoutineParams(croshealthd.RoutineBatteryCapacity),
			ExtraHardwareDeps: hwdep.D(hwdep.Battery()),
		}, {
			Name:              "battery_health",
			Val:               newRoutineParams(croshealthd.RoutineBatteryHealth),
			ExtraHardwareDeps: hwdep.D(hwdep.Battery()),
		}, {
			Name: "urandom",
			Val:  newRoutineParams(croshealthd.RoutineURandom),
		}, {
			Name:              "smartctl_check",
			Val:               newRoutineParams(croshealthd.RoutineSmartctlCheck),
			ExtraSoftwareDeps: []string{"smartctl"},
		}, {
			Name:    "cpu_cache",
			Val:     newRoutineParams(croshealthd.RoutineCPUCache),
			Timeout: 5 * time.Minute,
		}, {
			Name:    "cpu_stress",
			Val:     newRoutineParams(croshealthd.RoutineCPUStress),
			Timeout: 5 * time.Minute,
		}, {
			Name: "floating_point_accuracy",
			Val:  newRoutineParams(croshealthd.RoutineFloatingPointAccurary),
		}, {
			Name:              "nvme_self_test",
			Val:               newRoutineParams(croshealthd.RoutineNVMESelfTest),
			Timeout:           3 * time.Minute,
			ExtraSoftwareDeps: []string{"nvme"},
			ExtraHardwareDeps: hwdep.D(hwdep.Nvme(), hwdep.NvmeSelfTest()),
		}, {
			Name: "nvme_wear_level",
			Val:  newRoutineParams(croshealthd.RoutineNVMEWearLevel),
			// nvme_wear_level requires specific offsets in the nvme log that
			// are only currently defined for wilco devices.
			ExtraSoftwareDeps: []string{"nvme", "wilco"},
			ExtraHardwareDeps: hwdep.D(hwdep.Nvme()),
		}, {
			Name: "prime_search",
			Val:  newRoutineParams(croshealthd.RoutinePrimeSearch),
		}, {
			Name: "lan_connectivity",
			Val:  newRoutineParams(croshealthd.RoutineLanConnectivity),
		}, {
			Name: "signal_strength",
			Val:  newRoutineParams(croshealthd.RoutineSignalStrength),
		}, {
			Name: "gateway_can_be_pinged",
			Val:  newRoutineParams(croshealthd.RoutineGatewayCanBePinged),
		}, {
			Name: "has_secure_wifi_connection",
			Val:  newRoutineParams(croshealthd.RoutineHasSecureWifiConnection),
		}, {
			Name: "dns_resolver_present",
			Val:  newRoutineParams(croshealthd.RoutineDNSResolverPresent),
		}, {
			Name: "dns_latency",
			Val:  newRoutineParams(croshealthd.RoutineDNSLatency),
		}, {
			Name: "dns_resolution",
			Val:  newRoutineParams(croshealthd.RoutineDNSResolverPresent),
		}, {
			Name: "captive_portal",
			Val:  newRoutineParams(croshealthd.RoutineCaptivePortal),
		}, {
			Name: "http_firewall",
			Val:  newRoutineParams(croshealthd.RoutineHTTPFirewall),
		}, {
			Name: "https_firewall",
			Val:  newRoutineParams(croshealthd.RoutineHTTPSFirewall),
		}, {
			Name: "https_latency",
			Val:  newRoutineParams(croshealthd.RoutineHTTPSLatency),
		}, {
			Name: "memory",
			Val:  newRoutineParams(croshealthd.RoutineMemory),
			// TODO(b/279849842): Promote to critical.
			ExtraAttr: []string{"informational"},
		}, {
			Name: "sensitive_sensor",
			Val:  newRoutineParams(croshealthd.RoutineSensitiveSensor),
			// TODO(b/280388091): Promote to critical.
			ExtraAttr: []string{"informational"},
		}, {
			Name: "fingerprint",
			Val:  newRoutineParams(croshealthd.RoutineFingerprint),
			// Enabling this routine needs to configure the
			// cros_config. At this moment, only jinlon and drobit
			// are enabled.
			ExtraHardwareDeps: hwdep.D(hwdep.Model("jinlon", "drobit")),
			// TODO(b/279374234): Promote to critical.
			ExtraAttr: []string{"informational"},
		}, {
			Name: "fingerprint_alive",
			Val:  newRoutineParams(croshealthd.RoutineFingerprintAlive),
			// Enabling this routine needs to configure the
			// cros_config. At this moment, only jinlon and drobit
			// are enabled.
			ExtraHardwareDeps: hwdep.D(hwdep.Model("jinlon", "drobit")),
			// TODO(b/279374234): Promote to critical.
			ExtraAttr: []string{"informational"},
		}, {
			Name:              "emmc_lifetime",
			Val:               newRoutineParams(croshealthd.RoutineEMMCLifetime),
			ExtraHardwareDeps: hwdep.D(hwdep.Emmc()),
			// TODO(b/279707249): Promote to critical.
			ExtraAttr: []string{"informational"},
		}, {
			Name:              "led_lit_up",
			Val:               newRoutineParams(croshealthd.RoutineLedLitUp),
			ExtraHardwareDeps: hwdep.D(hwdep.ChromeEC()),
		}, {
			Name: "audio_set_volume",
			Val:  newRoutineParams(croshealthd.RoutineAudioSetVolume),
			// TODO(b/279670424): Promote to critical.
			ExtraAttr: []string{"informational"},
		}, {
			Name: "audio_set_gain",
			Val:  newRoutineParams(croshealthd.RoutineAudioSetGain),
			// TODO(b/279670424): Promote to critical.
			ExtraAttr: []string{"informational"},
		}, {
			Name: "bluetooth_power",
			Val:  newRoutineParams(croshealthd.RoutineBluetoothPower),
			// TODO(b/280388009): Promote to critical.
			ExtraAttr:         []string{"informational"},
			ExtraHardwareDeps: hwdep.D(hwdep.Bluetooth()),
		}, {
			Name: "bluetooth_discovery",
			Val:  newRoutineParams(croshealthd.RoutineBluetoothDiscovery),
			// TODO(b/280388009): Promote to critical.
			ExtraAttr:         []string{"informational"},
			ExtraHardwareDeps: hwdep.D(hwdep.Bluetooth()),
		}, {
			Name: "bluetooth_scanning",
			Val:  newRoutineParams(croshealthd.RoutineBluetoothScanning),
			// TODO(b/280388009): Promote to critical.
			ExtraAttr:         []string{"informational"},
			ExtraHardwareDeps: hwdep.D(hwdep.Bluetooth()),
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
