// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package tabletmode

import (
	"context"
	"regexp"

	"chromiumos/tast/errors"
	"chromiumos/tast/ssh"
	"chromiumos/tast/testing"
)

// Control defines functions to switch between tabletmode and laptopmode and
// resets the state of the device to its original orientation.
type Control interface {
	InitControl(ctx context.Context, dutConn *ssh.Conn) error
	ForceTabletMode(ctx context.Context) error
	ForceLaptopMode(ctx context.Context) error
	Reset(ctx context.Context) error
}

// ConvertibleModeControl implements the Control interface and saves the
// original state for tablet_mode_angle if needed.
type ConvertibleModeControl struct {
	conn      *ssh.Conn
	origAngle string
	origHyst  string
}

var errDutConnNilOnInit = errors.New("nil value passed to InitControl for dutConn parameter")
var errConnNil = errors.New("conn member unitialized for Control instance")

// InitControl initializes the saved state of a convertible if needed and sets
// the conn member.
func (cmc *ConvertibleModeControl) InitControl(ctx context.Context, dutConn *ssh.Conn) error {
	if dutConn == nil {
		return errDutConnNilOnInit
	}

	// Check whether tabletmode is supported.
	var err error
	if err = dutConn.CommandContext(ctx, "ectool", "tabletmode", "reset").Run(); err == nil {
		// The tabletmode command doesn't need to save state. You just need to
		// run it again with the reset arg.
		testing.ContextLog(ctx, "Using tabletmode command to control mode")
		cmc.conn = dutConn
		cmc.origAngle = ""
		cmc.origHyst = ""
		return nil
	}

	var out []byte
	if out, err = dutConn.CommandContext(ctx, "ectool", "motionsense", "tablet_mode_angle").Output(); err != nil {
		return err
	}

	re := regexp.MustCompile(`tablet_mode_angle=(\d+) hys=(\d+)`)
	m := re.FindSubmatch(out)
	if len(m) != 3 {
		return errors.Errorf("could not parse tablet_mode_angle from: %s", out)
	}

	testing.ContextLog(ctx, "Using motionsense tablet_mode_angle command to control mode")
	cmc.conn = dutConn
	cmc.origAngle = string(m[1])
	cmc.origHyst = string(m[2])
	return nil
}

// ForceTabletMode sets the system to tabletmode using the supported method
// found in InitControl.
func (cmc *ConvertibleModeControl) ForceTabletMode(ctx context.Context) error {
	if cmc.conn == nil {
		return errConnNil
	}
	if cmc.origAngle == "" {
		return cmc.conn.CommandContext(ctx, "ectool", "tabletmode", "on").Run()
	}

	return cmc.conn.CommandContext(ctx, "ectool", "motionsense", "tablet_mode_angle", "0", "0").Run()
}

// ForceLaptopMode sets the system to laptopmode using the supported method
// found in InitControl.
func (cmc *ConvertibleModeControl) ForceLaptopMode(ctx context.Context) error {
	if cmc.conn == nil {
		return errConnNil
	}
	if cmc.origAngle == "" {
		return cmc.conn.CommandContext(ctx, "ectool", "tabletmode", "off").Run()
	}

	return cmc.conn.CommandContext(ctx, "ectool", "motionsense", "tablet_mode_angle", "360", "0").Run()
}

// Reset restores the original state of the system (maybe recorded in
// InitControl if needed).
func (cmc *ConvertibleModeControl) Reset(ctx context.Context) error {
	if cmc.conn == nil {
		return errConnNil
	}
	if cmc.origAngle == "" {
		return cmc.conn.CommandContext(ctx, "ectool", "tabletmode", "reset").Run()
	}

	return cmc.conn.CommandContext(ctx, "ectool", "motionsense", "tablet_mode_angle", cmc.origAngle, cmc.origHyst).Run()
}

// DetachableModeControl implements the Control interface for detachables.
type DetachableModeControl struct {
	conn *ssh.Conn
}

// InitControl sets the conn member and checks that the basestate ectool command
// is supported.
func (dmc *DetachableModeControl) InitControl(ctx context.Context, dutConn *ssh.Conn) error {
	if dutConn == nil {
		return errDutConnNilOnInit
	}
	dmc.conn = dutConn
	return dmc.conn.CommandContext(ctx, "ectool", "basestate", "reset").Run()
}

// ForceTabletMode sets the base of the detachable to detached to force
// tabletmode.
func (dmc *DetachableModeControl) ForceTabletMode(ctx context.Context) error {
	if dmc.conn == nil {
		return errConnNil
	}
	return dmc.conn.CommandContext(ctx, "ectool", "basestate", "detach").Run()
}

// ForceLaptopMode sets the base of the detachable to attached to force
// laptopmode.
func (dmc *DetachableModeControl) ForceLaptopMode(ctx context.Context) error {
	if dmc.conn == nil {
		return errConnNil
	}
	return dmc.conn.CommandContext(ctx, "ectool", "basestate", "attach").Run()
}

// Reset removes any overrides to the basestate (caused by calls to
// ForceTabletMode, ForceLaptopMode, or other methods).
func (dmc *DetachableModeControl) Reset(ctx context.Context) error {
	if dmc.conn == nil {
		return errConnNil
	}
	return dmc.conn.CommandContext(ctx, "ectool", "basestate", "reset").Run()
}
