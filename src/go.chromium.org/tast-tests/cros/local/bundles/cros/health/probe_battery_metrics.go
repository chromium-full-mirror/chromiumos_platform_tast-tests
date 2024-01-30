// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package health

import (
	"context"
	"strconv"

	"go.chromium.org/tast-tests/cros/local/crosconfig"
	"go.chromium.org/tast-tests/cros/local/croshealthd"
	"go.chromium.org/tast-tests/cros/local/jsontypes"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type batteryInfo struct {
	CycleCount       string            `json:"cycle_count"`
	ModelName        string            `json:"model_name"`
	SerialNumber     string            `json:"serial_number"`
	Status           string            `json:"status"`
	Technology       string            `json:"technology"`
	Vendor           string            `json:"vendor"`
	ManufactureDate  *string           `json:"manufacture_date"`
	ChargeFull       float64           `json:"charge_full"`
	ChargeFullDesign float64           `json:"charge_full_design"`
	ChargeNow        float64           `json:"charge_now"`
	CurrentNow       float64           `json:"current_now"`
	VoltageMinDesign float64           `json:"voltage_min_design"`
	VoltageNow       float64           `json:"voltage_now"`
	Temperature      *jsontypes.Uint64 `json:"temperature"`
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         ProbeBatteryMetrics,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Check that we can probe cros_healthd for battery metrics",
		Contacts: []string{
			"cros-tdm-tpe-eng@google.com",
			"byronlee@google.com",
		},
		BugComponent: "b:982097", // ChromeOS > Platform > Enablement > Health
		Attr:         []string{"group:mainline"},
		SoftwareDeps: []string{"diagnostics"},
		HardwareDeps: hwdep.D(hwdep.Battery()),
		Fixture:      "crosHealthdRunning",
	})
}

func compareInt64Property(want *int64, got string) error {
	if want == nil {
		return errors.New("unexpected want: nil")
	}
	gotValue, err := strconv.ParseInt(got, 10, 64)
	if err != nil {
		return errors.Errorf("failed to parse got: %v", got)
	}
	if gotValue != *want {
		return errors.Errorf("got %v, want %v", gotValue, *want)
	}
	return nil
}

func compareStringProperty(want *string, got string) error {
	if want == nil {
		return errors.New("unexpected want: nil")
	}
	if got != *want {
		return errors.Errorf("got %v, want %v", got, *want)
	}
	return nil
}

func compareFloatProperty(want *float64, got float64) error {
	if want == nil {
		return errors.New("unexpected want: nil")
	}
	// Because the value of battery varies continuously, so we only check if it's roughly the same.
	// Checked with hardware team, they recommended that we can check if it's within 5%.
	maxWant := *want * 1.05
	minWant := *want * 0.95
	if got > maxWant || got < minWant {
		return errors.Errorf("got %v, want [%v, %v]", got, minWant, maxWant)
	}
	return nil
}

func validateBatteryData(ctx context.Context, battery *batteryInfo) error {
	pm, err := power.NewPowerManager(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get new power manager")
	}
	powerSupply, err := pm.GetPowerSupplyProperties(ctx)
	if err != nil {
		return err
	}
	if err := compareInt64Property(powerSupply.BatteryCycleCount, battery.CycleCount); err != nil {
		return errors.Wrap(err, "failed to verify CycleCount field")
	}
	if err := compareStringProperty(powerSupply.BatteryModelName, battery.ModelName); err != nil {
		return errors.Wrap(err, "failed to verify ModelName field")
	}
	if err := compareStringProperty(powerSupply.BatterySerialNumber, battery.SerialNumber); err != nil {
		return errors.Wrap(err, "failed to verify SerialNumber field")
	}
	if err := compareStringProperty(powerSupply.BatteryTechnology, battery.Technology); err != nil {
		return errors.Wrap(err, "failed to verify Technology field")
	}
	if err := compareStringProperty(powerSupply.BatteryVendor, battery.Vendor); err != nil {
		return errors.Wrap(err, "failed to verify Vendor field")
	}
	if err := compareFloatProperty(powerSupply.BatteryChargeFull, battery.ChargeFull); err != nil {
		return errors.Wrap(err, "failed to verify ChargeFull field")
	}
	if err := compareFloatProperty(powerSupply.BatteryChargeFullDesign, battery.ChargeFullDesign); err != nil {
		return errors.Wrap(err, "failed to verify ChargeFullDesign field")
	}
	if err := compareFloatProperty(powerSupply.BatteryCharge, battery.ChargeNow); err != nil {
		return errors.Wrap(err, "failed to verify ChargeNow field")
	}
	if err := compareFloatProperty(powerSupply.BatteryVoltageMinDesign, battery.VoltageMinDesign); err != nil {
		return errors.Wrap(err, "failed to verify VoltageMinDesign field")
	}
	if err := compareFloatProperty(powerSupply.BatteryVoltage, battery.VoltageNow); err != nil {
		return errors.Wrap(err, "failed to verify VoltageNow field")
	}

	// Battery status changes from time to time, so we only check if the status string is expected or not.
	_, ok := power.MapStringToBatteryStatus(battery.Status)
	if !ok {
		return errors.Errorf("status %v is not expected", battery.Status)
	}

	// We can't test battery.CurrentNow, because the value varies quickly.
	// For example, cros_healthd get 0.639 but when we fetch the value from power manager, it becomes 0.961.

	// Validate Smart Battery metrics.
	val, err := crosconfig.Get(ctx, "/cros-healthd/battery", "has-smart-battery-info")
	if err != nil && !crosconfig.IsNotFound(err) {
		return errors.Wrap(err, "failed to get has-smart-battery-info property")
	}

	hasSmartInfo := err == nil && val == "true"
	if hasSmartInfo {
		if battery.ManufactureDate == nil {
			return errors.New("Missing manufacture_date for smart battery")
		}
		if battery.Temperature == nil {
			return errors.New("Missing temperature for smart battery")
		}
	}

	return nil
}

func ProbeBatteryMetrics(ctx context.Context, s *testing.State) {
	params := croshealthd.TelemParams{Category: croshealthd.TelemCategoryBattery}
	var battery batteryInfo
	if err := croshealthd.RunAndParseJSONTelem(ctx, params, s.OutDir(), &battery); err != nil {
		s.Fatal("Failed to get battery telemetry info: ", err)
	}

	if err := validateBatteryData(ctx, &battery); err != nil {
		s.Fatal("Failed to validate battery data: ", err)
	}
}
