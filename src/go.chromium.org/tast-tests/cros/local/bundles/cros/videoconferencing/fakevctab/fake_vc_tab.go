// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package fakevctab contains the library of fake VC tab.
package fakevctab

import (
	"context"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
)

// VcTabUI represents the Fake VC Tab UI.
// It is usually launched by browersing to fake html.
type VcTabUI struct {
	tconn *chrome.TestConn
	ui    *uiauto.Context
	tab   *browser.Tab
}

var (
	vcTabName   = "VcTester"
	rootWebArea = nodewith.Role(role.RootWebArea).Name(vcTabName)

	startVideoButton           = nodewith.Name("Start Video").Role(role.Button).Ancestor(rootWebArea)
	stopVideoButton            = nodewith.Name("Stop Video").Role(role.Button).Ancestor(rootWebArea)
	startAudioButton           = nodewith.Name("Start Audio").Role(role.Button).Ancestor(rootWebArea)
	stopAudioButton            = nodewith.Name("Stop Audio").Role(role.Button).Ancestor(rootWebArea)
	startScreenCapturingButton = nodewith.Name("Start Screen Capturing").Role(role.Button).Ancestor(rootWebArea)
	stopScreenCapturingButton  = nodewith.Name("Stop Screen Capturing").Role(role.Button).Ancestor(rootWebArea)

	videoNode = nodewith.Role(role.Video).Ancestor(rootWebArea)
)

// LaunchTab opens a new tab for the url.
func LaunchTab(ctx context.Context, tconn *browser.TestConn, br *browser.Browser, url string) (*VcTabUI, error) {
	if _, err := br.NewTab(ctx, url); err != nil {
		return nil, err
	}

	tab, err := browser.GetTabByTitle(ctx, tconn, vcTabName)
	if err != nil {
		return nil, err
	}

	return &VcTabUI{tconn, uiauto.New(tconn), tab}, nil
}

// CloseTab close the tab with id inside VcTabUI.
func (extUI *VcTabUI) CloseTab(ctx context.Context) error {
	return browser.CloseTabsByID(ctx, extUI.tconn, []int{extUI.tab.ID})
}

// StartVideo clicks on "Start Video" button to activate camera.
func (extUI *VcTabUI) StartVideo(ctx context.Context) error {
	return extUI.ui.DoDefault(startVideoButton)(ctx)
}

// StopVideo clicks on "Stop Video" button to deactivate camera.
func (extUI *VcTabUI) StopVideo(ctx context.Context) error {
	return extUI.ui.DoDefault(stopVideoButton)(ctx)
}

// StartAudio clicks on "Start Audio" button to activate microphone.
func (extUI *VcTabUI) StartAudio(ctx context.Context) error {
	return uiauto.Combine("start audio",
		extUI.ui.DoDefault(startAudioButton),
		// Add a short sleep after starting audio. Refer to b/298280544 for more details.
		uiauto.Sleep(100*time.Millisecond),
	)(ctx)
}

// StopAudio clicks on "Stop Audio" button to deactivate microphone.
func (extUI *VcTabUI) StopAudio(ctx context.Context) error {
	return extUI.ui.DoDefault(stopAudioButton)(ctx)
}

// StartScreenCapture clicks on "Start Screen Capturing" button to activate screen sharing.
func (extUI *VcTabUI) StartScreenCapture(ctx context.Context) error {
	// There may be multiple "Choose what to share" dialogs, so add First() here.
	chooseWhatToShareWindow := nodewith.Role(role.Dialog).NameContaining("Choose what to share").HasClass("Widget").First()
	entireScreenTab := nodewith.Role(role.Tab).NameRegex(regexp.MustCompile("(?i)Entire Screen")).Ancestor(chooseWhatToShareWindow)
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
func (extUI *VcTabUI) StopScreenCapture(ctx context.Context) error {
	return extUI.ui.DoDefault(stopScreenCapturingButton)(ctx)
}
