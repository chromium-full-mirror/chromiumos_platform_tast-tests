// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast-tests/cros/services/cros/power"
	"go.chromium.org/tast/core/errors"
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
		BugComponent: "b:194910335", // ChromeOS > Platform > Enablement > Firmware > FAFT
		// TODO: When stable, move to firmware_pd.
		Data:         []string{firmware.ConfigFile},
		Attr:         []string{"group:firmware", "firmware_pd_unstable"},
		Vars:         []string{"servo"},
		SoftwareDeps: []string{"chrome"},
		ServiceDeps: []string{
			"tast.cros.ui.PowerMenuService",
			"tast.cros.browser.ChromeService",
			"tast.cros.power.BatteryService",
		},
		Fixture:      fixture.NormalMode,
		HardwareDeps: hwdep.D(hwdep.ChromeEC(), hwdep.Battery()),
		Timeout:      60 * time.Minute,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Params: []testing.Param{{
			Name: "normal",
			Val: firmware.PDTestParams{
				CC:       firmware.CCPolarityStandard,
				DTS:      firmware.DTSModeOn,
				Shutdown: false,
			},
		}, {
			Name: "flipcc",
			Val: firmware.PDTestParams{
				CC:       firmware.CCPolarityFlipped,
				DTS:      firmware.DTSModeOn,
				Shutdown: false,
			},
		}, {
			Name: "dtsoff",
			Val: firmware.PDTestParams{
				CC:       firmware.CCPolarityStandard,
				DTS:      firmware.DTSModeOff,
				Shutdown: false,
			},
		}, {
			Name: "flipcc_dtsoff",
			Val: firmware.PDTestParams{
				CC:       firmware.CCPolarityFlipped,
				DTS:      firmware.DTSModeOff,
				Shutdown: false,
			},
		}, {
			Name: "shutdown",
			Val: firmware.PDTestParams{
				CC:       firmware.CCPolarityStandard,
				DTS:      firmware.DTSModeOn,
				Shutdown: true,
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

// voltageSequence is the VBUS Voltage test sequence
var voltageSequence = [22]int{5, 9, 10, 12, 15, 20, 15, 12, 9, 5, 20, 5, 5, 9, 9, 10, 10, 12, 12, 15, 15, 20}

const (
	// pass
	pass int = 0
	// allowedFail
	allowedFail = 1
	// fail
	fail = 2
)

// compareVbus is a helper function for testing if VBUS falls within a expected range.
func compareVbus(ctx context.Context, h *firmware.Helper, s *testing.State, expectedVbusVoltage float64, okToFail bool) (int, string) {
	var vbusVoltage float64
	var tolerance float64
	var voltageDifference float64
	var resultStr string

	// Read Vbus voltage
	vbusVoltage, err := h.Servo.GetFloat(ctx, servo.VBusVoltage)
	if err != nil {
		return fail, "Failed to read VBus Voltage"
	}
	vbusVoltage = vbusVoltage / 1000

	// Compute voltage tolerance range. To handle the case where VBUS is
	// off, set the minimal tolerance to usbCSinkVoltage * vbusTolerance.
	tolerance = vbusTolerance * math.Max(expectedVbusVoltage, usbCSinkVoltage)
	// Verify that measured Vbus voltage is within expected range
	voltageDifference = math.Abs(expectedVbusVoltage - vbusVoltage)

	resultStr = fmt.Sprintf("Target = %FV:\tAct = %F\tDelta = %F", expectedVbusVoltage, vbusVoltage, voltageDifference)

	if voltageDifference > tolerance {
		if okToFail {
			return allowedFail, resultStr
		}
		return fail, resultStr
	}

	return pass, resultStr
}

// getVoltageAndCurrent converts a string in the following format: 0: 5000mV/3000mA
// into its voltage and current floats
func getVoltageAndCurrent(vc string) (float64, float64, error) {
	// Split string into index and voltage/current
	tmp0 := strings.Split(vc, " ")

	// Split string into voltage and current
	tmp1 := strings.Split(tmp0[1], "/")

	// Strip off mV
	vstr := tmp1[0][:len(tmp1[0])-2]

	// Strip off mA
	cstr := tmp1[1][:len(tmp1[1])-2]

	// convert to float
	v, err := strconv.ParseFloat(vstr, 64)
	if err != nil {
		return 0, 0, err
	}

	// convert to float
	c, err := strconv.ParseFloat(cstr, 64)
	if err != nil {
		return 0, 0, err
	}

	return v, c, nil
}

// charge starts charging a the given voltage
func charge(ctx context.Context, h *firmware.Helper, voltage int) error {
	err := h.Servo.RunServoCommand(ctx, fmt.Sprintf("usbc_action chg %d", voltage))
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
	var expectedVbusVoltage float64
	var okToFail bool
	var isOverride bool
	var pdTesterFailures []string
	var dutFailures []string

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
	if err := firmware.TestChargingVoltagesAfterDischarge(ctx, h, 93.0); err != nil {
		s.Fatal("Failed checking voltages after discharge test: ", err)
	}

	testParams := s.Param().(firmware.PDTestParams)

	if err := firmware.SetupPDTester(ctx, h, testParams.CC, testParams.DTS, testParams.RequiredPort); err != nil {
		s.Fatal("Failed to configure Servo for PD testing: ", err)
	}

	//
	// Move on to the actual VBUS Request test
	//

	dutVoltageLimit := h.Config.UsbcInputVoltageLimit

	isOverride = h.Config.ChargerProfileOverride
	if isOverride == true {
		s.Log("*** Custom charger profile takes over, which may cause voltage-not-matched. It is OK to fail. *** ")
	}

	// Obtain voltage limit due to maximum charging power. Note that this
	// voltage limit applies only when EC follows the default policy. There
	// are other policies like PREFER_LOW_VOLTAGE or PREFER_HIGH_VOLTAGE but
	// they are not implemented in this test.
	srccaps, err := h.Servo.GetPDAdapterSrcCaps(ctx)
	if err != nil {
		s.Fatal("Failed to get Source Caps: ", err)
	}

	chargingVoltages := make(map[int]bool)
	for _, sc := range srccaps {
		mv, _, err := getVoltageAndCurrent(sc)
		if err != nil {
			s.Fatal("error")
		}

		voltage := mv / 1000.0
		// Servo always returns integer voltages, even though they could theoretically be fractional.
		chargingVoltages[int(voltage)] = true
	}
	if !chargingVoltages[5] {
		s.Error("Charger doesn't support 5v, which should be impossible. Please try a different (i.e. 65w or greater) charger")
	}
	if !chargingVoltages[dutVoltageLimit] {
		s.Errorf("Charger doesn't support %vv. Please try a different (i.e. 65w or greater) charger", dutVoltageLimit)
	}
	if len(chargingVoltages) < 3 {
		s.Error("Charger doesn't support 3 different voltages. Please try a different (i.e. 65w or greater) charger")
	}

	s.Log("Start of PDTester initiated tests")

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
		// If PDTester is in SNK mode and the DUT is in S0, the DUT should
		// source VBUS = USBC_SINK_VOLTAGE. If PDTester is in SNK mode, and
		// the DUT is not in S0, the DUT shouldn't source VBUS, which means
		// VBUS = 0.
		if pdState.PowerRole == servo.PowerRoleSNK {
			expectedVbusVoltage = usbCSinkVoltage
			ps, err := h.Servo.GetECSystemPowerState(ctx)
			if err != nil {
				s.Fatal("Failed to get EC System Power State")
			}
			if ps == "S0" {
				expectedVbusVoltage = 0
			}
			okToFail = false
		} else {
			if voltage < dutVoltageLimit {
				expectedVbusVoltage = float64(voltage)
			} else {
				expectedVbusVoltage = float64(dutVoltageLimit)
			}
			okToFail = isOverride || (voltage > dutVoltageLimit)
		}

		result, resultStr := compareVbus(ctx, h, s, expectedVbusVoltage, okToFail)
		if result == fail {
			s.Logf("%s FAIL", resultStr)
		} else {
			s.Logf("%s PASS", resultStr)
		}

		if result == fail {
			pdTesterFailures = append(pdTesterFailures, resultStr)
		}
	}

	// PDTester is set back to 20V SRC mode.
	err = charge(ctx, h, usbCMaxVoltage)
	if err != nil {
		s.Fatal("Failed to set charging voltage: ", usbCMaxVoltage)
	}

	if len(pdTesterFailures) > 0 {
		s.Error("PDTester voltage source cap failures")
		for _, fail := range pdTesterFailures {
			s.Error(fail)
		}
		number := len(pdTesterFailures)
		s.Fatal("PDTester failed ", number, " times")
	}

	// The DUT must be in SNK mode for the pd <port> dev <voltage>
	// command to have an effect.
	dutPDState, err := h.Servo.GetDUTPDState(ctx)
	if err != nil {
		s.Fatal("Failed to get DUT PD State")
	}

	if dutPDState.PowerRole != servo.PowerRoleSNK {
		// DUT needs to be in SINK Mode, attempt to force change
		h.Servo.SendPowerSwapRequest(ctx)

		if err := testing.Poll(ctx, func(ctx context.Context) error {
			if dutPDState, err = h.Servo.GetServoPDState(ctx); err == nil {
				if dutPDState.PowerRole != servo.PowerRoleSNK {
					s.Fatal("DUT not able to connecto in SINK mode")
				}
			} else {
				s.Fatal("Failed to get DUT PD State")
			}
			return nil
		}, &testing.PollOptions{Timeout: pdPowerRolePollTimeout, Interval: pdPowerRolePollInterval}); err != nil {
			s.Fatal("Failed to get DUT PD State")
		}
	}

	s.Log("Start of DUT initiated tests")
	testedVoltages := make(map[int]bool)
	for _, v := range voltageSequence {
		if v > dutVoltageLimit {
			s.Logf("Target %vV: skipped, over the limit %vV", v, dutVoltageLimit)
			continue
		}

		if !chargingVoltages[v] {
			s.Logf("Target %vV: skipped, voltage unsupported", v)
			continue
		}
		testedVoltages[v] = true

		// Build 'pd <port> dev <voltage> command
		err = h.Servo.SendRequestSourceVoltage(ctx, v)
		if err != nil {
			s.Fatal("Failed to request source voltage")
		}

		failCount := 0
		okToFail = isOverride || (v > dutVoltageLimit)
		err = testing.Poll(ctx, func(ctx context.Context) error {
			result, resultStr := compareVbus(ctx, h, s, float64(v), okToFail)
			if result == fail {
				failCount++
				if failCount >= maxPollFailCount {
					s.Logf("%s FAIL", resultStr)
					dutFailures = append(dutFailures, resultStr)
					return errors.Wrap(err, "FAIL")
				}
			} else {
				s.Logf("%s PASS", resultStr)
				return nil
			}
			return nil
		}, &testing.PollOptions{Timeout: pdVBusPollTimeout, Interval: pdVBusPollInterval})
	}
	if len(testedVoltages) < 3 {
		s.Error("Charger doesn't support 3 different voltages. Please try a different (i.e. 65w or greater) charger")
	}

	// Make sure DUT is set back to its max voltage so DUT will accept all
	// options
	err = h.Servo.SendRequestSourceVoltage(ctx, dutVoltageLimit)
	if err != nil {
		s.Fatal("Failed to request source voltage")
	}
	s.Log("Sleeping for 10 seconds")
	// GoBigSleepLint: Wait for voltage to stabilize
	if err = testing.Sleep(ctx, 10*time.Second); err != nil {
		s.Fatal("Failed to sleep for 10 seconds: ", err)
	}

	// The next group of tests need DUT to connect in SNK and SRC modes
	err = h.Servo.SetDualroleState(ctx, servo.DROn)
	if err != nil {
		s.Fatal("Failed to enable DualRole")
	}

	if len(dutFailures) > 0 {
		s.Log("DUT voltage request failures")
		for _, fail := range dutFailures {
			s.Log(fail)
		}
		number := len(dutFailures)
		s.Fatal("DUT failed ", number, " times")
	}
}
