// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package tabletmode

import (
	"context"
	"regexp"

	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// Control defines functions to switch between tabletmode and laptopmode and
// resets the state of the device to its original orientation.
type Control interface {
	InitControl(ctx context.Context, dut *dut.DUT) error
	ForceTabletMode(ctx context.Context) error
	ForceLaptopMode(ctx context.Context) error
	Reset(ctx context.Context) error
}

// ConvertibleModeControl implements the Control interface and saves the
// original state for tablet_mode_angle if needed.
type ConvertibleModeControl struct {
	dut       *dut.DUT
	origAngle string
	origHyst  string
}

var errDutNilOnInit = errors.New("nil value passed to InitControl for dut parameter")
var errDutNil = errors.New("dut member unitialized for Control instance")

// InitControl initializes the saved state of a convertible if needed and sets
// the dut member.
func (cmc *ConvertibleModeControl) InitControl(ctx context.Context, dut *dut.DUT) error {
	if dut == nil {
		return errDutNilOnInit
	}

	// Check whether tabletmode is supported.
	var err error
	if err = dut.Conn().CommandContext(ctx, "ectool", "tabletmode", "reset").Run(); err == nil {
		// The tabletmode command doesn't need to save state. You just need to
		// run it again with the reset arg.
		testing.ContextLog(ctx, "Using tabletmode command to control mode")
		cmc.dut = dut
		cmc.origAngle = ""
		cmc.origHyst = ""
		return nil
	}

	var out []byte
	if out, err = dut.Conn().CommandContext(ctx, "ectool", "motionsense", "tablet_mode_angle").Output(); err != nil {
		return err
	}

	re := regexp.MustCompile(`tablet_mode_angle=(\d+) hys=(\d+)`)
	m := re.FindSubmatch(out)
	if len(m) != 3 {
		return errors.Errorf("could not parse tablet_mode_angle from: %s", out)
	}

	testing.ContextLog(ctx, "Using motionsense tablet_mode_angle command to control mode")
	cmc.dut = dut
	cmc.origAngle = string(m[1])
	cmc.origHyst = string(m[2])
	return nil
}

// ForceTabletMode sets the system to tabletmode using the supported method
// found in InitControl.
func (cmc *ConvertibleModeControl) ForceTabletMode(ctx context.Context) error {
	if cmc.dut == nil {
		return errDutNil
	}
	if cmc.origAngle == "" {
		return cmc.dut.Conn().CommandContext(ctx, "ectool", "tabletmode", "on").Run()
	}

	return cmc.dut.Conn().CommandContext(ctx, "ectool", "motionsense", "tablet_mode_angle", "0", "0").Run()
}

// ForceLaptopMode sets the system to laptopmode using the supported method
// found in InitControl.
func (cmc *ConvertibleModeControl) ForceLaptopMode(ctx context.Context) error {
	if cmc.dut == nil {
		return errDutNil
	}
	if cmc.origAngle == "" {
		return cmc.dut.Conn().CommandContext(ctx, "ectool", "tabletmode", "off").Run()
	}

	return cmc.dut.Conn().CommandContext(ctx, "ectool", "motionsense", "tablet_mode_angle", "360", "0").Run()
}

// Reset restores the original state of the system (maybe recorded in
// InitControl if needed).
func (cmc *ConvertibleModeControl) Reset(ctx context.Context) error {
	if cmc.dut == nil {
		return errDutNil
	}
	if cmc.origAngle == "" {
		return cmc.dut.Conn().CommandContext(ctx, "ectool", "tabletmode", "reset").Run()
	}

	return cmc.dut.Conn().CommandContext(ctx, "ectool", "motionsense", "tablet_mode_angle", cmc.origAngle, cmc.origHyst).Run()
}

// DetachableModeControl implements the Control interface for detachables.
type DetachableModeControl struct {
	dut *dut.DUT
}

// InitControl sets the dut member and checks that the basestate ectool command
// is supported.
func (dmc *DetachableModeControl) InitControl(ctx context.Context, dut *dut.DUT) error {
	if dut == nil {
		return errDutNilOnInit
	}
	dmc.dut = dut
	return dmc.dut.Conn().CommandContext(ctx, "ectool", "basestate", "reset").Run()
}

// ForceTabletMode sets the base of the detachable to detached to force
// tabletmode.
func (dmc *DetachableModeControl) ForceTabletMode(ctx context.Context) error {
	if dmc.dut == nil {
		return errDutNil
	}
	return dmc.dut.Conn().CommandContext(ctx, "ectool", "basestate", "detach").Run()
}

// ForceLaptopMode sets the base of the detachable to attached to force
// laptopmode.
func (dmc *DetachableModeControl) ForceLaptopMode(ctx context.Context) error {
	if dmc.dut == nil {
		return errDutNil
	}
	return dmc.dut.Conn().CommandContext(ctx, "ectool", "basestate", "attach").Run()
}

// Reset removes any overrides to the basestate (caused by calls to
// ForceTabletMode, ForceLaptopMode, or other methods).
func (dmc *DetachableModeControl) Reset(ctx context.Context) error {
	if dmc.dut == nil {
		return errDutNil
	}
	return dmc.dut.Conn().CommandContext(ctx, "ectool", "basestate", "reset").Run()
}
