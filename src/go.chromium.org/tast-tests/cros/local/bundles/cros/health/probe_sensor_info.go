// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package health

import (
	"context"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-cmp/cmp"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/health/iioservice"
	"go.chromium.org/tast-tests/cros/local/croshealthd"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ProbeSensorInfo,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that cros_healthd can fetch sensor info",
		Contacts:     []string{"cros-tdm-tpe-eng@google.com"},
		BugComponent: "b:982097",
		// TODO(b/251306646): Promote to critical.
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome", "diagnostics"},
		Fixture:      "crosHealthdRunning",
	})
}

type sensorInfo struct {
	LidAngle *uint16            `json:"lid_angle"`
	Sensors  []sensorAttributes `json:"sensors"`
}

type sensorAttributes struct {
	Name     *string `json:"name"`
	DeviceID int32   `json:"device_id"`
	Type     string  `json:"type"`
	Location string  `json:"location"`
}

var errUnreliableLidAngle = errors.New("unreliable lid angle")

// getLidAngleFromEctool parses the output of ectool and gets the value of lid angle.
func getLidAngleFromEctool(ctx context.Context) (uint16, error) {
	bStdout, bStderr, err := testexec.CommandContext(ctx, "ectool", "motionsense", "lid_angle").SeparatedOutput(testexec.DumpLogOnError)
	if err != nil {
		stderr := string(bStderr)
		if strings.Contains(stderr, "INVALID_COMMAND") || strings.Contains(stderr, "INVALID_PARAM") {
			// Some devices do not support lid_angle and return |INVALID_COMMAND| or
			// |INVALID_PARAM|. Check stderr and return "" in these cases.
			return 0, errUnreliableLidAngle
		}
		return 0, errors.Wrap(err, "failed to run ectool command")
	}

	raw := strings.ReplaceAll(strings.TrimSpace(string(bStdout)), "Lid angle: ", "")
	if raw == "unreliable" {
		return 0, errUnreliableLidAngle
	}

	lidAngle, err := strconv.ParseUint(raw, 10, 16)
	if err != nil {
		return 0, errors.Wrapf(err, "failed to convert lid angle to uint, raw value: %s", raw)
	}
	return uint16(lidAngle), nil
}

func pollLidAngleFromEctool(ctx context.Context) (uint16, error) {
	// Since lid angle could be nil when the device is in motion, we poll lid
	// angle from ectool to make sure the it is not temporarily unreliable.
	var lidAngle uint16
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		value, err := getLidAngleFromEctool(ctx)
		if err != nil {
			if !errors.Is(err, errUnreliableLidAngle) {
				return testing.PollBreak(err)
			}
			return err
		}
		lidAngle = value
		return nil
	}, &testing.PollOptions{Interval: time.Second, Timeout: 5 * time.Second}); err != nil {
		return 0, err
	}
	return lidAngle, nil
}

// validateLidAngle checks if the lid angle value obtained from cros_healthd is
// nearly the same as the value obtained from EC.
func validateLidAngle(ctx context.Context, got uint16, reliable bool) error {
	// Check to see if a Google EC exists. If it does, use ectool to get the lid
	// angle that should be reported.
	if _, err := os.Stat("/sys/class/chromeos/cros_ec"); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		if reliable {
			return errors.Errorf("failed to find EC but cros_healthd report reliable lid angle: %v", got)
		}
		return nil
	}

	want, err := pollLidAngleFromEctool(ctx)
	if err != nil {
		if !errors.Is(err, errUnreliableLidAngle) {
			return err
		}
		if reliable {
			return errors.Errorf("failed to get reliable lid angle measurement but cros_healthd report one: %v", got)
		}
		return nil
	}

	if !reliable {
		return errors.Errorf("got expected lid angle measurement: %v but cros_healthd did not report one", want)
	}

	// Given we read the lid angle with ectool directly or indirectly through
	// cros_healthd, we should get the same value. Add a tolerance in case there
	// is a slight variation in measurements.
	const lidAngleTolerance = 1
	if math.Abs(float64(got)-float64(want)) > lidAngleTolerance {
		return errors.Errorf("failed. LidAngle doesn't match and the difference is out of tolerance: got %v; want %v", got, want)
	}

	return nil
}

func pollLidAngleFromCrosHealthTool(ctx context.Context, params croshealthd.TelemParams, outDir string, info *sensorInfo) error {
	// Early return if we already collected reliable lid angle from cros_healthd.
	if info.LidAngle != nil {
		return nil
	}

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if err := croshealthd.RunAndParseJSONTelem(ctx, params, outDir, info); err != nil {
			return testing.PollBreak(err)
		}
		if info.LidAngle == nil {
			return errUnreliableLidAngle
		}
		return nil
	}, &testing.PollOptions{Interval: time.Second, Timeout: 5 * time.Second}); err != nil {
		return err
	}
	return nil
}

func validateSensorAttributes(ctx context.Context, info *sensorInfo) error {
	var got []iioservice.SensorAttributes
	for _, sensor := range info.Sensors {
		got = append(got, iioservice.SensorAttributes{
			Name:     sensor.Name,
			DeviceID: sensor.DeviceID,
			Type:     sensor.Type,
			Location: sensor.Location,
		})
	}
	expected, err := iioservice.ExpectedSensorAttributes(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get expected sensor attributes")
	}
	if diff := cmp.Diff(expected, got); diff != "" {
		return errors.Wrapf(err, "sensor attributes mismatch (-expected + got): %s", diff)
	}
	return nil
}

// ProbeSensorInfo is the main function of this tast test.
func ProbeSensorInfo(ctx context.Context, s *testing.State) {
	params := croshealthd.TelemParams{Category: croshealthd.TelemCategorySensor}
	var info sensorInfo
	if err := croshealthd.RunAndParseJSONTelem(ctx, params, s.OutDir(), &info); err != nil {
		s.Fatal("Failed to get sensor telemetry info: ", err)
	}

	sort.Slice(info.Sensors, func(i, j int) bool { return info.Sensors[i].DeviceID < info.Sensors[j].DeviceID })
	if err := validateSensorAttributes(ctx, &info); err != nil {
		s.Fatal("Failed to validate sensor attributes: ", err)
	}

	// Since lid angle could be nil when the device is in motion, we poll lid
	// angle from Healthd to make sure the it is not temporarily unreliable.
	err := pollLidAngleFromCrosHealthTool(ctx, params, s.OutDir(), &info)
	if err != nil && !errors.Is(err, errUnreliableLidAngle) {
		s.Fatal("Failed to get lid angle info: ", err)
	}

	var lidAngle uint16 = 0
	reliable := (info.LidAngle != nil)
	if reliable {
		lidAngle = *info.LidAngle
	}
	// Validate lid angle from cros_healthd even if it is unreliable.
	if err := validateLidAngle(ctx, lidAngle, reliable); err != nil {
		s.Fatal("Failed to validate lid angle: ", err)
	}
}
