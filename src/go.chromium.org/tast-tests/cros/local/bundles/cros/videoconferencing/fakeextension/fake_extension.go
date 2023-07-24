// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package fakeextension contains the library of fake VC extension.
package fakeextension

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/browser/browserui"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/errors"
)

const extensionName = "VcTester"

// ExtensionUI represents the FakeExtension UI.
// It is usually launched by clicking the extension app icon in the browser toolbar.
type ExtensionUI struct {
	tconn *chrome.TestConn
	ui    *uiauto.Context
}

// GrantAVPermissions grants Microphone, Camera permissions to the extension.
func GrantAVPermissions(ctx context.Context, br *browser.Browser) error {
	extID, err := extensionID(ctx, br)
	if err != nil {
		return err
	}

	extURLPattern := fmt.Sprintf("*://%s/*", extID)
	return br.GrantPermissions(ctx, []string{extURLPattern},
		browser.CameraContentSetting,
		browser.MicrophoneContentSetting,
	)
}

var (
	popupRootWindow         = nodewith.Role(role.Window).HasClass("ExtensionPopup")
	vcTesterExtensionButton = nodewith.Role(role.Button).HasClass("ExtensionsMenuButton").Name("VcTester")

	startVideoButton           = nodewith.Name("Start Video").Role(role.Button).Ancestor(popupRootWindow)
	stopVideoButton            = nodewith.Name("Stop Video").Role(role.Button).Ancestor(popupRootWindow)
	startAudioButton           = nodewith.Name("Start Audio").Role(role.Button).Ancestor(popupRootWindow)
	stopAudioButton            = nodewith.Name("Stop Audio").Role(role.Button).Ancestor(popupRootWindow)
	startScreenCapturingButton = nodewith.Name("Start Screen Capturing").Role(role.Button).Ancestor(popupRootWindow)
	stopScreenCapturingButton  = nodewith.Name("Stop Screen Capturing").Role(role.Button).Ancestor(popupRootWindow)

	videoNode      = nodewith.Role(role.Video).Ancestor(popupRootWindow)
	videoIsPlaying = nodewith.HasClass("state-playing").Ancestor(videoNode)
)

// Launch triggers VcTester popup window in browser extensions.
func Launch(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn) (*ExtensionUI, error) {
	ui := uiauto.New(tconn)

	// The extension is already launched if the popup window is found.
	isLaunched, err := ui.IsNodeFound(ctx, popupRootWindow)
	if err != nil {
		return nil, errors.Wrap(err, "failed to check popup window")
	} else if !isLaunched {
		if err := uiauto.Combine("trigger popup",
			ui.DoDefaultUntil(
				browserui.ExtensionsToolbarButton,
				ui.WithTimeout(5*time.Second).WaitUntilExists(vcTesterExtensionButton),
			),
			ui.DoDefaultUntil(vcTesterExtensionButton, ui.WaitUntilExists(popupRootWindow)),
		)(ctx); err != nil {
			return nil, errors.Wrap(err, "failed to launch extension")
		}
	}

	return &ExtensionUI{tconn, ui}, nil
}

// StartVideo clicks on "Start Video" button to activate camera.
func (extUI *ExtensionUI) StartVideo(ctx context.Context) error {
	return extUI.ui.DoDefaultUntil(
		startVideoButton,
		extUI.ui.WithTimeout(3*time.Second).WaitUntilExists(videoIsPlaying),
	)(ctx)
}

// StopVideo clicks on "Stop Video" button to deactivate camera.
func (extUI *ExtensionUI) StopVideo(ctx context.Context) error {
	return extUI.ui.DoDefault(stopVideoButton)(ctx)
}

// StartAudio clicks on "Start Audio" button to activate microphone.
func (extUI *ExtensionUI) StartAudio(ctx context.Context) error {
	return extUI.ui.DoDefault(startAudioButton)(ctx)
}

// StopAudio clicks on "Stop Audio" button to deactivate microphone.
func (extUI *ExtensionUI) StopAudio(ctx context.Context) error {
	return extUI.ui.DoDefault(stopAudioButton)(ctx)
}

// StartScreenCapture clicks on "Start Screen Capturing" button to activate screen sharing.
func (extUI *ExtensionUI) StartScreenCapture(ctx context.Context) error {
	// There may be multiple "Choose what to share" dialogs, so add First() here.
	chooseWhatToShareWindow := nodewith.Role(role.Dialog).NameContaining("Choose what to share").HasClass("Widget").First()
	entireScreenTab := nodewith.Role(role.Tab).Name("Entire Screen")
	display := nodewith.Role(role.Button).HasClass("DesktopMediaSourceView").Ancestor(chooseWhatToShareWindow).First()
	shareButton := nodewith.Name("Share").Role(role.Button).Ancestor(chooseWhatToShareWindow)
	return uiauto.Combine("share entire screen",
		extUI.ui.DoDefaultUntil(
			startScreenCapturingButton,
			extUI.ui.WithTimeout(3*time.Second).WaitUntilExists(chooseWhatToShareWindow),
		),
		extUI.ui.DoDefault(entireScreenTab),
		extUI.ui.DoDefault(display),
		extUI.ui.DoDefault(shareButton),
	)(ctx)
}

// StopScreenCapture clicks on "Stop Screen Capturing" button to deactivate screen sharing.
func (extUI *ExtensionUI) StopScreenCapture(ctx context.Context) error {
	return extUI.ui.DoDefault(stopScreenCapturingButton)(ctx)
}

// extensionID returns the extension id of the fake vc extension.
func extensionID(ctx context.Context, br *browser.Browser) (string, error) {
	bTconn, err := br.TestAPIConn(ctx)
	if err != nil {
		return "", errors.Wrap(err, "failed to connect to browser test API connection")
	}

	installApps, err := ash.ExtensionApps(ctx, bTconn)
	for _, app := range installApps {
		if app.Name == extensionName {
			return app.ID, nil
		}
	}
	return "", errors.Errorf("%q extension is not installed", extensionName)
}
