// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// This file implements boilerplate functionality for USB-PD tests.

package firmware

import (
	"context"
	"time"

	"fmt"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"

	"go.chromium.org/tast/core/testing/hwdep"
)

// DTSMode is a type for DTS Mode control in SetupPDTester
type DTSMode int

// USB-C DTS mode options
const (
	DTSModeOn DTSMode = iota
	DTSModeOff
)

// CCPolarity is a type for CC Polarity control in SetupPDTester
type CCPolarity int

// USB-C CC line polarity options
const (
	// Maps to "cc1"
	CCPolarityStandard CCPolarity = iota
	// Maps to "cc2"
	CCPolarityFlipped
)

// PDTestParams contains common test params for PD tests.
type PDTestParams struct {
	CC           CCPolarity
	DTS          DTSMode
	Shutdown     bool
	Suspend      bool
	RequiredPort *int
}

// AddPDPorts takes a list of testing.Params that use PDTestParams as their value and populates the PVS requirements and creates parameterized tests for each PD port.
// This is a somewhat hacky workaround for the PD port not being a first class dimension in PVS. See b/291571959 for more info.
func AddPDPorts(params []testing.Param, attrs []string) []testing.Param {
	var result []testing.Param
	for _, test := range params {
		test.ExtraAttr = attrs
		result = append(result, test)
	}
	for port := 0; port < servo.MaxPorts; port++ {
		for _, test := range params {
			if test.Name == "" {
				test.Name = fmt.Sprintf("port%d", port)
			} else {
				test.Name = fmt.Sprintf("%s_port%d", test.Name, port)
			}
			test.ExtraAttr = nil
			test.ExtraRequirements = []string{"sys-fw-0023-v01"}
			test.ExtraHardwareDeps = hwdep.D(hwdep.HasPDPort(uint32(port)))
			val := test.Val.(PDTestParams)
			val.RequiredPort = new(int)
			*val.RequiredPort = port
			test.Val = val
			result = append(result, test)
		}
	}
	return result
}

