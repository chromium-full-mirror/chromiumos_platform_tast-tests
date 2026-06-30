// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"math"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast-tests/cros/services/cros/power"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: PDVbusRequest,
		Desc: "Tests if the DUT can properly request VBUS voltages from a Source, and supply VBUS voltages to a Sink",
		Contacts: []string{
			"chromeos-faft@google.com", // Owning team list
			"shurst@google.com",        // Test author
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Data:         []string{firmware.ConfigFile},
		Vars:         []string{"servo"},
		TestBedDeps:  tbdep.ServoPresentAndWorking,
		SoftwareDeps: []string{"chrome"},
		ServiceDeps: []string{
			"tast.cros.power.BatteryService",
		},
		Fixture:      fixture.NormalMode,
		HardwareDeps: hwdep.D(hwdep.ChromeEC(), hwdep.Battery()),
		Timeout:      120 * time.Minute,
		Attr:         []string{"group:firmware", "firmware_pd", "firmware_meets_kpi", "firmware_stressed", "firmware_ec_ro", "firmware_ec_rw", "firmware_bios_pdc"},
		Params: []testing.Param{{
			Name:      "normal",
			ExtraAttr: []string{"firmware_enabled"},
			Val: firmware.PDTestParams{
				DTS: firmware.DTSModeOff,
			},
		}, {
			Name: "flipcc",
			Val: firmware.PDTestParams{
				CC:  firmware.CCPolarityFlipped,
				DTS: firmware.DTSModeOff,
			},
		}, {
			Name: "dts",
			Val: firmware.PDTestParams{
				DTS: firmware.DTSModeOn,
			},
		}, {
			Name: "flipcc_dts",
			Val: firmware.PDTestParams{
				CC:  firmware.CCPolarityFlipped,
				DTS: firmware.DTSModeOn,
			},
		}, {
			Name: "shutdown",
			Val: firmware.PDTestParams{
				Shutdown: true,
				DTS:      firmware.DTSModeOff,
			},
		}},
	})
}

const (
	// pdPowerRolePollTimeout is the timeout for a power role swap
	pdPowerRolePollTimeout time.Duration = 500 * time.Millisecond
	// pdPowerRolePollInterval is the time before testing for a power role swap
	pdPowerRolePollInterval time.Duration = 100 * time.Millisecond
	// pdPowerVBusPollTimeout
	pdVBusPollTimeout time.Duration = 10 * time.Second
	// pdPowerVBusPollInterval
	pdVBusPollInterval time.Duration = 1 * time.Second
)

const (
	// maxPollFailCount is the number of times test can fail in a Poll loop
	maxPollFailCount = 10
	// usbCMaxVoltage is the maximum voltage
	usbCMaxVoltage = 20
	// usbCSinkVoltage is the Sink Voltage
	usbCSinkVoltage = 5
)

const (
	// vbusTolerance is the VBUS measurement tolerance
	vbusTolerance = 0.12
)

// charge starts charging a the given voltage
func charge(ctx context.Context, h *firmware.Helper, voltage int) error {
	err := h.Servo.SetInt(ctx, servo.UsbcPr, voltage)
	return err
}

