// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package health tests the system daemon cros_healthd to ensure that telemetry
// and diagnostics calls can be completed successfully.
package health

import (
	"context"
	"encoding/json"

	"go.chromium.org/tast-tests/cros/local/croshealthd"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type sensitiveSensorRoutineTestParams struct {
	// If true, check the v2 routine. Otherwise, check the v1 routine.
	CheckRoutineV2 bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func: RunSensitiveSensorRoutine,
		Desc: "Checks that cros_healthd can run sensitive sensor routine",
		Contacts: []string{
			"cros-tdm-tpe-eng@google.com",
			"byronlee@chromium.org",
		},
		// ChromeOS > Platform > Enablement > Serviceability > Diagnostic & Health
		BugComponent: "b:982097",
		SoftwareDeps: []string{"diagnostics"},
		Attr:         []string{"group:mainline"},
		Fixture:      "crosHealthdRunning",
		Params: []testing.Param{{
			Name: "v1",
			Val: sensitiveSensorRoutineTestParams{
				CheckRoutineV2: false,
			},
		}, {
			Name: "v2",
			Val: sensitiveSensorRoutineTestParams{
				CheckRoutineV2: true,
			},
		}},
	})
}

type sensitiveSensorInfo struct {
	ID       int32    `json:"id"`
	Types    []string `json:"types"`
	Channels []string `json:"channels"`
}

type sensitiveSensorReport struct {
	PassedSensors        []sensitiveSensorInfo `json:"passed_sensors"`
	FailedSensors        []sensitiveSensorInfo `json:"failed_sensors"`
	SensorPresenceStatus string                `json:"sensor_presence_status"`
}

type sensitiveSensorRoutineOutput struct {
	BaseAccelerometer sensitiveSensorReport `json:"base_accelerometer"`
	LidAccelerometer  sensitiveSensorReport `json:"lid_accelerometer"`
	BaseGyroscope     sensitiveSensorReport `json:"base_gyroscope"`
	LidGyroscope      sensitiveSensorReport `json:"lid_gyroscope"`
	BaseMagnetometer  sensitiveSensorReport `json:"base_magnetometer"`
	LidMagnetometer   sensitiveSensorReport `json:"lid_magnetometer"`
	BaseGravitySensor sensitiveSensorReport `json:"base_gravity_sensor"`
	LidGravitySensor  sensitiveSensorReport `json:"lid_gravity_sensor"`
}

func buildSensitiveSensorRoutineArgs(ctx context.Context) ([]string, error) {
	return []string{"sensitive_sensor_v2"}, nil
}

func isExpectedSensorStatus(report sensitiveSensorReport) bool {
	// TODO(b/361720963): Check `report.FailedSensors` if we can stability monitor
	// changes of sensor value in the lab.
	// We don't check `report.FailedSensors` since some sensros will always report
	// the same value without user actions. That will make the routine failed with
	// non-empty `report.FailedSensors` on some devices in lab.
	return report.SensorPresenceStatus == "Matched" || report.SensorPresenceStatus == "Not Configured"
}

func verifySensitiveSensorRoutineResult(result croshealthd.RoutineResultV2) error {
	if result.Progress != 100 {
		return errors.Errorf("unexpected progress: got %d, want 100; output = %q", result.Progress, result.Output)
	}

	if result.Status == croshealthd.StatusFailed {
		var routineOutput sensitiveSensorRoutineOutput
		if err := json.Unmarshal([]byte(result.Output), &routineOutput); err != nil {
			return errors.Errorf("failed to unmarshal the routine output: %q", result.Output)
		}

		for _, report := range []sensitiveSensorReport{
			routineOutput.BaseAccelerometer, routineOutput.LidAccelerometer,
			routineOutput.BaseGyroscope, routineOutput.LidGyroscope,
			routineOutput.BaseMagnetometer, routineOutput.LidMagnetometer,
			routineOutput.BaseGravitySensor, routineOutput.LidGravitySensor} {
			if !isExpectedSensorStatus(report) {
				return errors.Errorf("unexpected routine output: %q", result.Output)
			}
		}
		return nil
	}

	if result.Status != croshealthd.StatusPassed {
		return errors.Errorf("unexpected status: got %q, want %q; output = %q", result.Status, croshealthd.StatusPassed, result.Output)
	}
	return nil
}

// RunSensitiveSensorRoutine runs the sensitive sensor routine.
func RunSensitiveSensorRoutine(ctx context.Context, s *testing.State) {
	param := s.Param().(sensitiveSensorRoutineTestParams)

	if param.CheckRoutineV2 {
		config := croshealthd.RoutineTestingConfigV2{
			ArgsBuilder:    buildSensitiveSensorRoutineArgs,
			RoutineRunner:  croshealthd.RunDiagV2,
			ResultVerifier: verifySensitiveSensorRoutineResult,
		}
		if err := croshealthd.TestDiagRoutineV2(ctx, config); err != nil {
			s.Fatal("Routine verification failed: ", err)
		}
	} else {
		result, err := croshealthd.RunDiagRoutine(ctx,
			croshealthd.NewRoutineParams(croshealthd.RoutineSensitiveSensor))
		if err != nil {
			s.Fatal("Unable to run routine: ", err)
		}

		// Only check if the v1 routine can be finished since we just run v2 routine
		// via v1 interface.
		if err := result.VerifyFinished(); err != nil {
			s.Fatal("Routine is not finished: ", err)
		}
	}
}