// SetupPDTester handles some boilerplate tasks to prepare the Servo for PD testing:
//  1. Make sure a suitable pair of Servos is attached (e.g. ServoV4 + Servo Micro)
//  2. Call RequireDUTPDInfo to get info about the connected port.
//  3. If the DUT has a battery, charge it up to >= 10%
//  4. Configure DTS mode and CC polarity per user request
//  5. Ensure a UDB-PD charger brick is attached and sourcing power
//  6. Disable CCD Watchdogs
func SetupPDTester(ctx context.Context, h *Helper, ccPolarity CCPolarity, dtsMode DTSMode, requiredPort *int) error {
	if err := h.RequireServo(ctx); err != nil {
		return errors.Wrap(err, "failed to require servo")
	}

	// Ensure that a PD Tester is attached (such as Servo Micro or C2D2)
	if err := h.Servo.RequirePDTester(ctx); err != nil {
		return errors.Wrap(err, "servo configuration does not support PD testing")
	}
	testing.ContextLog(ctx, "PD Tester found")

	// Turn off CC / PD communication on the Servo to help reset any bad state from a
	// previous test. The subsequent SetPDRole call will restore PD comms for both
	// Servo ports.
	if err := h.Servo.ServoCcOff(ctx); err != nil {
		return errors.Wrap(err, "cannot force CC off on Servo")
	}

	// Ensure that the Servo has a USB charger attached.
	// Note: The above command succeeds even if no charger is present
	if err := h.Servo.RequireChargerAttached(ctx); err != nil {
		return errors.Wrap(err, "servo must have a charger attached that is sourcing")
	}

	// Make sure the Servo is a PD source from the DUT's perspective. This
	// helps in case a previous test left the port in a strange state.
	if err := h.Servo.SetPDRole(ctx, servo.PDRoleSrc); err != nil {
		return errors.Wrap(err, "servo must be sourcing power to the DUT")
	}

	// Poll on the Servo C1 (DUT-facing) port until it is source-ready
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		pdState, err := h.Servo.GetServoPDState(ctx)
		if err != nil {
			return testing.PollBreak(
				errors.Wrap(err, "cannot access servo DUT port (C1) PD status"),
			)
		}

		testing.ContextLogf(ctx, "Servo DUT port (C1) PE State is %s", pdState.PEStateName)

		if !pdState.IsSourceReady() {
			return errors.New("Servo DUT port (C1) is not src-ready")
		}

		return nil
	}, &testing.PollOptions{Interval: time.Second, Timeout: 5 * time.Second}); err != nil {
		return errors.Wrap(err, "timed out waiting for servo DUT port to source power")
	}

	if err := h.Servo.RequireDUTPDInfo(ctx); err != nil {
		return errors.Wrap(err, "RequireDUTPDInfo failed")
	}
	if requiredPort != nil && h.Servo.DUTPDPort() != *requiredPort {
		return errors.Errorf("Incorrect PD port. Test wants port %d, got %d", *requiredPort, h.Servo.DUTPDPort())
	}

	// If a battery is present, ensure it is charged to at least minBattLevel percent
	hasBattery := h.Config.HasECCapability(ECBattery)
	testing.ContextLogf(ctx, "ECCapBattery: %t", hasBattery)

	if hasBattery {
		minBattLevel := 10
		if err := ChargeToLevel(ctx, h, minBattLevel, 10*time.Minute); err != nil {
			return errors.Wrap(err, "cannot charge battery")
		}

		cs, err := GetChargingState(ctx, h)
		if err != nil {
			return errors.Wrap(err, "cannot get charging state")
		}

		testing.ContextLogf(ctx, "battery capacity at test start: %s", cs["batt.state_of_charge"])
	}

	// Set DTS mode on the servo
	var dts servo.OnOffValue
	switch dtsMode {
	case DTSModeOn:
		dts = servo.On
	case DTSModeOff:
		dts = servo.Off
	default:
		panic("Invalid DTS Mode setting")
	}
	testing.ContextLogf(ctx, "Setting DTS Mode to %q", dts)
	if err := h.Servo.SetOnOff(ctx, servo.DTSMode, dts); err != nil {
		return errors.Wrap(err, "failed to set Servo DTS mode")
	}

	// Set USB-PD CC line polarity.
	var cc string
	switch ccPolarity {
	case CCPolarityStandard:
		cc = "cc1"
	case CCPolarityFlipped:
		cc = "cc2"
	default:
		panic("Invalid CC Polarity setting")
	}
	testing.ContextLogf(ctx, "Setting CC polarity to %s", cc)
	if err := h.Servo.SetString(ctx, servo.USBCPolarity, cc); err != nil {
		return errors.Wrap(err, "failed to set Servo USBC CC polarity")
	}

	// Modifying CC and DTS settings causes the Servo DUT port to reset. Wait a bit until
	// it reaches the source ready state.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		pdState, err := h.Servo.GetServoPDState(ctx)
		if err != nil {
			return testing.PollBreak(err)
		}
		testing.ContextLogf(ctx, "Servo DUT port PE State: %s", pdState.PEStateName)
		if !pdState.IsSourceReady() {
			return errors.New("Servo DUT port is not ready")
		}
		return nil
	}, &testing.PollOptions{Interval: time.Second, Timeout: 5 * time.Second}); err != nil {
		return errors.Wrap(err, "timed out waiting for Servo DUT port to be ready")
	}

	// Turn off CCD watchdogs as this can interfere with PD tests.
	if err := h.Servo.RemoveCCDWatchdogs(ctx); err != nil {
		return errors.Wrap(err, "failed to disable CCD watchdogs")
	}

	return nil
}

// ShutdownDUT is a helper function for commanding the DUT to shutdown and waiting
// until it does so with a timeout.
func ShutdownDUT(ctx context.Context, h *Helper) error {
	// Run shutdown command via SSH
	testing.ContextLog(ctx, "Shutting down DUT")
	if err := h.DUT.Conn().CommandContext(ctx, "/sbin/shutdown", "-P", "now").Start(); err != nil {
		return errors.Wrap(err, "failed to run `/sbin/shutdown -P now` cmd")
	}

	// Wait for shutdown
	testing.ContextLog(ctx, "Waiting for G3 power state")
	if err := h.WaitForPowerStates(ctx, PowerStateInterval, PowerStateTimeout, "G3"); err != nil {
		return errors.Wrap(err, "failed to get G3 power state")
	}

	return nil
}
