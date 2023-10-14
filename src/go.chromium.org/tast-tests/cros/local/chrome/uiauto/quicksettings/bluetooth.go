// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package quicksettings

import (
	"context"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/testing"
)

// bluetoothDetailedView is the detailed Bluetooth view within the Quick
// Settings.
var bluetoothDetailedView = nodewith.ClassNameRegex(regexp.MustCompile(`^BluetoothDetailedView[A-Za-z]*$`))

// BluetoothDetailedViewPairNewDeviceButton is the "Pair new device" button
// child within the detailed Bluetooth view.
var BluetoothDetailedViewPairNewDeviceButton = nodewith.Role(role.Button).NameContaining("Pair new device").Ancestor(bluetoothDetailedView)

// BluetoothPairNewDeviceDialog is the "Pair new device" dialog opened when
// BluetoothDetailedViewPairNewDeviceButton is clicked.
var BluetoothPairNewDeviceDialog = nodewith.NameContaining("Pair new device").Role(role.RootWebArea)

// BluetoothDetailedViewSettingsButton is the Settings button child within the
// detailed Bluetooth view.
var BluetoothDetailedViewSettingsButton = nodewith.HasClass("IconButton").NameContaining("Bluetooth settings").Ancestor(bluetoothDetailedView)

// BluetoothDetailedViewToggleButton is the Bluetooth toggle child within the
// detailed Bluetooth view.
var BluetoothDetailedViewToggleButton = nodewith.Role(role.Button).NameContaining("Toggle Bluetooth").Ancestor(bluetoothDetailedView)

// NavigateToBluetoothDetailedView will navigate to the detailed Bluetooth view
// within the Quick Settings. This is safe to call even when the Quick Settings
// are already open.
func NavigateToBluetoothDetailedView(ctx context.Context, tconn *chrome.TestConn) error {
	ui := uiauto.New(tconn).WithTimeout(5 * time.Second)

	// The Quick Settings may be auto-collapsed after being expanded due to notifications or other
	// events so we continue attempting to navigate to the Bluetooth page for up to one minute.
	return testing.Poll(ctx, func(ctx context.Context) error {
		if err := Show(ctx, tconn); err != nil {
			return err
		}
		return uiauto.Combine("Click the Bluetooth feature tile",
			ui.LeftClick(FeatureTileBluetooth),
			ui.WaitUntilExists(bluetoothDetailedView),
		)(ctx)
	}, &testing.PollOptions{Timeout: time.Minute, Interval: 5 * time.Second})
}
