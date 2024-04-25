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
		// TODO(b/277548688): Monitor test results and promote stable tests to critical.
		Params: []testing.Param{{
			// Contact: byronlee@google.com
			Name:              "battery_capacity",
			Val:               croshealthd.NewRoutineParams(croshealthd.RoutineBatteryCapacity),
			Fixture:           "crosHealthdRunning",
			ExtraAttr:         []string{"informational"},
			ExtraHardwareDeps: hwdep.D(hwdep.Battery()),
		}, {
			// Contact: byronlee@google.com
			Name:              "battery_health",
			Val:               croshealthd.NewRoutineParams(croshealthd.RoutineBatteryHealth),
			Fixture:           "crosHealthdRunning",
			ExtraAttr:         []string{"informational"},
			ExtraHardwareDeps: hwdep.D(hwdep.Battery()),
		}, {
			// Contact: yycheng@google.com
			Name:    "urandom",
			Val:     croshealthd.NewRoutineParams(croshealthd.RoutineURandom),
			Fixture: "crosHealthdRunning",
		}, {
			// Contact: dennyh@google.com
			Name:              "smartctl_check",
			Val:               croshealthd.NewRoutineParams(croshealthd.RoutineSmartctlCheck),
			Fixture:           "crosHealthdRunning",
			ExtraAttr:         []string{"informational"},
			ExtraSoftwareDeps: []string{"smartctl"},
			ExtraHardwareDeps: hwdep.D(hwdep.Nvme()),
		}, {
			// Contact: yycheng@google.com
			Name:    "cpu_cache",
			Val:     croshealthd.NewRoutineParams(croshealthd.RoutineCPUCache),
			Fixture: "crosHealthdRunningAndRebootDUT",
		}, {
			// Contact: yycheng@google.com
			Name:    "cpu_stress",
			Val:     croshealthd.NewRoutineParams(croshealthd.RoutineCPUStress),
			Fixture: "crosHealthdRunningAndRebootDUT",
		}, {
			// Contact: yycheng@google.com
			Name:    "floating_point_accuracy",
			Val:     croshealthd.NewRoutineParams(croshealthd.RoutineFloatingPointAccurary),
			Fixture: "crosHealthdRunning",
		}, {
			// Contact: dennyh@google.com
			Name:              "nvme_self_test",
			Val:               croshealthd.NewRoutineParams(croshealthd.RoutineNVMESelfTest),
			Fixture:           "crosHealthdRunning",
			ExtraAttr:         []string{"informational"},
			Timeout:           3 * time.Minute,
			ExtraSoftwareDeps: []string{"nvme"},
			ExtraHardwareDeps: hwdep.D(hwdep.Nvme(), hwdep.NvmeSelfTest()),
		}, {
			// Contact: yycheng@google.com
			Name:    "prime_search",
			Val:     croshealthd.NewRoutineParams(croshealthd.RoutinePrimeSearch),
			Fixture: "crosHealthdRunning",
		}, {
			// Contact: weiluanwang@google.com
			Name:    "lan_connectivity",
			Val:     croshealthd.NewRoutineParams(croshealthd.RoutineLanConnectivity),
			Fixture: "crosHealthdRunning",
		}, {
			// Contact: weiluanwang@google.com
			Name:      "gateway_can_be_pinged",
			Val:       croshealthd.NewRoutineParams(croshealthd.RoutineGatewayCanBePinged),
			Fixture:   "crosHealthdRunning",
			ExtraAttr: []string{"informational"},
		}, {
			// Contact: weiluanwang@google.com
			Name:    "dns_resolver_present",
			Val:     croshealthd.NewRoutineParams(croshealthd.RoutineDNSResolverPresent),
			Fixture: "crosHealthdRunning",
		}, {
			// Contact: weiluanwang@google.com
			Name:    "dns_resolution",
			Val:     croshealthd.NewRoutineParams(croshealthd.RoutineDNSResolverPresent),
			Fixture: "crosHealthdRunning",
		}, {
			// Contact: yycheng@google.com
			Name:    "memory",
			Val:     croshealthd.NewRoutineParams(croshealthd.RoutineMemory),
			Fixture: "crosHealthdRunningAndRebootDUT",
		}, {
			// Contact: byronlee@google.com
			Name:      "sensitive_sensor",
			Val:       croshealthd.NewRoutineParams(croshealthd.RoutineSensitiveSensor),
			Fixture:   "crosHealthdRunning",
			ExtraAttr: []string{"informational"},
		}, {
			// Contact: kerker@google.com
			Name:              "fingerprint",
			Val:               croshealthd.NewRoutineParams(croshealthd.RoutineFingerprint),
			Fixture:           "crosHealthdRunning",
			ExtraHardwareDeps: hwdep.D(hwdep.FingerprintDiagSupported()),
		}, {
			// Contact: kerker@google.com
			Name:              "fingerprint_alive",
			Val:               croshealthd.NewRoutineParams(croshealthd.RoutineFingerprintAlive),
			Fixture:           "crosHealthdRunning",
			ExtraHardwareDeps: hwdep.D(hwdep.FingerprintDiagSupported()),
		}, {
			// Contact: dennyh@google.com
			Name:              "emmc_lifetime",
			Val:               croshealthd.NewRoutineParams(croshealthd.RoutineEMMCLifetime),
			Fixture:           "crosHealthdRunning",
			ExtraAttr:         []string{"informational"},
			ExtraHardwareDeps: hwdep.D(hwdep.Emmc(), hwdep.SkipOnModel("faffy")),
		}, {
			// Contact: dennyh@google.com
			// TODO(b/324001664): Fix the issue on faffy.
			Name:              "emmc_lifetime_unstable",
			Val:               croshealthd.NewRoutineParams(croshealthd.RoutineEMMCLifetime),
			Fixture:           "crosHealthdRunning",
			ExtraAttr:         []string{"informational"},
			ExtraHardwareDeps: hwdep.D(hwdep.Emmc(), hwdep.Model("faffy")),
		}, {
			// Contact: byronlee@google.com
			Name:              "bluetooth_power",
			Val:               croshealthd.NewRoutineParams(croshealthd.RoutineBluetoothPower),
			Fixture:           "crosHealthdRunning",
			ExtraHardwareDeps: hwdep.D(hwdep.Bluetooth()),
		}, {
			// Contact: byronlee@google.com
			Name:              "bluetooth_discovery",
			Val:               croshealthd.NewRoutineParams(croshealthd.RoutineBluetoothDiscovery),
			Fixture:           "crosHealthdRunning",
			ExtraHardwareDeps: hwdep.D(hwdep.Bluetooth()),
		}, {
			// Contact: byronlee@google.com
			Name:              "bluetooth_scanning",
			Val:               croshealthd.NewRoutineParams(croshealthd.RoutineBluetoothScanning),
			Fixture:           "crosHealthdRunning",
			ExtraHardwareDeps: hwdep.D(hwdep.Bluetooth()),
		}, {
			// Contact: byronlee@google.com
			Name:    "disk_read",
			Val:     croshealthd.NewRoutineParams(croshealthd.RoutineDiskRead),
			Fixture: "crosHealthdRunning",
		}, {
			// Contact: weiluanwang@google.com
			Name:    "power_button",
			Val:     croshealthd.NewRoutineParams(croshealthd.RoutinePowerButton),
			Fixture: "crosHealthdRunning",
		}},
	})
}

// DiagnosticsPass is a paramaterized test that runs supported diagnostic
// routines through cros_healthd. The purpose of this test is to ensure that the
// routines can pass, which is a stricter version of DiagnosticsRun.* test.
func DiagnosticsPass(ctx context.Context, s *testing.State) {
	params := s.Param().(croshealthd.RoutineParams)
	result, err := croshealthd.RunDiagRoutine(ctx, params)
	if err != nil {
		s.Fatalf("Unable to run routine: %s", err)
	}
	if err := result.VerifyPassed(); err != nil {
		s.Fatalf("Routine is not passed: %s", err)
	}
}
