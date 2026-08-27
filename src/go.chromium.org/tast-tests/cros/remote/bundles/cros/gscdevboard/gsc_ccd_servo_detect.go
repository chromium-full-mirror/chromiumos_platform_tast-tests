// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/utils"
	"go.chromium.org/tast/core/testing"
)

var hypEcUartEnabled bool

type expectedServoDetectResult struct {
	rdd     ti50.CCDStateStandardVal
	ccdMode ti50.CCDStateStandardVal
	servo   ti50.CCDStateStandardVal
	ccdUART ti50.CCDStateStandardVal
}

// Map the commands run to their expected ccdstate values.
var testServoDetectStates = map[string]expectedServoDetectResult{
	// GSC detects servo is connected.
	"servo_connect": expectedServoDetectResult{
		rdd:     ti50.CCDStateOff,
		ccdMode: ti50.CCDStateOff,
		servo:   ti50.CCDStateOn,
		ccdUART: ti50.CCDStateOff,
	},
	// GSC can detect Rdd when servo is connected.
	"servo_connect, rdd_connect": expectedServoDetectResult{
		rdd:     ti50.CCDStateOn,
		ccdMode: ti50.CCDStateOn,
		servo:   ti50.CCDStateOn,
		ccdUART: ti50.CCDStateOff,
	},
	// GSC enables CCD EC uart when servo is disconnected.
	"servo_connect, rdd_connect, servo_disconnect": expectedServoDetectResult{
		rdd:     ti50.CCDStateOn,
		ccdMode: ti50.CCDStateOn,
		servo:   ti50.CCDStateUndetectable,
		ccdUART: ti50.CCDStateOn,
	},
	// GSC can detect servo after it reboots.
	"servo_connect, gsc_reboot": expectedServoDetectResult{
		rdd:     ti50.CCDStateOff,
		ccdMode: ti50.CCDStateOff,
		servo:   ti50.CCDStateOn,
		ccdUART: ti50.CCDStateOff,
	},
	// GSC detects Rdd is connected.
	"rdd_connect": expectedServoDetectResult{
		rdd:     ti50.CCDStateOn,
		ccdMode: ti50.CCDStateOn,
		servo:   ti50.CCDStateUndetectable,
		ccdUART: ti50.CCDStateOn,
	},
	// GSC cannot detect servo when it already has the CCD EC uart enabled.
	"rdd_connect, servo_connect": expectedServoDetectResult{
		rdd:     ti50.CCDStateOn,
		ccdMode: ti50.CCDStateOn,
		servo:   ti50.CCDStateUndetectable,
		ccdUART: ti50.CCDStateOn,
	},
	// GSC can detect servo when it reboots even if CCD was enabled before the reset.
	"rdd_connect, servo_connect, gsc_reboot": expectedServoDetectResult{
		rdd:     ti50.CCDStateOn,
		ccdMode: ti50.CCDStateOn,
		servo:   ti50.CCDStateOn,
		ccdUART: ti50.CCDStateOff,
	},
	// If GSC resets and servo is connected, it'll enable CCD EC uart after servo disconnects.
	"rdd_connect, servo_connect, gsc_reboot, servo_disconnect": expectedServoDetectResult{
		rdd:     ti50.CCDStateOn,
		ccdMode: ti50.CCDStateOn,
		servo:   ti50.CCDStateUndetectable,
		ccdUART: ti50.CCDStateOn,
	},
	// GSC can detect servo after Rdd disconnects.
	"rdd_connect, servo_connect, rdd_disconnect": expectedServoDetectResult{
		rdd:     ti50.CCDStateOff,
		ccdMode: ti50.CCDStateOff,
		servo:   ti50.CCDStateOn,
		ccdUART: ti50.CCDStateOff,
	},
	// GSC can detect servo after Rdd disconnects.
	"rdd_connect, servo_disconnect": expectedServoDetectResult{
		rdd:     ti50.CCDStateOn,
		ccdMode: ti50.CCDStateOn,
		servo:   ti50.CCDStateUndetectable,
		ccdUART: ti50.CCDStateOn,
	},
	// GSC can detect servo is disconnected after Rdd disconnect.
	"rdd_connect, servo_disconnect, rdd_disconnect": expectedServoDetectResult{
		rdd:     ti50.CCDStateOff,
		ccdMode: ti50.CCDStateOff,
		servo:   ti50.CCDStateOff,
		ccdUART: ti50.CCDStateOff,
	},
}

