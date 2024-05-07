// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package fakevctab contains the library of fake VC tab.
package fakevctab

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/testing"
)

// VcTabUI represents the Fake VC Tab UI.
// It is usually launched by browersing to fake html.
type VcTabUI struct {
	ui  *uiauto.Context
	br  *browser.Browser
	url string
}

var (
	vcTabName   = "VcTester"
	vcTabURL    = "/vc_tester/popup.html"
	rootWebArea = nodewith.Role(role.RootWebArea).Name(vcTabName)

	startVideoButton           = nodewith.Name("Start Video").Role(role.Button).Ancestor(rootWebArea)
	stopVideoButton            = nodewith.Name("Stop Video").Role(role.Button).Ancestor(rootWebArea)
	startAudioButton           = nodewith.Name("Start Audio").Role(role.Button).Ancestor(rootWebArea)
	stopAudioButton            = nodewith.Name("Stop Audio").Role(role.Button).Ancestor(rootWebArea)
	startScreenCapturingButton = nodewith.Name("Start Screen Capturing").Role(role.Button).Ancestor(rootWebArea)
	stopScreenCapturingButton  = nodewith.Name("Stop Screen Capturing").Role(role.Button).Ancestor(rootWebArea)

	videoNode = nodewith.Role(role.Video).Ancestor(rootWebArea)
)

// SetupServerAndPermission sets the permission for the tab and returns the url.
func SetupServerAndPermission(ctx context.Context, br *browser.Browser, s *testing.State) string {
	// Grant permission.
	srv := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	br.GrantPermissions(ctx, []string{fmt.Sprintf("%s/*", srv.URL)},
		browser.CameraContentSetting,
		browser.MicrophoneContentSetting,
	)

	return srv.URL + vcTabURL
}

// LaunchTab opens a new tab for the url.
func LaunchTab(ctx context.Context, tconn *browser.TestConn, br *browser.Browser, url string) (*VcTabUI, error) {
	if _, err := br.NewTab(ctx, url); err != nil {
		return nil, err
	}

	return &VcTabUI{uiauto.New(tconn), br, url}, nil
}

// CloseTab closes the tab with id inside VcTabUI.
func (tabUI *VcTabUI) CloseTab(ctx context.Context) error {
	return tabUI.br.CloseWithURL(ctx, tabUI.url)
}

// StartVideo clicks on "Start Video" button to activate camera.
func (tabUI *VcTabUI) StartVideo(ctx context.Context) error {
	return tabUI.ui.DoDefault(startVideoButton)(ctx)
}

// StopVideo clicks on "Stop Video" button to deactivate camera.
func (tabUI *VcTabUI) StopVideo(ctx context.Context) error {
	return tabUI.ui.DoDefault(stopVideoButton)(ctx)
}

// StartAudio clicks on "Start Audio" button to activate microphone.
func (tabUI *VcTabUI) StartAudio(ctx context.Context) error {
	return uiauto.Combine("start audio",
		tabUI.ui.DoDefault(startAudioButton),
		// Add a short sleep after starting audio. Refer to b/298280544 for more details.
		uiauto.Sleep(100*time.Millisecond),
	)(ctx)
}

// StopAudio clicks on "Stop Audio" button to deactivate microphone.
func (tabUI *VcTabUI) StopAudio(ctx context.Context) error {
	return tabUI.ui.DoDefault(stopAudioButton)(ctx)
}

// StartScreenCapture clicks on "Start Screen Capturing" button to activate screen sharing.
func (tabUI *VcTabUI) StartScreenCapture(ctx context.Context) error {
	// There may be multiple "Choose what to share" dialogs, so add First() here.
	chooseWhatToShareWindow := nodewith.Role(role.Dialog).NameContaining("Choose what to share").HasClass("Widget").First()
	entireScreenTab := nodewith.Role(role.Tab).NameRegex(regexp.MustCompile("(?i)Entire Screen")).Ancestor(chooseWhatToShareWindow)
	display := nodewith.Role(role.Button).HasClass("DesktopMediaSourceView").Ancestor(chooseWhatToShareWindow).First()
	shareButton := nodewith.Name("Share").Role(role.Button).Ancestor(chooseWhatToShareWindow)
	return uiauto.Combine("share entire screen",
		tabUI.ui.DoDefaultUntil(
			startScreenCapturingButton,
			tabUI.ui.WithTimeout(3*time.Second).WaitUntilExists(chooseWhatToShareWindow),
		),
		tabUI.ui.DoDefault(entireScreenTab),
		tabUI.ui.DoDefault(display),
		tabUI.ui.DoDefault(shareButton),
	)(ctx)
}

// StopScreenCapture clicks on "Stop Screen Capturing" button to deactivate screen sharing.
func (tabUI *VcTabUI) StopScreenCapture(ctx context.Context) error {
	return tabUI.ui.DoDefault(stopScreenCapturingButton)(ctx)
}
