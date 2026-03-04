// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package utils used to do some component excution function.
package utils

import (
	"context"
	"strings"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	sshPollingTimeout = 60 * time.Second
)

// ConnectPeripheralsViaDock connects the peripherals via the dock, verifies each connection and returns the list of USB devices.
// Peripherals devices such as external display, ethernet, USB (audio) devices.
func ConnectPeripheralsViaDock(ctx context.Context, dut *dut.DUT, extDispID, ethernetID string, usbTypeADeviceIDs []string) ([]string, error) {
	if err := ControlFixture(ctx, extDispID, "on"); err != nil {
		return nil, errors.Wrap(err, "failed to connect to the external display")
	}

	if err := ControlFixture(ctx, ethernetID, "on"); err != nil {
		return nil, errors.Wrap(err, "failed to connect to the Ethernet dongle")
	}

	for _, deviceID := range usbTypeADeviceIDs {
		if err := ControlFixture(ctx, deviceID, "on"); err != nil {
			return nil, errors.Wrapf(err, "failed to connect to the USB Type-A device: %s", deviceID)
		}
	}

	usbDevices, err := GetStableUSBDevices(ctx, dut)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get a list of USB devices")
	}

	if err := VerifyPeripheralsConnection(ctx, dut, true, usbDevices); err != nil {
		return nil, errors.Wrap(err, "failed to verify connections to the peripherals")
	}

	return usbDevices, nil
}

// GetStableUSBDevices returns the list of USB devices if the list is stable for 2 seconds.
func GetStableUSBDevices(ctx context.Context, dut *dut.DUT) ([]string, error) {
	var previous []string
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		current, err := GetUSBDevice(ctx, dut)
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to get the USB devices"))
		}

		stringLess := func(a, b string) bool {
			return a < b
		}

		if !cmp.Equal(previous, current, cmpopts.SortSlices(stringLess)) {
			previous = current
			return errors.Errorf("expect the list of USB devices is stable for 2s, but it's not; got %v", strings.Join(current, "\n"))
		}
		return nil
	}, &testing.PollOptions{Timeout: sshPollingTimeout, Interval: 2 * time.Second}); err != nil {
		return nil, err
	}
	return previous, nil
}