// PDVbusRequest test is written to use both the DUT and PDTester
// test board. It requires that the DUT support dualrole (SRC or SNK)
// operation. VBUS change requests occur in two methods.
//
// The 1st test initiates the VBUS change by using special PDTester
// feature to send new SRC CAP message. This causes the DUT to request
// a new VBUS voltage matching what's in the SRC CAP message.
//
// The 2nd test configures the DUT in SNK mode and uses the pd console
// command 'pd 0/1 dev V' command where V is the desired voltage
// 5/12/20. This test is more risky and won't be executed if the 1st
// test is failed. If the DUT max input voltage is not 20V, like 12V,
// and the FAFT config is set wrong, it may negotiate to a voltage
// higher than it can support, that may damage the DUT.
//
// FAFT configs:
//
//	UsbcInputVoltageLimit
//	ChargerProfileOverride
//
// Pass criteria is all voltage transitions are successful.
func PDVbusRequest(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to create config: ", err)
	}

	if err := h.RequireRPCClient(ctx); err != nil {
		s.Fatal("Failed to connect to RPC: ", err)
	}
	client := power.NewBatteryServiceClient(h.RPCClient.Conn)
	if _, err := client.New(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to start BatteryServiceClient: ", err)
	}
	defer client.Close(ctx, &empty.Empty{})
	if _, err := client.StopChargeLimit(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to stop charge limit: ", err)
	}

	// If battery is full, discharge it some before starting the test
	if err := firmware.DischargeBattery(ctx, h, 93.0); err != nil {
		s.Fatal("Failed discharge battery: ", err)
	}

	testParams := s.Param().(firmware.PDTestParams)

	if err := firmware.SetupPDTester(ctx, h, testParams, s.OutDir()); err != nil {
		s.Fatal("Failed to configure Servo for PD testing: ", err)
	}

	//
	// Move on to the actual VBUS Request test
	//

	dutVoltageLimit := h.Config.UsbcInputVoltageLimit
	dutPowerLimit := h.Config.MaxChargingPower

	// Read Servo SRCCAPS
	srccaps, err := h.Servo.GetPDAdapterSrcCaps(ctx)
	if err != nil {
		s.Fatal("Failed to get Source Caps: ", err)
	}

	chargingVoltages := make(map[int]bool)
	for _, sc := range srccaps {
		voltage := sc.Voltage / 1000
		// Servo always returns integer voltages, even though they could theoretically be fractional.
		chargingVoltages[voltage] = true
	}
	s.Logf("srcCaps = %+v allVoltages = %v", srccaps, chargingVoltages)

	// Check that servo v4 is providing:
	// 5v - All chargers should have this
	if !chargingVoltages[5] {
		s.Error("Charger doesn't support 5v, which should be impossible. Please try a different (i.e. 65w or greater) charger")
	}
	// The max voltage of the DUT
	if !chargingVoltages[dutVoltageLimit] {
		s.Errorf("Charger doesn't support %vv. Please try a different (i.e. 65w or greater) charger", dutVoltageLimit)
	}
	// At least 3 voltages
	if len(chargingVoltages) < 3 {
		s.Error("Charger doesn't support 3 different voltages. Please try a different (i.e. 65w or greater) charger")
	}

	// If a lower voltage can supply the max power the dut can handle (sMaxPowerMw), then
	// the DUT will probably pick that lower voltage and not the highest available voltage.
	lowestVoltageForMaxPower := dutVoltageLimit
	foundMaxPower := false
	for _, pdo := range srccaps {
		if (float64(pdo.Current) * float64(pdo.Voltage) / 1000000.0) >= dutPowerLimit {
			foundMaxPower = true
			if pdo.Voltage/1000.0 < lowestVoltageForMaxPower {
				s.Logf("PDO: %+v (%d mW)", pdo, pdo.Current*pdo.Voltage/1000)
				lowestVoltageForMaxPower = pdo.Voltage / 1000.0
			} else {
				s.Logf("DUT might not use this PDO: %+v (%d mW)", pdo, pdo.Current*pdo.Voltage/1000)
			}
		} else {
			s.Logf("PDO: %+v (%d mW)", pdo, pdo.Current*pdo.Voltage/1000)
		}
	}
	if !foundMaxPower && chargingVoltages[dutVoltageLimit] {
		s.Logf("Charger does not support %f W, but this is fine", dutPowerLimit)
		// If !chargingVoltages[dutVoltageLimit], we already reported an error earlier.
	}

	// Set dps disable
	if err := h.Servo.RunECCommand(ctx, "dps dis"); err != nil {
		s.Log("Servo console command dps disable failed: ", err)
	}

	defer func() {
		//Set dps enable
		err := h.Servo.RunECCommand(ctx, "dps en")
		if err != nil {
			s.Log("Servo console command dps enable failed: ", err)
		}
		// PDTester is set back to 20V SRC mode.
		err = charge(ctx, h, usbCMaxVoltage)
		if err != nil {
			s.Fatal("Failed to set charging voltage: ", usbCMaxVoltage)
		}
	}()

	s.Log("Start of PDTester initiated tests")

	// Loop over many voltages, and tell the servo not to advertise any SRC CAP over that
	// voltage.
	for voltage := range chargingVoltages {
		s.Logf("********* %v *********", voltage)
		// Set charging voltage
		err := charge(ctx, h, voltage)
		if err != nil {
			s.Fatal("Failed to set charging voltage: ", voltage)
		}
		// Wait for new PD contract to be established and voltage to settle
		s.Log("Sleeping for 10 seconds")
		// GoBigSleepLint: Wait for new PD contract to be established
		if err := testing.Sleep(ctx, 10*time.Second); err != nil {
			s.Fatal("Failed to sleep for 10 seconds: ", err)
		}

		pdState, err := h.Servo.GetServoPDState(ctx)
		if err != nil {
			s.Fatal("Failed to get PD State")
		}
		s.Logf("Servo PD state: %+v", pdState)
		if !pdState.IsSourceReady() {
			s.Fatal("PD state is not SRC ready")
		}
		vbus, err := h.Servo.GetFloat(ctx, servo.VBusVoltage)
		if err != nil {
			s.Fatal("Failed to get vbus: ", err)
		}
		vbus = vbus / 1000.0

		if lowestVoltageForMaxPower < voltage {
			s.Logf("VBus: %f V (expect %d V or %d V)",
				vbus, voltage, lowestVoltageForMaxPower)
			if math.Abs(vbus-float64(voltage)) < vbusTolerance*float64(voltage) {
				lowestVoltageForMaxPower = voltage
				s.Logf("%d V is now expected", lowestVoltageForMaxPower)
			} else {
				if math.Abs(vbus-float64(lowestVoltageForMaxPower)) > vbusTolerance*float64(lowestVoltageForMaxPower) {
					s.Errorf("Vbus should be within 12%% of %d, got %f V", lowestVoltageForMaxPower, vbus)
				}
			}
		} else {
			s.Logf("VBus: %f V (expect %d V)", vbus, voltage)
			if math.Abs(vbus-float64(voltage)) > vbusTolerance*float64(voltage) {
				s.Errorf("Vbus should be within 12%% of %d, got %f V", voltage, vbus)
			}
		}
	}

	// PDTester is set back to 20V SRC mode.
	err = charge(ctx, h, usbCMaxVoltage)
	if err != nil {
		s.Fatal("Failed to set charging voltage: ", usbCMaxVoltage)
	}

	// The next group of tests need DUT to connect in SNK and SRC modes
	err = h.Servo.SetDualroleState(ctx, servo.DROn)
	if err != nil {
		s.Fatal("Failed to enable DualRole")
	}

	if testParams.Shutdown {
		if err := h.Servo.SetPowerState(ctx, servo.PowerStateOn); err != nil {
			testing.ContextLog(ctx, "Failed to power on DUT: ", err)
		}
		if err := h.WaitConnect(ctx); err != nil {
			s.Fatal("Failed to boot after test: ", err)
		}
	}
}
