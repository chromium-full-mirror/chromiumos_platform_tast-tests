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
		BugComponent: "b:982097", // ChromeOS > Platform > Enablement > Health
		SoftwareDeps: []string{"diagnostics"},
		Attr:         []string{"group:mainline"},
		Fixture:      "crosHealthdRunning",
		// TODO(b/277548688): Monitor test results and promote stable tests to critical.
		Params: []testing.Param{{
			// Contact: byronlee@google.com
			Name:              "battery_capacity",
			Val:               croshealthd.NewRoutineParams(croshealthd.RoutineBatteryCapacity),
			ExtraAttr:         []string{"informational"},
			ExtraHardwareDeps: hwdep.D(hwdep.Battery()),
		}, {
			// Contact: byronlee@google.com
			Name:              "battery_health",
			Val:               croshealthd.NewRoutineParams(croshealthd.RoutineBatteryHealth),
			ExtraAttr:         []string{"informational"},
			ExtraHardwareDeps: hwdep.D(hwdep.Battery()),
		}, {
			// Contact: yycheng@google.com
			Name: "urandom",
			Val:  croshealthd.NewRoutineParams(croshealthd.RoutineURandom),
		}, {
			// Contact: dennyh@google.com
			Name:              "smartctl_check",
			Val:               croshealthd.NewRoutineParams(croshealthd.RoutineSmartctlCheck),
			ExtraAttr:         []string{"informational"},
			ExtraSoftwareDeps: []string{"smartctl"},
		}, {
			// Contact: yycheng@google.com
			Name:      "cpu_cache",
			Val:       croshealthd.NewRoutineParams(croshealthd.RoutineCPUCache),
			ExtraAttr: []string{"informational", "group:criticalstaging"},
		}, {
			// Contact: yycheng@google.com
			Name:      "cpu_stress",
			Val:       croshealthd.NewRoutineParams(croshealthd.RoutineCPUStress),
			ExtraAttr: []string{"informational", "group:criticalstaging"},
		}, {
			// Contact: yycheng@google.com
			Name: "floating_point_accuracy",
			Val:  croshealthd.NewRoutineParams(croshealthd.RoutineFloatingPointAccurary),
		}, {
			// Contact: dennyh@google.com
			Name:              "nvme_self_test",
			Val:               croshealthd.NewRoutineParams(croshealthd.RoutineNVMESelfTest),
			ExtraAttr:         []string{"informational"},
			Timeout:           3 * time.Minute,
			ExtraSoftwareDeps: []string{"nvme"},
			ExtraHardwareDeps: hwdep.D(hwdep.Nvme(), hwdep.NvmeSelfTest()),
		}, {
			// Contact: dennyh@google.com
			Name:      "nvme_wear_level",
			Val:       croshealthd.NewRoutineParams(croshealthd.RoutineNVMEWearLevel),
			ExtraAttr: []string{"informational"},
			// nvme_wear_level requires specific offsets in the nvme log that
			// are only currently defined for wilco devices.
			ExtraSoftwareDeps: []string{"nvme", "wilco"},
			ExtraHardwareDeps: hwdep.D(hwdep.Nvme()),
		}, {
			// Contact: yycheng@google.com
			Name: "prime_search",
			Val:  croshealthd.NewRoutineParams(croshealthd.RoutinePrimeSearch),
		}, {
			// Contact: weiluanwang@google.com
			Name: "lan_connectivity",
			Val:  croshealthd.NewRoutineParams(croshealthd.RoutineLanConnectivity),
		}, {
			// Contact: weiluanwang@google.com
			Name:      "gateway_can_be_pinged",
			Val:       croshealthd.NewRoutineParams(croshealthd.RoutineGatewayCanBePinged),
			ExtraAttr: []string{"informational"},
		}, {
			// Contact: weiluanwang@google.com
			Name: "dns_resolver_present",
			Val:  croshealthd.NewRoutineParams(croshealthd.RoutineDNSResolverPresent),
		}, {
			// Contact: weiluanwang@google.com
			Name: "dns_resolution",
			Val:  croshealthd.NewRoutineParams(croshealthd.RoutineDNSResolverPresent),
		}, {
			// Contact: yycheng@google.com
			Name:      "memory",
			Val:       croshealthd.NewRoutineParams(croshealthd.RoutineMemory),
			ExtraAttr: []string{"informational", "group:criticalstaging"},
		}, {
			// Contact: byronlee@google.com
			Name:      "sensitive_sensor",
			Val:       croshealthd.NewRoutineParams(croshealthd.RoutineSensitiveSensor),
			ExtraAttr: []string{"informational"},
		}, {
			// Contact: kerker@google.com
			Name:              "fingerprint",
			Val:               croshealthd.NewRoutineParams(croshealthd.RoutineFingerprint),
			ExtraAttr:         []string{"informational"},
			ExtraHardwareDeps: hwdep.D(hwdep.FingerprintDiagSupported()),
		}, {
			// Contact: kerker@google.com
			Name:              "fingerprint_alive",
			Val:               croshealthd.NewRoutineParams(croshealthd.RoutineFingerprintAlive),
			ExtraAttr:         []string{"informational"},
			ExtraHardwareDeps: hwdep.D(hwdep.FingerprintDiagSupported()),
		}, {
			// Contact: dennyh@google.com
			Name:              "emmc_lifetime",
			Val:               croshealthd.NewRoutineParams(croshealthd.RoutineEMMCLifetime),
			ExtraAttr:         []string{"informational"},
			ExtraHardwareDeps: hwdep.D(hwdep.Emmc()),
		}, {
			// Contact: kerker@google.com
			Name: "audio_set_volume",
			Val:  croshealthd.NewRoutineParams(croshealthd.RoutineAudioSetVolume),
		}, {
			// Contact: kerker@google.com
			Name: "audio_set_gain",
			Val:  croshealthd.NewRoutineParams(croshealthd.RoutineAudioSetGain),
		}, {
			// Contact: byronlee@google.com
			Name:              "bluetooth_power",
			Val:               croshealthd.NewRoutineParams(croshealthd.RoutineBluetoothPower),
			ExtraHardwareDeps: hwdep.D(hwdep.Bluetooth()),
		}, {
			// Contact: byronlee@google.com
			Name:              "bluetooth_discovery",
			Val:               croshealthd.NewRoutineParams(croshealthd.RoutineBluetoothDiscovery),
			ExtraHardwareDeps: hwdep.D(hwdep.Bluetooth()),
		}, {
			// Contact: byronlee@google.com
			Name:              "bluetooth_scanning",
			Val:               croshealthd.NewRoutineParams(croshealthd.RoutineBluetoothScanning),
			ExtraHardwareDeps: hwdep.D(hwdep.Bluetooth()),
		}, {
			// Contact: byronlee@google.com
			Name: "disk_read",
			Val:  croshealthd.NewRoutineParams(croshealthd.RoutineDiskRead),
		}, {
			// Contact: weiluanwang@google.com
			Name:      "power_button",
			Val:       croshealthd.NewRoutineParams(croshealthd.RoutinePowerButton),
			ExtraAttr: []string{"informational"},
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
