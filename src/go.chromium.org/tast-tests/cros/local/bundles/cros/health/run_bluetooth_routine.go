// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package health tests the system daemon cros_healthd to ensure that telemetry
// and diagnostics calls can be completed successfully.
package health

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/local/croshealthd"
	"go.chromium.org/tast/core/testing"
)

type bluetoothRoutineTestParams struct {
	// Type of testing routine. Should be one of:
	//  * `croshealthd.RoutineBluetoothPower`
	//  * `croshealthd.RoutineBluetoothDiscovery`
	//  * `croshealthd.RoutineBluetoothScanning`
	BluetoothRoutineType string
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         RunBluetoothRoutine,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that cros_healthd can run Bluetooth routines",
		Contacts: []string{
			"cros-tdm-tpe-eng@google.com",
			"byronlee@google.com",
		},
		BugComponent: "b:982097", // ChromeOS > Platform > Enablement > Health
		SoftwareDeps: []string{"diagnostics"},
		Attr:         []string{"group:mainline"},
		TestBedDeps:  []string{tbdep.BluetoothStateNormal},
		Params: []testing.Param{{
			Name: "v1_power_bluez",
			Val: bluetoothRoutineTestParams{
				BluetoothRoutineType: croshealthd.RoutineBluetoothPower,
			},
			Fixture: "crosHealthdRunningAndBluetoothEnabledWithBlueZ",
			// TODO(b/363888266): Promote tast to critical
			ExtraAttr: []string{"informational", "group:criticalstaging"},
		}, {
			Name: "v1_discovery_bluez",
			Val: bluetoothRoutineTestParams{
				BluetoothRoutineType: croshealthd.RoutineBluetoothDiscovery,
			},
			Fixture: "crosHealthdRunningAndBluetoothEnabledWithBlueZ",
			// TODO(b/363888266): Promote tast to critical
			ExtraAttr: []string{"informational", "group:criticalstaging"},
		}, {
			Name: "v1_scanning_bluez",
			Val: bluetoothRoutineTestParams{
				BluetoothRoutineType: croshealthd.RoutineBluetoothScanning,
			},
			Fixture: "crosHealthdRunningAndBluetoothEnabledWithBlueZ",
			// TODO(b/363888266): Promote tast to critical
			ExtraAttr: []string{"informational", "group:criticalstaging"},
		}, {
			Name: "v1_power_floss",
			Val: bluetoothRoutineTestParams{
				BluetoothRoutineType: croshealthd.RoutineBluetoothPower,
			},
			Fixture: "crosHealthdRunningAndBluetoothEnabledWithFloss",
			// TODO(b/363888266): Promote tast to critical
			ExtraAttr: []string{"informational", "group:criticalstaging"},
		}, {
			Name: "v1_discovery_floss",
			Val: bluetoothRoutineTestParams{
				BluetoothRoutineType: croshealthd.RoutineBluetoothDiscovery,
			},
			Fixture: "crosHealthdRunningAndBluetoothEnabledWithFloss",
			// TODO(b/363888266): Promote tast to critical
			ExtraAttr: []string{"informational", "group:criticalstaging"},
		}, {
			Name: "v1_scanning_floss",
			Val: bluetoothRoutineTestParams{
				BluetoothRoutineType: croshealthd.RoutineBluetoothScanning,
			},
			Fixture: "crosHealthdRunningAndBluetoothEnabledWithFloss",
			// TODO(b/363888266): Promote tast to critical
			ExtraAttr: []string{"informational", "group:criticalstaging"},
		}},
	})
}

// RunBluetoothRoutine runs the Bluetooth related routines.
// TODO(b/363888266): Migrate health.DiagnosticsPassV2.bluetooth* to this test.
func RunBluetoothRoutine(ctx context.Context, s *testing.State) {
	param := s.Param().(bluetoothRoutineTestParams)

	result, err := croshealthd.RunDiagRoutine(ctx,
		croshealthd.NewRoutineParams(param.BluetoothRoutineType))
	if err != nil {
		s.Fatal("Unable to run routine: ", err)
	}

	if err := result.VerifyPassed(); err != nil {
		s.Fatal("Routine is not passed: ", err)
	}
}
