// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bluetooth

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/usbdevice"
	"go.chromium.org/tast-tests/cros/common/xmlrpc"
)

// KeyboardPeripheral is an interface for making RPC calls to a chameleond daemon
// targeting a specific bluetooth keyboard peripheral chameleon device flow.
//
// This is based off of the Python class "chameleond.devices.raspi_bluetooth_flow.RaspiHIDKeyboard"
// from the chameleon source. Refer to that source for more complete
// documentation.
type KeyboardPeripheral interface {
	BluezPeripheral

	// KeyboardSendTrace calls the Chameleond RPC method of the same name.
	// Sends scan codes from a data trace over the BT link.
	KeyboardSendTrace(ctx context.Context, inputScanCodes []int) error

	// KeyboardSendString calls the Chameleond RPC method of the same name.
	// Sends characters one-by-one over the BT link.
	KeyboardSendString(ctx context.Context, stringToSend string) error

	// KeyboardSendKeyEvent converts the incoming key into a Bluetooth HID report and
	// sends the event by invoking the 'KeyboardSendTrace' Chameleond RPC method.
	KeyboardSendKeyEvent(ctx context.Context, key byte) error
}

// CommonKeyboardPeripheral is a base implementation of KeyboardPeripheral that
// provides methods for making XMLRPC calls to a chameleond daemon.
// See the KeyboardPeripheral interface for more detailed documentation.
type CommonKeyboardPeripheral struct {
	CommonBluezPeripheral
}

// NewCommonKeyboardPeripheral creates a new instance of
// CommonKeyboardPeripheral.
func NewCommonKeyboardPeripheral(xmlrpcClient *xmlrpc.XMLRpc, methodNamePrefix string) *CommonKeyboardPeripheral {
	return &CommonKeyboardPeripheral{
		CommonBluezPeripheral: *NewCommonBluezPeripheral(xmlrpcClient, methodNamePrefix),
	}
}

// KeyboardSendTrace calls the Chameleond RPC method of the same name.
// This implements KeyboardPeripheral.KeyboardSendTrace, see that for more
// details.
func (c *CommonKeyboardPeripheral) KeyboardSendTrace(ctx context.Context, inputScanCodes []int) error {
	return c.RPC("KeyboardSendTrace").Args(inputScanCodes).Call(ctx)
}

// KeyboardSendKeyEvent converts the incoming key into a Bluetooth HID report and
// sends the event by invoking the 'KeyboardSendTrace' Chameleond RPC method.
func (c *CommonKeyboardPeripheral) KeyboardSendKeyEvent(ctx context.Context, key byte) error {
	// Leverage the `usbdevice` package to encode key codes, it currently allows for the setup of only one key.
	k := usbdevice.NewUsbKeyboard(ctx)
	k.Type(string(key))

	// Definitions regarding [Human Interface Device Profile 1.1.1], see:
	// 	https://www.bluetooth.com/specifications/specs/human-interface-device-profile-1-1-1/
	const (
		// Message type: DATA, see:
		// 	3.1.1 Bluetooth HID Protocol Message Header
		// 		Table 3.1: Bluetooth HID Protocol Message Type Codes
		messageType = 0x0A

		// Parameter: Input, see:
		// 	3.1.1 Bluetooth HID Protocol Message Header
		// 		Table 3.12: DATA Parameter Definition
		parameter = 0x01

		// The 1 byte Bluetooth HID Protocol Header (HIDP-Hdr),
		// divided into two 4-bit fields: the HIDP Message type and a Parameter, see:
		// 	3.1.1 Bluetooth HID Protocol Message Header
		hidpHeader = messageType<<4 + parameter

		// Report ID: Keyboard, see:
		// 	3.3.2 Bluetooth HID device Boot Protocol Requirements
		// 		Table 3.15: Bluetooth HID Boot Reports
		reportID = 0x01
	)

	// A key event contains 2 reports, key press and release.
	const reportCount = 2
	reports := make([][]int, reportCount)
	for i := 0; i < reportCount; i++ {
		// The `usbdevice` retrieves data from its queue and when the queue is empty, the key release data will be thrown.
		data, err := k.Data()
		if err != nil {
			return err
		}

		// Converting byte slice into integer slice as the RPC `KeyboardSendTrace` only accepts arguments in integers.
		dataPayload := make([]int, len(data))
		for i, b := range data {
			dataPayload[i] = int(b)
		}

		report := append([]int{hidpHeader, reportID}, dataPayload...)
		reports[i] = report
	}

	return c.RPC("KeyboardSendTrace").Args(reports).Call(ctx)
}

// KeyboardSendString calls the Chameleond RPC method of the same name.
// This implements KeyboardPeripheral.KeyboardSendString, see that for more
// details.
func (c *CommonKeyboardPeripheral) KeyboardSendString(ctx context.Context, stringToSend string) error {
	return c.RPC("KeyboardSendString").Args(stringToSend).Call(ctx)
}
