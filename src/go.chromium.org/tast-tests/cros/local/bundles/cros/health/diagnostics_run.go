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
		SoftwareDeps: []string{"diagnostics"},
		Attr:         []string{"group:mainline"},
		Params: []testing.Param{{
			// Contact: byronlee@google.com
			Name:              "battery_capacity",
			Val:               croshealthd.NewRoutineParams(croshealthd.RoutineBatteryCapacity),
			Fixture:           "crosHealthdRunning",
			ExtraHardwareDeps: hwdep.D(hwdep.Battery()),
		}, {
			// Contact: byronlee@google.com
			Name:              "battery_health",
			Val:               croshealthd.NewRoutineParams(croshealthd.RoutineBatteryHealth),
			Fixture:           "crosHealthdRunning",
			ExtraHardwareDeps: hwdep.D(hwdep.Battery()),
		}, {
			// Contact: dennyh@google.com
			Name:              "smartctl_check",
			Val:               croshealthd.NewRoutineParams(croshealthd.RoutineSmartctlCheck),
			Fixture:           "crosHealthdRunning",
			ExtraSoftwareDeps: []string{"smartctl"},
			ExtraHardwareDeps: hwdep.D(hwdep.Nvme()),
		}, {
			// Contact: yycheng@google.com
			Name:    "cpu_stress",
			Val:     croshealthd.NewRoutineParams(croshealthd.RoutineCPUStress),
			Fixture: "crosHealthdRunningAndRebootDUT",
		}, {
			// Contact: dennyh@google.com
			Name:              "nvme_self_test",
			Val:               croshealthd.NewRoutineParams(croshealthd.RoutineNVMESelfTest),
			Fixture:           "crosHealthdRunning",
			Timeout:           3 * time.Minute,
			ExtraSoftwareDeps: []string{"nvme"},
			ExtraHardwareDeps: hwdep.D(hwdep.Nvme(), hwdep.NvmeSelfTest()),
		}, {
			// Contact: dennyh@google.com
			Name:    "nvme_wear_level",
			Val:     croshealthd.NewRoutineParams(croshealthd.RoutineNVMEWearLevel),
			Fixture: "crosHealthdRunning",
			// nvme_wear_level requires specific offsets in the nvme log that
			// are only currently defined for wilco devices.
			ExtraSoftwareDeps: []string{"nvme", "wilco"},
			ExtraHardwareDeps: hwdep.D(hwdep.Nvme()),
		}, {
			// Contact: weiluanwang@google.com
			Name:      "captive_portal",
			Val:       croshealthd.NewRoutineParams(croshealthd.RoutineCaptivePortal),
			Fixture:   "crosHealthdRunning",
			ExtraAttr: []string{"informational"},
		}, {
			// Contact: weiluanwang@google.com
			// Cannot be added to DiagnosticsPass.* since the result would be
			// "Not run" in lab's network. See b/286497166.
			Name:    "signal_strength",
			Val:     croshealthd.NewRoutineParams(croshealthd.RoutineSignalStrength),
			Fixture: "crosHealthdRunning",
		}, {
			// Contact: weiluanwang@google.com
			Name:    "gateway_can_be_pinged",
			Val:     croshealthd.NewRoutineParams(croshealthd.RoutineGatewayCanBePinged),
			Fixture: "crosHealthdRunning",
		}, {
			// Contact: weiluanwang@google.com
			// Cannot be added to DiagnosticsPass.* since the result would be
			// "Not run" in lab's network. See b/286497147.
			Name:    "has_secure_wifi_connection",
			Val:     croshealthd.NewRoutineParams(croshealthd.RoutineHasSecureWifiConnection),
			Fixture: "crosHealthdRunning",
		}, {
			// Contact: weiluanwang@google.com
			// Cannot be added to DiagnosticsPass.* since that requires the
			// routine to be run in a good network environment. The
			// DiagnosticsPass.* counterpart will be flaky in a normal lab.
			Name:    "dns_latency",
			Val:     croshealthd.NewRoutineParams(croshealthd.RoutineDNSLatency),
			Fixture: "crosHealthdRunning",
		}, {
			// Contact: weiluanwang@google.com
			Name:    "http_firewall",
			Val:     croshealthd.NewRoutineParams(croshealthd.RoutineHTTPFirewall),
			Fixture: "crosHealthdRunning",
			// TODO(b/281464322): Promote to critical.
			ExtraAttr: []string{"informational"},
		}, {
			// Contact: weiluanwang@google.com
			Name:    "https_firewall",
			Val:     croshealthd.NewRoutineParams(croshealthd.RoutineHTTPSFirewall),
			Fixture: "crosHealthdRunning",
			// TODO(b/281464322): Promote to critical.
			ExtraAttr: []string{"informational"},
		}, {
			// Contact: weiluanwang@google.com
			// Cannot be added to DiagnosticsPass.* since that requires the
			// routine to be run in a good network environment. The
			// DiagnosticsPass.* counterpart will be flaky in a normal lab.
			Name:    "https_latency",
			Val:     croshealthd.NewRoutineParams(croshealthd.RoutineHTTPSLatency),
			Fixture: "crosHealthdRunning",
		}, {
			// Contact: byronlee@google.com
			Name:    "sensitive_sensor",
			Val:     croshealthd.NewRoutineParams(croshealthd.RoutineSensitiveSensor),
			Fixture: "crosHealthdRunning",
			// TODO(b/280388091): Promote to critical.
			ExtraAttr: []string{"informational"},
		}, {
			// Contact: dennyh@google.com
			Name:              "emmc_lifetime",
			Val:               croshealthd.NewRoutineParams(croshealthd.RoutineEMMCLifetime),
			Fixture:           "crosHealthdRunning",
			ExtraHardwareDeps: hwdep.D(hwdep.Emmc(), hwdep.SkipOnModel("faffy")),
			// TODO(b/279707249): Promote to critical.
			ExtraAttr: []string{"informational", "group:criticalstaging"},
		}, {
			// Contact: dennyh@google.com
			// TODO(b/324001664): Fix the issue on faffy.
			Name:              "emmc_lifetime_unstable",
			Val:               croshealthd.NewRoutineParams(croshealthd.RoutineEMMCLifetime),
			Fixture:           "crosHealthdRunning",
			ExtraHardwareDeps: hwdep.D(hwdep.Emmc(), hwdep.Model("faffy")),
			ExtraAttr:         []string{"informational"},
		}},
	})
}

// DiagnosticsRun is a paramaterized test that runs supported diagnostic
// routines through cros_healthd. The purpose of this test is to ensure that the
// routines can be run without errors, and not to check if the routines pass or
// fail.
func DiagnosticsRun(ctx context.Context, s *testing.State) {
	params := s.Param().(croshealthd.RoutineParams)
	result, err := croshealthd.RunDiagRoutine(ctx, params)
	if err != nil {
		s.Fatalf("Unable to run routine: %s", err)
	}
	if err := result.VerifyFinished(); err != nil {
		s.Fatalf("Routine is not finished: %s", err)
	}
}
