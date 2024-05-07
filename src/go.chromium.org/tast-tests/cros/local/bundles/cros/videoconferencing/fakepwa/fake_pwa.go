// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package fakepwa contains the library of fake VC PWA app.
package fakepwa

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/testing"
)

// VcPwaUI represents the Fake VC Tab UI.
// It is usually launched by browersing to fake html.
type VcPwaUI struct {
	tconn *chrome.TestConn
	ui    *uiauto.Context
	appID string
}

var (
	vcPwaName   = "VcTester"
	vcPwaURL    = "/vc_tester/popup.html"
	rootWebArea = nodewith.Role(role.RootWebArea).Name(vcPwaName)

	startVideoButton           = nodewith.Name("Start Video").Role(role.Button).Ancestor(rootWebArea)
	stopVideoButton            = nodewith.Name("Stop Video").Role(role.Button).Ancestor(rootWebArea)
	startAudioButton           = nodewith.Name("Start Audio").Role(role.Button).Ancestor(rootWebArea)
	stopAudioButton            = nodewith.Name("Stop Audio").Role(role.Button).Ancestor(rootWebArea)
	startScreenCapturingButton = nodewith.Name("Start Screen Capturing").Role(role.Button).Ancestor(rootWebArea)
	stopScreenCapturingButton  = nodewith.Name("Stop Screen Capturing").Role(role.Button).Ancestor(rootWebArea)

	videoNode = nodewith.Role(role.Video).Ancestor(rootWebArea)
)

// SetupServerAndPermission installs the app, sets its permission and returns its appID.
func SetupServerAndPermission(ctx context.Context, br *browser.Browser, s *testing.State, tconn *chrome.TestConn) string {
	// Grant permission.
	srv := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	br.GrantPermissions(ctx, []string{fmt.Sprintf("%s/*", srv.URL)},
		browser.CameraContentSetting,
		browser.MicrophoneContentSetting,
	)

	vcPwaFullURL := srv.URL + vcPwaURL
	if err := apps.InstallPWAForURL(ctx, tconn, br, vcPwaFullURL, 15*time.Second); err != nil {
		s.Fatal("Failed to install PWA for URL: ", err)
	}
	appID, err := apps.InstalledAppID(ctx, tconn, func(app *ash.ChromeApp) bool {
		return app.Name == "VcTester"
	}, &testing.PollOptions{Timeout: 5 * time.Second})
	if err != nil {
		s.Fatal("Failed to get appID: ", err)
	}
	if err = apps.Close(ctx, tconn, appID); err != nil {
		s.Fatal("Failed to close app: ", err)
	}

	return appID
}

// LaunchApp opens an app with appID.
func LaunchApp(ctx context.Context, tconn *browser.TestConn, br *browser.Browser, appID string) (*VcPwaUI, error) {
	if err := apps.Launch(ctx, tconn, appID); err != nil {
		return nil, err
	}

	return &VcPwaUI{tconn, uiauto.New(tconn), appID}, nil
}

// CloseApp closes the app with appID inside VcPwaUI.
func (pwaUI *VcPwaUI) CloseApp(ctx context.Context) error {
	if err := apps.Close(ctx, pwaUI.tconn, pwaUI.appID); err != nil {
		return err
	}

	return ash.WaitForAppClosed(ctx, pwaUI.tconn, pwaUI.appID)
}

// StartVideo clicks on "Start Video" button to activate camera.
func (pwaUI *VcPwaUI) StartVideo(ctx context.Context) error {
	return pwaUI.ui.DoDefault(startVideoButton)(ctx)
}

// StopVideo clicks on "Stop Video" button to deactivate camera.
func (pwaUI *VcPwaUI) StopVideo(ctx context.Context) error {
	return pwaUI.ui.DoDefault(stopVideoButton)(ctx)
}

// StartAudio clicks on "Start Audio" button to activate microphone.
func (pwaUI *VcPwaUI) StartAudio(ctx context.Context) error {
	return uiauto.Combine("start audio",
		pwaUI.ui.DoDefault(startAudioButton),
		// Add a short sleep after starting audio. Refer to b/298280544 for more details.
		uiauto.Sleep(100*time.Millisecond),
	)(ctx)
}

// StopAudio clicks on "Stop Audio" button to deactivate microphone.
func (pwaUI *VcPwaUI) StopAudio(ctx context.Context) error {
	return pwaUI.ui.DoDefault(stopAudioButton)(ctx)
}

// StartScreenCapture clicks on "Start Screen Capturing" button to activate screen sharing.
func (pwaUI *VcPwaUI) StartScreenCapture(ctx context.Context) error {
	// There may be multiple "Choose what to share" dialogs, so add First() here.
	chooseWhatToShareWindow := nodewith.Role(role.Dialog).NameContaining("Choose what to share").HasClass("Widget").First()
	entireScreenTab := nodewith.Role(role.Tab).NameRegex(regexp.MustCompile("(?i)Entire Screen")).Ancestor(chooseWhatToShareWindow)
	display := nodewith.Role(role.Button).HasClass("DesktopMediaSourceView").Ancestor(chooseWhatToShareWindow).First()
	shareButton := nodewith.Name("Share").Role(role.Button).Ancestor(chooseWhatToShareWindow)
	return uiauto.Combine("share entire screen",
		pwaUI.ui.DoDefaultUntil(
			startScreenCapturingButton,
			pwaUI.ui.WithTimeout(3*time.Second).WaitUntilExists(chooseWhatToShareWindow),
		),
		pwaUI.ui.DoDefault(entireScreenTab),
		pwaUI.ui.DoDefault(display),
		pwaUI.ui.DoDefault(shareButton),
	)(ctx)
}

// StopScreenCapture clicks on "Stop Screen Capturing" button to deactivate screen sharing.
func (pwaUI *VcPwaUI) StopScreenCapture(ctx context.Context) error {
	return pwaUI.ui.DoDefault(stopScreenCapturingButton)(ctx)
}
