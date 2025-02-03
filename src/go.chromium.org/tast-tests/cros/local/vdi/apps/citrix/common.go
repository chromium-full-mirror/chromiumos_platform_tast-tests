// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package citrix

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/uidetection"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

var topGroup = nodewith.HasClass("halfToolbarIconTop").Role(role.Group)

// ShowDesktop shows desktop.
func ShowDesktop(ud *uidetection.Context, dataPath func(string) string, tconn *chrome.TestConn) uiauto.Action {
	startBtn := uidetection.CustomIcon(dataPath(startBtnIcon))
	shutText := uidetection.Word("Shut").First()
	desktopText := uidetection.Word("Desktop").Below(shutText).First()
	return uiauto.Retry(3,
		uiauto.NamedCombine("show desktop",
			ud.RightClick(startBtn),
			ud.LeftClick(desktopText),
			WaitForDesktop(tconn),
		))
}

// EnterDesktop enters the Citrix desktop.
func EnterDesktop(tconn *chrome.TestConn) uiauto.Action {
	ui := uiauto.New(tconn)
	desktopText := nodewith.Name("Desktop").Role(role.StaticText).First()
	launchingDesktopText := nodewith.Name("Launching desktop...").Role(role.StaticText).First()
	return uiauto.NamedCombine("enter desktop",
		ui.WaitUntilAnyExists(desktopText, launchingDesktopText),
		uiauto.IfSuccessThen(ui.Exists(desktopText), ui.DoDefault(desktopText)),
		ui.WaitUntilExists(launchingDesktopText),
		ui.WaitUntilGone(launchingDesktopText),
		WaitForDesktop(tconn),
	)
}

// WaitForDesktop waits for desktop to be visible.
func WaitForDesktop(tconn *chrome.TestConn) uiauto.Action {
	ui := uiauto.New(tconn)
	return uiauto.NamedAction("wait for desktop",
		ui.WithTimeout(time.Minute).WaitUntilExists(topGroup),
	)
}

// LogOff logs off from Citrix desktop.
func LogOff(tconn *chrome.TestConn) uiauto.Action {
	moreOptionBtn := nodewith.HasClass("more").First()
	logOffText := nodewith.Name("Log Off").Role(role.StaticText).First()
	ui := uiauto.New(tconn)
	return uiauto.NamedCombine("log off from Citrix desktop",
		ui.LeftClick(topGroup),
		ui.LeftClick(moreOptionBtn),
		ui.LeftClick(logOffText),
	)
}

// ConnectUSBDevice connects the USB device to the DUT.
func ConnectUSBDevice(tconn *chrome.TestConn) uiauto.Action {
	ui := uiauto.New(tconn)
	usbBtn := nodewith.HasClass("usb").First()
	usbDeviceText := nodewith.Name("USB Devices").Role(role.StaticText)
	connectCheckBox := nodewith.Name("Connect").Role(role.CheckBox).Focusable().First()
	closeButton := nodewith.Name("Close button").HasClass("closeBtn")
	return uiauto.NamedCombine("connect USB device",
		ui.LeftClick(topGroup),
		ui.LeftClick(usbBtn),
		ui.WaitUntilExists(usbDeviceText),
		ui.DoDefaultUntil(connectCheckBox, ui.WithTimeout(time.Second).WaitUntilGone(connectCheckBox)),
		ui.LeftClick(closeButton),
	)
}

// OpenApp opens the application with given name in Citrix.
func OpenApp(ud *uidetection.Context, dataPath func(string) string, appName, appTitle string) uiauto.Action {
	appText := uidetection.Word(appName).First()
	return uiauto.NamedAction("open app: "+appName,
		openApp(ud, dataPath, appText, appTitle),
	)
}

// OpenAppByIcon opens the application with given icon in Citrix.
func OpenAppByIcon(ud *uidetection.Context, dataPath func(string) string, appIcon, appTitle string) uiauto.Action {
	icon := uidetection.CustomIcon(dataPath(appIcon))
	return uiauto.NamedAction("open app by icon: "+appIcon,
		openApp(ud, dataPath, icon, appTitle),
	)
}

// openApp opens the application with given finder in Citrix.
func openApp(ud *uidetection.Context, dataPath func(string) string, finder *uidetection.Finder, appTitle string) uiauto.Action {
	topBtn := uidetection.CustomIcon(dataPath(topBtnIcon))
	appTitleText := uidetection.TextBlockFromSentence(appTitle).Below(topBtn).First()
	return uiauto.Retry(3, uiauto.Combine("open app",
		ud.WithTimeout(15*time.Second).DoubleClick(finder),
		ud.WaitUntilExists(appTitleText),
	))
}

// CloseApp closes the application with given name in Citrix.
func CloseApp(ctx context.Context, tconn *chrome.TestConn, kb *input.KeyboardEventWriter, ud *uidetection.Context, appTitle string) error {
	appTitleText := uidetection.TextBlockFromSentence(appTitle).First()
	if err := uiauto.Retry(3, ud.WithTimeout(10*time.Second).LeftClick(appTitleText))(ctx); err != nil {
		// Do not return an error if it fails.
		// It still requires trying to close the app window by pressing the keyboard.
		testing.ContextLog(ctx, "Failed to click on app title: ", err)
	}

	// Not all Chromebooks have the same layout for the function keys.
	topRow, err := input.KeyboardTopRowLayout(ctx, kb)
	if err != nil {
		return errors.Wrap(err, "failed to obtain the top-row layout")
	}

	// Press 'Alt+F4' to close app window.
	closeAppWindowKey := "Alt+F4"
	if topRow.ZoomToggle != "F4" {
		closeAppWindowKey = "Alt+" + topRow.SelectTask
	}

	if err := uiauto.NamedAction("close app",
		uiauto.New(tconn).WithTimeout(time.Minute).RetryUntil(
			kb.AccelAction(closeAppWindowKey),
			ud.WithTimeout(5*time.Second).WaitUntilGone(appTitleText),
		))(ctx); err != nil {
		return errors.Wrap(err, "failed to close app")
	}
	return nil
}

// DeleteFile deletes the file with given name.
func DeleteFile(ud *uidetection.Context, dataPath func(string) string, fileName string) uiauto.Action {
	fileNameText := uidetection.Word(fileName).First()
	deleteText := uidetection.Word("Delete").First()
	return uiauto.Retry(3,
		uiauto.NamedCombine(fmt.Sprintf("delete file %q", fileName),
			ud.WithTimeout(15*time.Second).RightClick(fileNameText),
			ud.WithTimeout(15*time.Second).LeftClick(deleteText),
		))
}

// DeleteFileIfExists deletes the file with given name if it exists.
func DeleteFileIfExists(ud *uidetection.Context, dataPath func(string) string, fileName string) uiauto.Action {
	fileNameText := uidetection.Word(fileName).First()
	return uiauto.NamedAction(fmt.Sprintf("delete file %q if exists", fileName),
		uiauto.IfSuccessThen(ud.Exists(fileNameText), DeleteFile(ud, dataPath, fileName)),
	)
}
