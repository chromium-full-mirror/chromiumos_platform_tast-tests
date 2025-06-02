// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package typecswitch contains the usb switch interface for the tests in the typec directory.
package typecswitch

import (
	"context"
)

// Switch interface for a usb switch device
type Switch interface {
	DisablePorts(context.Context) error
	EnablePort(context.Context) error
	DevicePort(context.Context) (int, error)
	TestPort(context.Context) (int, error)
	Close(context.Context) error
	GetType() string
	EnterUsb2Mode(context.Context) error
	EnterUsb3Mode(context.Context) error
	EnterDpMode(context.Context) error
}