func init() {
	testing.AddTest(&testing.Test{
		Func:    GSCCCDServoDetect,
		Desc:    "Tests GSC servo detection",
		Timeout: 90 * time.Second,
		Contacts: []string{
			"cros-hwsec@google.com", // CrOS GSC Developers
			"mruthven@chromium.org", // Test Author
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr: []string{"group:gsc",
			"gsc_dt_ab", "gsc_dt_shield", "gsc_h1_shield", "gsc_ot_fpga_cw310", "gsc_ot_shield",
			"gsc_image_ti50",
			"gsc_nightly"},
		Fixture: fixture.GSCOpenCCD,
		Params: []testing.Param{{
			Name: "enable_uart_on_servo_disconnect",
			Val:  []string{"servo_connect", "rdd_connect", "servo_disconnect"},
		}, {
			Name: "detect_after_reboot",
			Val:  []string{"servo_connect", "gsc_reboot"},
		}, {
			Name: "enable_uart_after_gsc_reboot_and_servo_disconnect",
			Val:  []string{"rdd_connect", "servo_connect", "gsc_reboot", "servo_disconnect"},
		}, {
			Name: "update_servo_connected_after_rdd_disconnect",
			Val:  []string{"rdd_connect", "servo_connect", "rdd_disconnect"},
		}, {
			Name: "update_servo_disconnected_after_rdd_disconnect",
			Val:  []string{"rdd_connect", "servo_disconnect", "rdd_disconnect"},
		}},
	})
}

func GSCCCDServoDetect(ctx context.Context, s *testing.State) {
	b := utils.NewDevboardHelper(s, s.FixtValue().(*fixture.Value))
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}
	i := ti50.MustOpenCrOSImage(ctx, b, s, b.TestbedType)
	defer i.Close(ctx)

	commands := s.Param().([]string)
	hypEcUartEnabled = true

	b.Reset(ctx)
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")

	runSetupCommand(ctx, b, th, i, "servo_disconnect")
	runSetupCommand(ctx, b, th, i, "rdd_disconnect")
	// Turn on the AP.
	b.GpioSet(ctx, ti50.GpioTi50PltRstL, true)

	var caseStr string
	// Iterate through all of the commands.
	for index, command := range commands {
		if caseStr == "" {
			caseStr = command
		} else {
			caseStr = fmt.Sprintf("%s, %s", caseStr, command)
		}
		expected, exists := testServoDetectStates[caseStr]
		if !exists {
			s.Fatal("key not found in test map: ", caseStr)
		}

		s.Logf("%d setting %s", index, command)
		s.Log("commands: ", caseStr)
		s.Logf("Expected result: %+v", expected)
		runCommandCheckState(ctx, s, b, th, i, command, expected, caseStr)
		s.Logf("%s: ok", caseStr)
	}
}

