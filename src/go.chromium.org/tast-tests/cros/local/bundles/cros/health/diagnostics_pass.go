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
		Func:         DiagnosticsPass,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests that the cros_healthd diagnostic routines can pass",
		Contacts:     []string{"cros-tdm-tpe-eng@google.com"},
		BugComponent: "b:982097",
		SoftwareDeps: []string{"chrome", "diagnostics"},
		Attr:         []string{"group:mainline", "informational"},
		Fixture:      "crosHealthdRunning",
		// TODO(b/277548688): Monitor test results and promote stable tests to critical.
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
			Name:    "cpu_cache",
			Val:     croshealthd.NewRoutineParams(croshealthd.RoutineCPUCache),
			Timeout: 5 * time.Minute,
		}, {
			Name:    "cpu_stress",
			Val:     croshealthd.NewRoutineParams(croshealthd.RoutineCPUStress),
			Timeout: 5 * time.Minute,
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
			Name: "lan_connectivity",
			Val:  croshealthd.NewRoutineParams(croshealthd.RoutineLanConnectivity),
		}, {
			Name: "signal_strength",
			Val:  croshealthd.NewRoutineParams(croshealthd.RoutineSignalStrength),
		}, {
			Name: "gateway_can_be_pinged",
			Val:  croshealthd.NewRoutineParams(croshealthd.RoutineGatewayCanBePinged),
		}, {
			Name: "has_secure_wifi_connection",
			Val:  croshealthd.NewRoutineParams(croshealthd.RoutineHasSecureWifiConnection),
		}, {
			Name: "dns_resolver_present",
			Val:  croshealthd.NewRoutineParams(croshealthd.RoutineDNSResolverPresent),
		}, {
			Name: "dns_latency",
			Val:  croshealthd.NewRoutineParams(croshealthd.RoutineDNSLatency),
		}, {
			Name: "dns_resolution",
			Val:  croshealthd.NewRoutineParams(croshealthd.RoutineDNSResolverPresent),
		}, {
			Name: "captive_portal",
			Val:  croshealthd.NewRoutineParams(croshealthd.RoutineCaptivePortal),
		}, {
			Name: "http_firewall",
			Val:  croshealthd.NewRoutineParams(croshealthd.RoutineHTTPFirewall),
		}, {
			Name: "https_firewall",
			Val:  croshealthd.NewRoutineParams(croshealthd.RoutineHTTPSFirewall),
		}, {
			Name: "https_latency",
			Val:  croshealthd.NewRoutineParams(croshealthd.RoutineHTTPSLatency),
		}, {
			Name: "memory",
			Val:  croshealthd.NewRoutineParams(croshealthd.RoutineMemory),
		}, {
			Name: "sensitive_sensor",
			Val:  croshealthd.NewRoutineParams(croshealthd.RoutineSensitiveSensor),
		}, {
			Name: "fingerprint",
			Val:  croshealthd.NewRoutineParams(croshealthd.RoutineFingerprint),
			// Enabling this routine needs to configure the
			// cros_config. At this moment, only jinlon and drobit
			// are enabled.
			ExtraHardwareDeps: hwdep.D(hwdep.Model("jinlon", "drobit")),
		}, {
			Name: "fingerprint_alive",
			Val:  croshealthd.NewRoutineParams(croshealthd.RoutineFingerprintAlive),
			// Enabling this routine needs to configure the
			// cros_config. At this moment, only jinlon and drobit
			// are enabled.
			ExtraHardwareDeps: hwdep.D(hwdep.Model("jinlon", "drobit")),
		}, {
			Name:              "emmc_lifetime",
			Val:               croshealthd.NewRoutineParams(croshealthd.RoutineEMMCLifetime),
			ExtraHardwareDeps: hwdep.D(hwdep.Emmc()),
		}, {
			Name:              "led_lit_up",
			Val:               croshealthd.NewRoutineParams(croshealthd.RoutineLedLitUp),
			ExtraHardwareDeps: hwdep.D(hwdep.ChromeEC()),
		}, {
			Name: "audio_set_volume",
			Val:  croshealthd.NewRoutineParams(croshealthd.RoutineAudioSetVolume),
		}, {
			Name: "audio_set_gain",
			Val:  croshealthd.NewRoutineParams(croshealthd.RoutineAudioSetGain),
		}, {
			Name:              "bluetooth_power",
			Val:               croshealthd.NewRoutineParams(croshealthd.RoutineBluetoothPower),
			ExtraHardwareDeps: hwdep.D(hwdep.Bluetooth()),
		}, {
			Name:              "bluetooth_discovery",
			Val:               croshealthd.NewRoutineParams(croshealthd.RoutineBluetoothDiscovery),
			ExtraHardwareDeps: hwdep.D(hwdep.Bluetooth()),
		}, {
			Name:              "bluetooth_scanning",
			Val:               croshealthd.NewRoutineParams(croshealthd.RoutineBluetoothScanning),
			ExtraHardwareDeps: hwdep.D(hwdep.Bluetooth()),
		}, {
			Name: "disk_read",
			Val:  croshealthd.NewRoutineParams(croshealthd.RoutineDiskRead),
		}},
	})
}

// DiagnosticsPass is a paramaterized test that runs supported diagnostic
// routines through cros_healthd. The purpose of this test is to ensure that the
// routines can pass, which is a stricter version of DiagnosticsRun.* test.
func DiagnosticsPass(ctx context.Context, s *testing.State) {
	params := s.Param().(croshealthd.RoutineParams)
	routine := params.Routine
	s.Logf("Running routine: %s", routine)
	result, err := croshealthd.RunDiagRoutine(ctx, params)
	if err != nil {
		s.Fatalf("Unable to run %s routine: %s", routine, err)
	}

	if result.Status != croshealthd.StatusPassed {
		s.Fatalf("Unexpected routine status for %q routine : got %q; want Passed; message = %q",
			routine, result.Status, result.StatusMessage)
	}

	if result.Progress != 100 {
		s.Fatalf("Unexpected progress value for %q routine : got %d; want 100; message = %q",
			routine, result.Progress, result.StatusMessage)
	}
}
