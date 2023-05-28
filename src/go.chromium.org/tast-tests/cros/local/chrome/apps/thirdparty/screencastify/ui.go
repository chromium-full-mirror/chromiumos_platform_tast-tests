// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package screencastify

import (
	"context"
	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/browser/browserui"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"time"

	"go.chromium.org/tast/core/errors"
)

// Screencastify represents the Screencastify UI.
// It is usually launched by clicking the extension app icon in the browser toolbar.
type Screencastify struct {
	tconn *chrome.TestConn
	ui    *uiauto.Context
}

var screencastifyExtensionButton = nodewith.Role(role.Button).HasClass("ExtensionsMenuButton").NameContaining("Screencastify")
var screencastifyPopupWindow = nodewith.HasClass("recorder").Ancestor(browserui.ExtensionPopupRootView).First()

// Elements on extension popup page.
var microphoneSwitchButton = nodewith.Role(role.CheckBox).Name("Microphone").Ancestor(screencastifyPopupWindow)
var settingsVoicePopupButton = nodewith.Role(role.PopUpButton).Name("settings_voice").Ancestor(screencastifyPopupWindow)
var cameraSwitchButton = nodewith.Role(role.CheckBox).Name("Embed webcam").Ancestor(screencastifyPopupWindow)
var cameraPopupButton = nodewith.Role(role.PopUpButton).NameContaining("Camera").Ancestor(screencastifyPopupWindow)
var recordButton = nodewith.Role(role.Button).Name("Record").Ancestor(screencastifyPopupWindow)

// Elements on recording page.
var recordingRootView = nodewith.Role(role.RootWebArea).NameContaining("Preview")

// Launch triggers Screencastify popup window in browser extensions.
func Launch(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn) (*Screencastify, error) {
	ui := uiauto.New(tconn)

	// The extension is already launched if the popup window is found.
	isLaunched, err := ui.IsNodeFound(ctx, screencastifyPopupWindow)
	if err != nil {
		return nil, errors.Wrap(err, "failed to check popup window")
	} else if !isLaunched {
		if err := uiauto.Combine("launch Screencastify",
			ui.DoDefaultUntil(
				browserui.ExtensionsToolbarButton,
				ui.WithTimeout(5*time.Second).WaitUntilExists(screencastifyExtensionButton),
			),
			ui.DoDefaultUntil(screencastifyExtensionButton, ui.WaitUntilExists(screencastifyPopupWindow)),
		)(ctx); err != nil {
			return nil, errors.Wrap(err, "failed to launch Screencastify extension")
		}
	}

	return &Screencastify{tconn, ui}, nil
}

// ToggleMicrophone clicks Microphone toggle button to turn on/off audio on Screencastify extension popup window.
func (sc *Screencastify) ToggleMicrophone(enabled bool) action.Action {
	return func(ctx context.Context) error {
		if err := sc.ui.WaitUntilExists(microphoneSwitchButton)(ctx); err != nil {
			return errors.Wrap(err, "microphone button is not shown, perhaps page rendering failed")
		}

		// settingsVoicePopupButton is only shown when microphone is enabled.
		audioEnabled, err := sc.ui.IsNodeFound(ctx, settingsVoicePopupButton)
		if err != nil {
			return errors.Wrap(err, "failed to check audio setting node")
		}
		// Mic status is already as expected. No action required.
		if (audioEnabled && enabled) || (!audioEnabled && !enabled) {
			return nil
		}

		if err := sc.ui.DoDefault(microphoneSwitchButton)(ctx); err != nil {
			return errors.Wrap(err, "failed to switch microphone")
		}

		if enabled {
			return sc.ui.WaitUntilExists(settingsVoicePopupButton)(ctx)
		}
		return sc.ui.WaitUntilGone(settingsVoicePopupButton)(ctx)
	}
}

// ToggleCamera clicks Camera toggle button to turn on/off camera on Screencastify extension popup window.
func (sc *Screencastify) ToggleCamera(enabled bool) action.Action {
	return func(ctx context.Context) error {
		if err := sc.ui.WaitUntilExists(cameraSwitchButton)(ctx); err != nil {
			return errors.Wrap(err, "camera button is not shown, perhaps page rendering failed")
		}

		// cameraPopupButton is only shown when Camera is enabled.
		camEnabled, err := sc.ui.IsNodeFound(ctx, cameraPopupButton)
		if err != nil {
			return errors.Wrap(err, "failed to check camera setting node")
		}
		// Camera status is already as expected. No action required.
		if (camEnabled && enabled) || (!camEnabled && !enabled) {
			return nil
		}

		if err := sc.ui.DoDefault(cameraSwitchButton)(ctx); err != nil {
			return errors.Wrap(err, "failed to switch Camera")
		}

		if enabled {
			return sc.ui.WaitUntilExists(cameraPopupButton)(ctx)
		}
		return sc.ui.WaitUntilGone(cameraPopupButton)(ctx)
	}
}

// RecordType represents for the type of record.
type RecordType string

// Available options of record type.
const (
	RecordTypeCameraOnly RecordType = "Webcam Only"
	RecordTypeDesktop               = "Desktop"
	RecordTypeBrowserTab            = "Browser Tab"
)

// SetRecordType selects record type. 3 options are available as defined in RecordType.
func (sc *Screencastify) SetRecordType(recordType RecordType) action.Action {
	button := nodewith.Role(role.Tab).NameContaining(string(recordType)).Ancestor(screencastifyPopupWindow)
	return sc.ui.LeftClick(button)
}

// StartRecording clicks "Record" button to start recording.
func (sc *Screencastify) StartRecording(ctx context.Context) error {
	return sc.ui.DoDefault(recordButton)(ctx)
}

// WaitUntilCameraRecordingStarted waits until camera record started.
func (sc *Screencastify) WaitUntilCameraRecordingStarted(ctx context.Context) error {
	node := nodewith.Role(role.Video).HasClass("camera")
	return sc.ui.WaitUntilExists(node)(ctx)
}

// StopRecordingOnPreviewPage clicks "STOP" button in recording page to stop recording.
func (sc *Screencastify) StopRecordingOnPreviewPage(ctx context.Context) error {
	stopRecordingButton := nodewith.Role(role.Button).NameContaining("STOP").Ancestor(recordingRootView)
	return sc.ui.DoDefault(stopRecordingButton)(ctx)
}

// StopRecordingOnPopupWindow clicks "STOP" button in exntension popup window.
func (sc *Screencastify) StopRecordingOnPopupWindow(ctx context.Context) error {
	stopButton := nodewith.Name("stop").Role(role.Button).Ancestor(screencastifyPopupWindow)
	return sc.ui.DoDefault(stopButton)(ctx)
}

// ShareEntireScreen shares the entire screen with AV options.
// It assumes the user is on Screencastify extension popup window.
func (sc *Screencastify) ShareEntireScreen(micEnabled, camEnabled bool) action.Action {
	chooseWhatToShareWindow := nodewith.Role(role.Dialog).Name("Choose what to share").HasClass("Widget")
	display := nodewith.Role(role.Button).HasClass("DesktopMediaSourceView").Ancestor(chooseWhatToShareWindow).First()
	shareButton := nodewith.Name("Share").Role(role.Button).Ancestor(chooseWhatToShareWindow)
	return uiauto.Combine("share entire screen",
		sc.SetRecordType(RecordTypeDesktop),
		sc.ToggleMicrophone(micEnabled),
		sc.ToggleCamera(camEnabled),
		sc.StartRecording,
		sc.ui.DoDefault(display),
		sc.ui.DoDefault(shareButton),
	)
}
