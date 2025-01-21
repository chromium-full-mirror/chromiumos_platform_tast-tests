// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package typec

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/common/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast-tests/cros/remote/typec/battery"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: DischargeChargeBattery,
		Desc: "Discharge and charge battery test, different voltage",
		Contacts: []string{
			"chromeos-usb-champs@google.com",
			"kamilplucinski@google.com", // Test author
		},
		BugComponent: "b:958036", // ChromeOS > Platform > Technologies > USB
		Vars:         []string{"servo"},
		Timeout:      48 * time.Hour,
		Fixture:      fixture.NormalMode,
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		Attr:         []string{"group:typec", "typec_manual"},
		Params: []testing.Param{{
			Name: "normal",
			Val: firmware.PDTestParams{
				DTS: firmware.DTSModeOff,
			},
		}, {
			Name: "flipcc",
			Val: firmware.PDTestParams{
				CC:  firmware.CCPolarityFlipped,
				DTS: firmware.DTSModeOff,
			},
		}},
	})
}

const (
	desiredChargedLevel    = 20
	desiredDischargedLevel = 10
	deviationPercent       = 5
	dischargingTimeout     = 600 * time.Minute
)

func DischargeChargeBattery(ctx context.Context, s *testing.State) {

	h := s.FixtValue().(*fixture.Value).Helper
	batteryProxy := battery.GetBatteryProxy(ctx, h)
	batteryProxy.SetOutdir(s.OutDir())

	// set up PD testing, here we start with at least 10% battery level
	if err := configurePDTesting(ctx, s.Param().(firmware.PDTestParams), h); err != nil {
		s.Fatalf("%s: Failed to configure servod PD testing", err)
	}

	voltages := []int{5, 9, 15, 20}

	for _, voltage := range voltages {
		testName := fmt.Sprintf("using %dV up to %d%%", voltage, desiredChargedLevel)
		s.Run(ctx, testName, func(ctx context.Context, s *testing.State) {
			if err := batteryProxy.DischargeBatteryWithTimeout(ctx, desiredDischargedLevel, dischargingTimeout); err != nil {
				s.Fatalf("%s: Failed to discharge battery: %v", testName, err)
			}
			if err := chargeBattery(ctx, s, batteryProxy, voltage); err != nil {
				s.Fatalf("%s: Failed charge battery: %v", testName, err)
			}
			if err := utils.SaveDmesgToFile(ctx, s.OutDir(), testName); err != nil {
				s.Fatalf("%s: Failed to save dmesg output to file: %v", testName, err)
			}
		})
	}
}

func configurePDTesting(ctx context.Context, testParams firmware.PDTestParams, h *firmware.Helper) error {
	if err := h.RequireConfig(ctx); err != nil {
		return errors.Wrap(err, "failed to create config")
	}

	if err := firmware.SetupPDTester(ctx, h, testParams); err != nil {
		return errors.Wrap(err, "failed to configure servo for PD testing")
	}
	return nil
}

func chargeBattery(ctx context.Context, s *testing.State, batteryProxy *battery.Battery, voltage int) error {
	if err := batteryProxy.SetChargingVoltageAndCharge(ctx, voltage); err != nil {
		return errors.Wrap(err, "failed to set charging voltage")
	}
	return batteryProxy.ChargeAndMonitorUpTo(ctx, desiredChargedLevel, func(data battery.DataPoint) {
		battery.ValidateCharging(ctx, data, voltage, deviationPercent)
	})
}
