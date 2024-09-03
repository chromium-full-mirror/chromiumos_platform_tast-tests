// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package health tests the system daemon cros_healthd to ensure that telemetry
// and diagnostics calls can be completed successfully.
package health

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/local/croshealthd"
	"go.chromium.org/tast/core/testing"
)

type bluetoothRoutineTestParams struct {
	// Type of testing routine. Should be one of:
	//  * `croshealthd.RoutineBluetoothPower`
	//  * `croshealthd.RoutineBluetoothDiscovery`
	//  * `croshealthd.RoutineBluetoothScanning`
	//  * `croshealthd.RoutineBluetoothPowerV2`
	//  * `croshealthd.RoutineBluetoothDiscoveryV2`
	//  * `croshealthd.RoutineBluetoothScanningV2`
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
		}, {
			Name: "v2_power",
			Val: bluetoothRoutineTestParams{
				BluetoothRoutineType: croshealthd.RoutineBluetoothPowerV2,
			},
			// Bluetooth v2 routines are only supported when Floss is enabled.
			Fixture: "crosHealthdRunningAndBluetoothEnabledWithFloss",
			// TODO(b/363888266): Promote tast to critical
			ExtraAttr: []string{"informational", "group:criticalstaging"},
		}, {
			Name: "v2_discovery",
			Val: bluetoothRoutineTestParams{
				BluetoothRoutineType: croshealthd.RoutineBluetoothDiscoveryV2,
			},
			// Bluetooth v2 routines are only supported when Floss is enabled.
			Fixture: "crosHealthdRunningAndBluetoothEnabledWithFloss",
		}, {
			Name: "v2_scanning",
			Val: bluetoothRoutineTestParams{
				BluetoothRoutineType: croshealthd.RoutineBluetoothScanningV2,
			},
			// Bluetooth v2 routines are only supported when Floss is enabled.
			Fixture: "crosHealthdRunningAndBluetoothEnabledWithFloss",
		}},
	})
}

func createBuilder(routine string) func(context.Context) ([]string, error) {
	return func(context.Context) ([]string, error) {
		return []string{routine}, nil
	}
}

func isBluetoothV2Routine(routine string) bool {
	return routine == croshealthd.RoutineBluetoothPowerV2 ||
		routine == croshealthd.RoutineBluetoothDiscoveryV2 ||
		routine == croshealthd.RoutineBluetoothScanningV2
}

// RunBluetoothRoutine runs the Bluetooth related routines.
func RunBluetoothRoutine(ctx context.Context, s *testing.State) {
	param := s.Param().(bluetoothRoutineTestParams)

	if isBluetoothV2Routine(param.BluetoothRoutineType) {
		config := croshealthd.RoutineTestingConfigV2{
			ArgsBuilder:    createBuilder(param.BluetoothRoutineType),
			RoutineRunner:  croshealthd.RunDiagV2,
			ResultVerifier: croshealthd.VerifyRoutinePassedV2,
		}
		if err := croshealthd.TestDiagRoutineV2(ctx, config); err != nil {
			// TODO(b/362645235): Remove the sleep used for debugging b/362645235.
			if param.BluetoothRoutineType == croshealthd.RoutineBluetoothPowerV2 {
				// GoBigSleepLint: Ensure that adapter added event is missing at the end of Bluetooth power routine.
				testing.Sleep(ctx, time.Second*5)
			}
			s.Fatal("Routine verification failed: ", err)
		}
		return
	}

	result, err := croshealthd.RunDiagRoutine(ctx,
		croshealthd.NewRoutineParams(param.BluetoothRoutineType))
	if err != nil {
		s.Fatal("Unable to run routine: ", err)
	}

	if err := result.VerifyPassed(); err != nil {
		s.Fatal("Routine is not passed: ", err)
	}
}
