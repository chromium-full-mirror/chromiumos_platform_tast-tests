// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package typecswitch contains the usb switch interface for the tests in the typec directory.
package typecswitch

import (
	"context"
	"strconv"

	"go.chromium.org/tast-tests/cros/common/usbutils/unigraf"
	"go.chromium.org/tast-tests/cros/remote/typec/mcci"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
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

// GetSwitch returns an interface for the usb switch.
func GetSwitch(ctx context.Context, s *testing.State) (Switch, error) {
	if unigrafURI, unigrafPresent := s.Var("typec.UnigrafUri"); unigrafPresent {
		unigrafObj, err := unigraf.New(ctx, unigrafURI)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create unigraf object")
		}
		return unigrafObj, nil

	} else if mcciSerial, mcciPresent := s.Var("typec.McciSerial"); mcciPresent {
		path, _ := s.Var("typec.McciPath")
		portStr, _ := s.Var("typec.McciPort")
		portUsed, err := strconv.Atoi(portStr)
		if err != nil {
			return nil, errors.Wrap(err, "failed to parse MCCI port cmdline argument")
		}

		mcciObj, err := mcci.GetSwitch(mcciSerial, path, portUsed)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get MCCI switch handle")
		}

		return mcciObj, nil
	}

	return nil, errors.New("failed to parse usb switch device arguments")
}
