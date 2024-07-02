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
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
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
var BluetoothDetailedViewToggleButton = nodewith.HasClass("HoverHighlightView").NameContaining("Toggle Bluetooth").Ancestor(bluetoothDetailedView)

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

// EnsureBluetoothPairingDialogIsOpened returns an action that will navigate to the detailed
// Bluetooth view within the Quick Settings, then launch the Bluetooth Pair New Device Dialog.
// It does nothing if the dialog is launched.
func EnsureBluetoothPairingDialogIsOpened(tconn *chrome.TestConn) uiauto.Action {
	return func(ctx context.Context) (retErr error) {
		ui := uiauto.New(tconn)

		if dialogIsFound, err := ui.IsNodeFound(ctx, BluetoothPairNewDeviceDialog); err != nil {
			return err
		} else if dialogIsFound {
			// No action required if the dialog is already launched.
			return nil
		}

		cleanupCtx := ctx
		ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
		defer cancel()

		if err := NavigateToBluetoothDetailedView(ctx, tconn); err != nil {
			return errors.Wrap(err, "failed to navigate to the detailed Bluetooth view")
		}
		defer Hide(cleanupCtx, tconn)

		if err := ui.WithInterval(time.Second).LeftClickUntil(
			BluetoothDetailedViewPairNewDeviceButton,
			ui.Exists(BluetoothPairNewDeviceDialog),
		)(ctx); err != nil {
			return errors.Wrap(err, "failed to open the pairing dialog")
		}
		return nil
	}
}

// ToggleBluetoothAndSelectKeepOnInBluetoothWarningDialog will click on
// Bluetooth feature pod toggle and selects the "Keep on" option from Bluetooth
// warning dialog. Bluetooth should be powered on before calling this function.
func ToggleBluetoothAndSelectKeepOnInBluetoothWarningDialog(ctx context.Context, tconn *chrome.TestConn) error {
	ui := uiauto.New(tconn).WithTimeout(5 * time.Second)
	if err := ToggleBluetoothFromFeaturePod(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to click Toggle Bluetooth from feature pod./")
	}

	if err := uiauto.Combine("Click on 'Keep on' option in warning dialog",
		ui.WaitUntilExists(BluetoothWarningDialog),
		ui.LeftClickUntil(BluetoothWarningDialogKeepOn, ui.Gone(BluetoothWarningDialog)),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to click the 'Keep on' in warning dialog")
	}
	return nil
}

// ToggleBluetoothAndSelectTurnOffInBluetoothWarningDialog will click on
// Bluetooth feature pod toggle and also select the "Turn Off" option from
// Bluetooth warning dialog. Bluetooth should be powered on before calling this
// function.
func ToggleBluetoothAndSelectTurnOffInBluetoothWarningDialog(ctx context.Context, tconn *chrome.TestConn) error {
	ui := uiauto.New(tconn).WithTimeout(5 * time.Second)
	if err := ToggleBluetoothFromFeaturePod(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to click Toggle Bluetooth from feature pod./")
	}

	if err := uiauto.Combine("Click on 'Turn off' option in warning dialog",
		ui.WaitUntilExists(BluetoothWarningDialog),
		ui.LeftClickUntil(BluetoothWarningDialogTurnOff, ui.Gone(BluetoothWarningDialog)),
	)(ctx); err != nil {
		return errors.Wrap(err, "ailed to click the 'Turn off' in warning dialog")
	}
	return nil
}

// ToggleBluetoothFromFeaturePod will click on Bluetooth feature pod in quick
// settings and toggles Bluetooth state.
func ToggleBluetoothFromFeaturePod(ctx context.Context, tconn *chrome.TestConn) error {
	ui := uiauto.New(tconn).WithTimeout(5 * time.Second)

	if err := Show(ctx, tconn); err != nil {
		return err
	}

	if err := ui.LeftClick(FeatureTileBluetoothToggle)(ctx); err != nil {
		return errors.Wrap(err, "failed to click the Bluetooth feature pod icon button")
	}

	return nil
}
