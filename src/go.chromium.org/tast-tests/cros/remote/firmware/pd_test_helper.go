// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// This file implements boilerplate functionality for USB-PD tests.

package firmware

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
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

// SetupPDTester handles some boilerplate tasks to prepare the Servo for PD testing:
//  1. Make sure a suitable pair of Servos is attached (e.g. ServoV4 + Servo Micro)
//  2. If the DUT has a battery, charge it up to >= 10%
//  3. Configure DTS mode and CC polarity per user request
//  4. Ensure a UDB-PD charger brick is attached and sourcing power
//  5. Disable CCD Watchdogs
func SetupPDTester(ctx context.Context, h *Helper, ccPolarity CCPolarity, dtsMode DTSMode) error {
	if err := h.RequireServo(ctx); err != nil {
		return errors.Wrap(err, "failed to require servo")
	}

	// Ensure that a PD Tester is attached (such as Servo Micro or C2D2)
	if err := h.Servo.RequirePDTester(ctx); err != nil {
		return errors.Wrap(err, "servo configuration does not support PD testing")
	}
	testing.ContextLog(ctx, "PD Tester found")

	// Make sure the Servo is a PD source from the DUT's perspective. This
	// helps in case a previous test left the port in a strange state.
	if err := h.Servo.SetPDRole(ctx, servo.PDRoleSrc); err != nil {
		return errors.Wrap(err, "servo must be sourcing power to the DUT")
	}

	// Ensure that the Servo has a USB charger attached.
	// Note: The above command succeeds even if no charger is present
	if err := h.Servo.RequireChargerAttached(ctx); err != nil {
		return errors.Wrap(err, "servo must have a charger attached that is sourcing")
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
		if pdState.PEStateName != "PD_STATE_SRC_READY" {
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
