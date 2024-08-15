// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package citrix

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/exp/slices"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/uidetection"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// WaitForDesktop waits for desktop to be visible.
func WaitForDesktop(ud *uidetection.Context, dataPath func(string) string) uiauto.Action {
	topBtn := uidetection.CustomIcon(dataPath(topBtnIcon))
	return uiauto.NamedAction("wait for desktop",
		ud.WaitUntilExists(topBtn),
	)
}

// OpenApp opens the application with given name in Citrix.
func OpenApp(ud *uidetection.Context, dataPath func(string) string, appName, appTitle string) uiauto.Action {
	topBtn := uidetection.CustomIcon(dataPath(topBtnIcon))
	appText := uidetection.Word(appName).Below(topBtn).First()
	appTitleText := uidetection.TextBlockFromSentence(appTitle).Below(topBtn).First()
	return uiauto.Retry(3, uiauto.NamedCombine("open app: "+appName,
		ud.DoubleClick(appText),
		ud.WaitUntilExists(appTitleText),
	))
}

// ConnectUSBDevice connects the USB device to the DUT.
func ConnectUSBDevice(kb *input.KeyboardEventWriter, ud *uidetection.Context, dataPath func(string) string, deviceName string) uiauto.Action {
	topBtn := uidetection.CustomIcon(dataPath(topBtnIcon))
	deviceBtn := uidetection.CustomIcon(dataPath(usbDeviceBtnIcon))
	closeBtn := uidetection.CustomIcon(dataPath(closeDialogBtnIcon))
	usbDevicesText := uidetection.TextBlockFromSentence("USB Devices").First()
	deviceNameText := uidetection.TextBlockFromSentence(deviceName).First()
	availableText := uidetection.Word("Available").First()
	connectText := uidetection.Word("Connect").First()
	scrollDown := uiauto.NamedCombine("scroll down",
		ud.LeftClick(deviceNameText),
		uiauto.Repeat(3, kb.AccelAction("Down")),
	)
	connect := uiauto.NamedAction("connect",
		ud.LeftClickUntil(connectText, ud.WithTimeout(5*time.Second).WaitUntilGone(connectText)),
	)

	return uiauto.NamedCombine("connect USB device",
		ud.LeftClick(topBtn),
		ud.LeftClick(deviceBtn),
		ud.WaitUntilExists(usbDevicesText),
		uiauto.IfSuccessThen(ud.Gone(availableText), scrollDown),
		uiauto.IfSuccessThen(ud.Exists(availableText), connect),
		ud.LeftClick(closeBtn),
	)
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

// SwitchResolution switches the display resolution to the given option.
func SwitchResolution(ud *uidetection.Context, resolutionOption citrixResolution, dataPath func(string) string) uiauto.Action {
	topBtn := uidetection.CustomIcon(dataPath(topBtnIcon))
	moreOptionBtn := uidetection.CustomIcon(dataPath(moreOptionBtnIcon))
	closeBtn := uidetection.CustomIcon(dataPath(closeDialogBtnIcon))
	displayResolutionText := uidetection.TextBlockFromSentence("Display Resolution")
	// Narrow down the resolution checkbox range.
	// Default to use "Resolution" text as the upper bound.
	previousOptionText := uidetection.Word("Resolution")
	optionOrder := slices.Index(citrixResolutionSlice, resolutionOption)
	// Set the previous resolution option as upper bound .
	if optionOrder != 0 {
		previousOption := string(citrixResolutionSlice[optionOrder-1])
		previousOptionText = uidetection.TextBlockFromSentence(previousOption)
	}
	resolutionCheckBox := uidetection.CustomIcon(dataPath(resolutionButtonIcon)).Below(previousOptionText)
	// Set the next resolution option as lower bound.
	if optionOrder != len(citrixResolutionSlice)-1 {
		nextResolution := citrixResolutionSlice[optionOrder+1]
		nextResolutionOptionText := uidetection.TextBlockFromSentence(string(nextResolution))
		resolutionCheckBox = resolutionCheckBox.Above(nextResolutionOptionText)
	}
	return uiauto.NamedCombine("switch resolution as "+string(resolutionOption),
		ud.LeftClick(topBtn),
		ud.LeftClick(moreOptionBtn),
		ud.LeftClick(displayResolutionText),
		ud.WaitUntilGone(displayResolutionText),
		ud.LeftClick(resolutionCheckBox),
		ud.LeftClick(closeBtn),
	)
}

// DeleteFile deletes the file with given name.
func DeleteFile(ud *uidetection.Context, fileName string) uiauto.Action {
	fileNameText := uidetection.Word(fileName).First()
	deleteText := uidetection.Word("Delete").First()
	return uiauto.NamedCombine(fmt.Sprintf("delete file %q", fileName),
		ud.RightClick(fileNameText),
		ud.LeftClick(deleteText),
	)
}

// DeleteFileIfExists deletes the file with given name if it exists.
func DeleteFileIfExists(ud *uidetection.Context, fileName string) uiauto.Action {
	fileNameText := uidetection.Word(fileName).First()
	return uiauto.NamedAction(fmt.Sprintf("delete file %q if exists", fileName),
		uiauto.IfSuccessThen(ud.Exists(fileNameText), DeleteFile(ud, fileName)),
	)
}
