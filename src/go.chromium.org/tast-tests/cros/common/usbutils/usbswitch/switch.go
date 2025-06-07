// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package usbswitch contains the usb switch interface and data types.
package usbswitch

import (
	"context"
)

// Switch interface for a usb switch device
type Switch interface {
	DisablePorts(context.Context) error              // Disable all ports.
	EnablePort(context.Context) error                // Enable the used port.
	SetActiveSwitchPort(context.Context, int) error  // Should NOT mutate active port state.
	Close(context.Context) error                     // Close the switch.
	GetType() SwitchType                             // Get the type of the switch.
	EnterMode(context.Context, ConnectionMode) error // Enter particular USB mode.
	FlipOrientation(context.Context, bool) error     // Flip the orientation of the USB plug.
}

// SwitchType is the type of the switch.
type SwitchType int

const (
	Mcci SwitchType = iota
	UTC274
)

// ConnectionMode is the mode of the connection.
type ConnectionMode int

const (
	Usb2Mode ConnectionMode = iota
	Usb3Mode
	DpMode
	TBT4Mode
)

// String returns a string representation of the ConnectionMode.
func (cm ConnectionMode) String() string {
	switch cm {
	case Usb2Mode:
		return "USB2Mode"
	case Usb3Mode:
		return "USB3Mode"
	case DpMode:
		return "DPMode"
	case TBT4Mode:
		return "TBT4Mode"
	default:
		return "UnknownMode"
	}
}
