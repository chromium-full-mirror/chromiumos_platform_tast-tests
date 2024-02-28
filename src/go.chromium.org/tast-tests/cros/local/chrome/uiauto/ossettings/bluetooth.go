// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ossettings

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/bluetooth"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
)

// osBluetoothSettingsButton is the arrow button on the OS Settings that a user
// can click to navigate to the Bluetooth Settings.
var osBluetoothSettingsButton = nodewith.HasClass("subpage-arrow").NameContaining("Bluetooth").Role(role.Button)

// OsSettingsBluetoothToggleButton is the Bluetooth toggle on the OS Settings page.
var OsSettingsBluetoothToggleButton = nodewith.NameContaining("Bluetooth").Role(role.ToggleButton)

// BluetoothPairNewDeviceButton is the "pair new device" button within the OS Settings and Bluetooth Settings.
var BluetoothPairNewDeviceButton = nodewith.NameContaining("Pair new device").Role(role.Button)

// BluetoothPairNewDeviceModal is the modal that is opened when the "pair new
// device" button within either the OS Settings or Bluetooth Settings is pressed.
var BluetoothPairNewDeviceModal = nodewith.NameContaining("Pair new device").Role(role.Heading)

// BluetoothConnectedDeviceRows are the list items in the "Connected devices" list on the Bluetooth Settings page.
var BluetoothConnectedDeviceRows = nodewith.NameContaining("connected").HasClass("list-item").Role(role.Button)

// BluetoothForgetDeviceButton is the "Forget" button on the Bluetooth Device Details subpage.
var BluetoothForgetDeviceButton = nodewith.NameContaining("Forget").HasClass("cancel-button").Role(role.Button)

// BluetoothConfirmForgetButton is the confirmation button on the "Forget device" modal that is opened
// after clicking BluetoothForgetDeviceButton.
var BluetoothConfirmForgetButton = nodewith.NameContaining("Forget").HasClass("action-button").Role(role.Button)

// NavigateToBluetoothSettingsPage will navigate to the Bluetooth sub-page
// within the OS Settings by clicking the sub-page button. This is safe to call
// when the OS Settings are already open.
func NavigateToBluetoothSettingsPage(ctx context.Context, tconn *chrome.TestConn, bt bluetooth.Bluetooth) (*OSSettings, error) {
	app, err := LaunchAtPage(ctx, tconn, Bluetooth)
	if err != nil {
		return app, err
	}

	if err := bt.Enable(ctx); err != nil {
		return app, err
	}

	ui := uiauto.New(tconn)

	if err := uiauto.Combine("Focus and click the Bluetooth Settings button",
		ui.FocusAndWait(osBluetoothSettingsButton),
		ui.LeftClickUntil(osBluetoothSettingsButton, ui.Gone(osBluetoothSettingsButton)),
	)(ctx); err != nil {
		return app, err
	}

	return app, nil
}

// NavigateToBluetoothDeviceDetailsPage will navigate to the Bluetooth Device Details
// subpage for the device specified by |deviceName|. This is safe to call when OS Settings
// are already open.
func NavigateToBluetoothDeviceDetailsPage(ctx context.Context, tconn *chrome.TestConn, deviceName string) (*OSSettings, error) {
	app, err := Launch(ctx, tconn)
	if err != nil {
		return nil, err
	}

	ui := uiauto.New(tconn)

	var connectedDevice = BluetoothConnectedDeviceRows.NameContaining(deviceName)

	if err := uiauto.Combine("Focus and click the Bluetooth Settings button and the Connected device's Device Details subpage button",
		ui.FocusAndWait(osBluetoothSettingsButton),
		ui.LeftClick(osBluetoothSettingsButton),
		ui.FocusAndWait(connectedDevice),
		ui.LeftClick(connectedDevice),
	)(ctx); err != nil {
		return nil, err
	}

	return app, nil
}

// NavigateToBluetoothSavedDevicesSubpage will navigate to the Bluetooth Saved Devices subpage
// within the OS Settings by clicking the subpage button on the Bluetooth Settings subpage.
// This is safe to call when the OS Settings are already open.
func NavigateToBluetoothSavedDevicesSubpage(ctx context.Context, tconn *chrome.TestConn, cr *chrome.Chrome) (*OSSettings, error) {
	app, err := Launch(ctx, tconn)
	if err != nil {
		return nil, err
	}

	ui := uiauto.New(tconn)

	if err := uiauto.Combine("Navigate to the Bluetooth Saved Devices subpage",
		ui.FocusAndWait(osBluetoothSettingsButton),
		ui.LeftClick(osBluetoothSettingsButton),
		ui.LeftClick(SavedDevicesSubpageLink),
	)(ctx); err != nil {
		return nil, err
	}

	return app, nil
}
