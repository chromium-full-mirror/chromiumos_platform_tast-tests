// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ossettings

import (
	"context"
	"fmt"
	"regexp"

	"go.chromium.org/tast-tests/cros/local/bluetooth"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
)

// BluetoothSettingsSubpageButton is the arrow button on the OS Settings page
// that a user can click to navigate to the Bluetooth Settings subpage.
var BluetoothSettingsSubpageButton = nodewith.HasClass("subpage-arrow").NameContaining("Bluetooth").Role(role.Button)

// OSSettingsBluetoothToggleButton is the Bluetooth toggle on the OS Settings page.
var OSSettingsBluetoothToggleButton = nodewith.NameContaining("Bluetooth").Role(role.ToggleButton)

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

// BluetoothDeviceItemButtonOnBluetoothSettingsPage returns the button of the Bluetooth
// device item on the OS-Settings Bluetooth Page.
func BluetoothDeviceItemButtonOnBluetoothSettingsPage(name string) *nodewith.Finder {
	// Regular expression for finding a device entry.
	//
	// These are examples of the target label:
	// 	Device 1 of 3 named KEYBD_REF. Device is connected. Device type is unknown. Device has 99% battery.
	// 	Device 2 of 3 named MOUSE_REF. Device is not connected. Device type is unknown.
	// 	Device 3 of 3 named RenamedBT. Device is not connected. Device is a mouse.
	r := regexp.MustCompile(fmt.Sprintf(`^Device \d+ of \d+ named %s\. Device is (connected|not connected)\. Device (type is|is a) \w+\.( Device has (\d+)%% battery\.)?$`, name))
	return nodewith.NameRegex(r).Role(role.Button).HasClass("list-item")
}

// NavigateToBluetoothSettingsSubpage will navigate to the Bluetooth settings
// subpage within the OS Settings.
// NOTE: When the OsSettingsRevampWayfinding feature flag is enabled, the
// Bluetooth settings subpage is automatically up-leveled, in place of the
// top-level Bluetooth page.
// TODO(b/309808834) Once the top-level bluetooth page is revamped, update this
// helper method to navigate to the top-level page instead.
func NavigateToBluetoothSettingsSubpage(ctx context.Context, tconn *chrome.TestConn, bt bluetooth.Bluetooth) (*OSSettings, error) {
	app, err := LaunchAtPage(ctx, tconn, Bluetooth)
	if err != nil {
		return app, err
	}

	if err := bt.Enable(ctx); err != nil {
		return app, err
	}

	return app, nil
}

// NavigateToBluetoothDeviceDetailsPage will navigate to the Bluetooth Device Details
// subpage for the device specified by |deviceName|. This is safe to call when OS Settings
// are already open.
func NavigateToBluetoothDeviceDetailsPage(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn, deviceName string) (*OSSettings, error) {
	condition := uiauto.New(tconn).Exists(nodewith.Name("Bluetooth subpage back button").Ancestor(WindowFinder))
	app, err := LaunchAtPageURL(ctx, tconn, cr, "bluetoothDevices", condition)
	if err != nil {
		return nil, err
	}

	ui := uiauto.New(tconn)
	if err := uiauto.Combine("Focus and click the device's Device Details subpage button",
		ui.FocusAndWait(BluetoothDeviceItemButtonOnBluetoothSettingsPage(deviceName)),
		ui.LeftClick(BluetoothDeviceItemButtonOnBluetoothSettingsPage(deviceName)),
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
		ui.FocusAndWait(BluetoothSettingsSubpageButton),
		ui.LeftClick(BluetoothSettingsSubpageButton),
		ui.LeftClick(SavedDevicesSubpageLink),
	)(ctx); err != nil {
		return nil, err
	}

	return app, nil
}
