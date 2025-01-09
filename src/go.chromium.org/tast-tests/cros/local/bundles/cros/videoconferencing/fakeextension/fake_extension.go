// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package fakeextension contains the library of fake VC extension.
package fakeextension

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/videoconferencing/common"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/browser/browserui"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"

	"go.chromium.org/tast/core/errors"
)

// ExtensionUI represents the FakeExtension UI.
// It is usually launched by clicking the extension app icon in the browser toolbar.
type ExtensionUI struct {
	common.VcWebApp
}

// GrantAVPermissions grants Microphone, Camera permissions to the extension.
func GrantAVPermissions(ctx context.Context, tconn *chrome.TestConn) error {
	extID, err := extensionID(ctx, tconn)
	if err != nil {
		return err
	}

	extURLPattern := fmt.Sprintf("*://%s/*", extID)
	return browser.GrantPermissions(ctx, tconn, []string{extURLPattern},
		browser.CameraContentSetting,
		browser.MicrophoneContentSetting,
	)
}

var (
	vcTesterExtensionButton = nodewith.Role(role.Button).HasClass("ExtensionsMenuButton").Name(common.VcAppName)
)

// Launch triggers VcTester popup window in browser extensions.
func Launch(ctx context.Context, tconn *chrome.TestConn, cr *chrome.Chrome) (*ExtensionUI, error) {
	if _, err := cr.NewConn(ctx, chrome.NewTabURL); err != nil {
		return nil, err
	}

	ui := uiauto.New(tconn)
	extUI := ExtensionUI{common.VcWebApp{UI: ui}}

	// The extension is already launched if the popup window is found.
	if err := uiauto.Combine("trigger popup",
		ui.DoDefaultUntil(
			browserui.ExtensionsToolbarButton,
			ui.WithTimeout(5*time.Second).WaitUntilExists(vcTesterExtensionButton),
		),
		ui.DoDefault(vcTesterExtensionButton),
		extUI.WaitUntilAllButtonsExists,
	)(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to launch extension")
	}

	return &extUI, nil
}

// extensionID returns the extension id of the fake vc extension.
func extensionID(ctx context.Context, tconn *chrome.TestConn) (string, error) {
	installApps, _ := ash.ExtensionApps(ctx, tconn)
	for _, app := range installApps {
		if app.Name == common.VcAppName {
			return app.ID, nil
		}
	}
	return "", errors.Errorf("%q extension is not installed", common.VcAppName)
}
