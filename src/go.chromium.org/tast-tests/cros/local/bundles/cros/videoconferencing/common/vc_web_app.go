// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package common this file contains shared logic between extension, pwa and tab.
package common

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
)

var (
	// VcAppName is the name of App used for extension, pwa and tab.
	VcAppName = "VcTester"
	// VcAppURL is the url to access the App used for pwa and tab.
	VcAppURL = "/vc_tester/popup.html"

	rootWebArea                = nodewith.Role(role.RootWebArea).Name(VcAppName)
	startVideoButton           = nodewith.Name("Start Video").Role(role.Button).Ancestor(rootWebArea)
	stopVideoButton            = nodewith.Name("Stop Video").Role(role.Button).Ancestor(rootWebArea)
	startAudioButton           = nodewith.Name("Start Audio").Role(role.Button).Ancestor(rootWebArea)
	stopAudioButton            = nodewith.Name("Stop Audio").Role(role.Button).Ancestor(rootWebArea)
	startScreenCapturingButton = nodewith.Name("Start Screen Capturing").Role(role.Button).Ancestor(rootWebArea)
	stopScreenCapturingButton  = nodewith.Name("Stop Screen Capturing").Role(role.Button).Ancestor(rootWebArea)

	audioPlayButton  = nodewith.Name("play").Role(role.Button).Ancestor(nodewith.Name("HelloAudio"))
	audioPauseButton = nodewith.Name("pause").Role(role.Button).Ancestor(nodewith.Name("HelloAudio"))
)

// VcWebApp defines common actions for extension, pwa and tab.
type VcWebApp struct {
	UI *uiauto.Context
}

// StartVideo clicks on "Start Video" button to activate camera.
func (webApp *VcWebApp) StartVideo(ctx context.Context) error {
	return webApp.UI.RetryUntil(webApp.UI.DoDefault(startVideoButton),
		webApp.UI.WithTimeout(10*time.Second).WaitUntilEnabled(stopVideoButton))(ctx)
}

// StopVideo clicks on "Stop Video" button to deactivate camera.
func (webApp *VcWebApp) StopVideo(ctx context.Context) error {
	return webApp.UI.RetryUntil(webApp.UI.DoDefault(stopVideoButton),
		webApp.UI.WithTimeout(10*time.Second).WaitUntilEnabled(startVideoButton))(ctx)
}

// StartAudio clicks on "Start Audio" button to activate microphone.
func (webApp *VcWebApp) StartAudio(ctx context.Context) error {
	return webApp.UI.RetryUntil(webApp.UI.DoDefault(startAudioButton),
		webApp.UI.WithTimeout(10*time.Second).WaitUntilEnabled(stopAudioButton))(ctx)
}

// StopAudio clicks on "Stop Audio" button to deactivate microphone.
func (webApp *VcWebApp) StopAudio(ctx context.Context) error {
	return webApp.UI.RetryUntil(webApp.UI.DoDefault(stopAudioButton),
		webApp.UI.WithTimeout(10*time.Second).WaitUntilEnabled(startAudioButton))(ctx)
}

// StartScreenCapture clicks on "Start Screen Capturing" button to activate screen sharing.
func (webApp *VcWebApp) StartScreenCapture(ctx context.Context) error {
	return webApp.UI.RetryUntil(webApp.UI.DoDefault(startScreenCapturingButton),
		webApp.UI.WithTimeout(10*time.Second).WaitUntilEnabled(stopScreenCapturingButton))(ctx)
}

// StopScreenCapture clicks on "Stop Screen Capturing" button to deactivate screen sharing.
func (webApp *VcWebApp) StopScreenCapture(ctx context.Context) error {
	return webApp.UI.RetryUntil(webApp.UI.DoDefault(stopScreenCapturingButton),
		webApp.UI.WithTimeout(10*time.Second).WaitUntilEnabled(startScreenCapturingButton))(ctx)
}

// PlayAudio clicks on the audioPlayButton button and wait until the pauseButton to appear.
func (webApp *VcWebApp) PlayAudio(ctx context.Context) error {
	return uiauto.Combine("Play the audio",
		webApp.UI.WaitUntilExists(audioPlayButton),
		webApp.UI.DoDefault(audioPlayButton),
		webApp.UI.WaitUntilExists(audioPauseButton),
	)(ctx)
}

// WaitUntilAllButtonsExists waits for all buttons to exists after launching the app.
func (webApp *VcWebApp) WaitUntilAllButtonsExists(ctx context.Context) error {
	return uiauto.Combine("WaitUntilPageReady",
		webApp.UI.WaitUntilExists(startVideoButton),
		webApp.UI.WaitUntilExists(stopVideoButton),
		webApp.UI.WaitUntilExists(startAudioButton),
		webApp.UI.WaitUntilExists(stopAudioButton),
		webApp.UI.WaitUntilExists(startScreenCapturingButton),
		webApp.UI.WaitUntilExists(stopScreenCapturingButton),
	)(ctx)
}