func runCommandCheckState(ctx context.Context, s *testing.State, b utils.DevboardHelper, th utils.FirmwareTestingHelper, i *ti50.CrOSImage, command string, expected expectedServoDetectResult, caseStr string) {
	seed := time.Now().UnixNano()
	s.Logf("Random seed: %d", seed)
	r := rand.New(rand.NewSource(seed))

	runSetupCommand(ctx, b, th, i, command)
	var errors []string

	ccdstate, err := i.CCDStateInfo(ctx)
	th.MustSucceed(err, "failed to get ccdstate output")
	s.Logf("ccdstate: %+v", ccdstate)

	gpio, err := i.Command(ctx, "gpioget")
	th.MustSucceed(err, "failed to get gpioget output")
	s.Logf("gpio: %+v", gpio)

	if expected.rdd != ccdstate.Rdd.State {
		errors = append(errors, fmt.Sprintf("rdd mismatch: expected %s got %s", expected.rdd, ccdstate.Rdd))
	}
	if expected.ccdMode != ccdstate.CCDModeSignal.State {
		errors = append(errors, fmt.Sprintf("CCD_MODE mismatch: expected %s got %s", expected.ccdMode, ccdstate.CCDModeSignal))
	}
	if expected.servo != ccdstate.Servo.State {
		errors = append(errors, fmt.Sprintf("servo mismatch: expected %s got %s", expected.servo, ccdstate.Servo))
	}

	uartTxEnabled := expected.ccdUART == ti50.CCDStateOn

	if strings.Contains(ccdstate.StateFlags, "UARTEC+TX") {
		if !uartTxEnabled {
			errors = append(errors, fmt.Sprintf("unexpected State Flags: +TX found in %s", ccdstate.StateFlags))
		}
	} else if uartTxEnabled {
		errors = append(errors, fmt.Sprintf("unexpected State Flags: +TX not found in %s", ccdstate.StateFlags))
	}

	if len(errors) != 0 {
		s.Fatalf("%s: %s", caseStr, errors)
		return
	}
	if ccdstate.Rdd.State != ti50.CCDStateOn {
		return
	}

	var ecUartTxEnabled bool
	if b.TestbedType == ti50.GscH1Shield {
		ecUartTxEnabled = uartTxEnabled
	} else {
		// Ti50 shields cannot connect Servo and read the EC UART.
		// The shield will not be able to read the UART when it's
		// simulating servo connected.
		ecUartTxEnabled = hypEcUartEnabled && uartTxEnabled
	}

	s.Log("Testing UARTs")
	// Test forwarding on each of three ports.
	if err = b.TestUARTForwarding(ctx, r, ti50.UartEC, true, ecUartTxEnabled, caseStr); err != nil {
		s.Errorf("EC UART failed: %s", err)
	}
	if err = b.TestUARTForwarding(ctx, r, ti50.UartAP, true, uartTxEnabled, caseStr); err != nil {
		s.Errorf("AP UART failed: %s", err)
	}
	if !b.GscProperties().HasFpmcuUart() {
		return
	}
	if err = b.TestUARTForwarding(ctx, r, ti50.UartFPMCU, true, uartTxEnabled, caseStr); err != nil {
		s.Errorf("FPMCU UART failed: %s", err)
	}
}

func runSetupCommand(ctx context.Context, b utils.DevboardHelper, th utils.FirmwareTestingHelper, i *ti50.CrOSImage, command string) {
	switch command {
	case "gsc_reboot":
		th.MustSucceed(i.Reboot(ctx), "failed to reboot gsc")
	case "rdd_disconnect":
		b.GpioApplyStrap(ctx, ti50.CcdDisconnected)
	case "rdd_connect":
		b.GpioApplyStrap(ctx, ti50.CcdSuzyQ)
	case "servo_disconnect":
		b.GpioApplyStrap(ctx, ti50.ServoMicroDisconnected)
		// For servo disconnect DT shields put the uart signal back in
		// alternate mode. It can read EC UART.
		hypEcUartEnabled = true
	case "servo_connect":
		b.GpioApplyStrap(ctx, ti50.ServoMicroConnected)
		// The DT shields have to output high to simulate servo connect.
		// It isn't in Alternate mode, so it can't read EC UART.
		hypEcUartEnabled = false
	}
	// Wait until GSC stops debouncing any ccdstate.
	th.MustSucceed(i.WaitForStableCCDState(ctx), "failed to wait for stable ccdstate")
}
