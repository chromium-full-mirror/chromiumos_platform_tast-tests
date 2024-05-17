// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package fakearc contains the fake VcTester arc app.
package fakearc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/local/arc/apputil"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/videoconferencing/fixture"
	"go.chromium.org/tast/core/testing"

	"go.chromium.org/tast-tests/cros/common/testexec"
)

var (
	// VcAppName is the name of App used for extension, pwa and tab.
	VcAppName = "VcTester"
	// AppNameArc is the APK name of the app.
	AppNameArc = "VcTester.apk"
	// ArcPackageName is the package name of the VcTester.apk.
	ArcPackageName = "chromeos.videoconferencing.vctester"

	startVideoButtonText = "START CAMERA"
	stopVideoButtonText  = "STOP CAMERA"
	startAudioButtonText = "START AUDIO"
	stopAudioButtonText  = "STOP AUDIO"
	audioPlayButtonText  = "PLAY HELLO"
)

// VcArcApp defines common actions for extension, pwa and tab.
type VcArcApp struct {
	app      *apputil.App
	cr       *chrome.Chrome
	hasError func() bool
	outDir   string
}

// Install installs the VcTest.apk and grant permissions.
func Install(ctx context.Context, s *testing.State) {

	a := s.FixtValue().(fixture.FixtData).ARC()

	if err := a.Install(ctx, s.DataPath(AppNameArc)); err != nil {
		s.Fatal("Failed to install the APK: ", err)
	}

	permissions := []string{
		"android.permission.CAMERA",
		"android.permission.RECORD_AUDIO"}
	for _, permission := range permissions {
		if err := a.Command(ctx, "pm", "grant", ArcPackageName, permission).Run(testexec.DumpLogOnError); err != nil {
			s.Fatal("Failed to grant permission : ", err)
		}
	}
}

// Launch an instance of the VcTest arc app.
func Launch(ctx context.Context, s *testing.State, tconn *chrome.TestConn, cr *chrome.Chrome, kb *input.KeyboardEventWriter) *VcArcApp {
	a := s.FixtValue().(fixture.FixtData).ARC()
	d := s.FixtValue().(fixture.FixtData).UIDevice()

	app, err := apputil.NewApp(ctx, kb, tconn, a, d, VcAppName, ArcPackageName)
	if err != nil {
		s.Fatal("Failed to new app : ", err)
	}

	if _, err := app.Launch(ctx); err != nil {
		s.Fatal("Failed to launch app : ", err)
	}

	if err := uiauto.New(tconn).RightClick(nodewith.ClassName("HeaderView"))(ctx); err != nil {
		s.Fatal("Failed to make test app be focused: ", err)
	}

	return &VcArcApp{app, cr, s.HasError, s.OutDir()}
}

// StartVideo clicks on "Start Video" button to activate camera.
func (arcApp *VcArcApp) StartVideo(ctx context.Context) error {
	return arcApp.clickOnButton(ctx, startVideoButtonText)
}

// StopVideo clicks on "Stop Video" button to deactivate camera.
func (arcApp *VcArcApp) StopVideo(ctx context.Context) error {
	return arcApp.clickOnButton(ctx, stopVideoButtonText)
}

// StartAudio clicks on "Start Audio" button to activate microphone.
func (arcApp *VcArcApp) StartAudio(ctx context.Context) error {
	return arcApp.clickOnButton(ctx, startAudioButtonText)
}

// StopAudio clicks on "Stop Audio" button to deactivate microphone.
func (arcApp *VcArcApp) StopAudio(ctx context.Context) error {
	return arcApp.clickOnButton(ctx, stopAudioButtonText)
}

// PlayAudio clicks on the audioPlayButton button.
func (arcApp *VcArcApp) PlayAudio(ctx context.Context) error {
	return arcApp.clickOnButton(ctx, audioPlayButtonText)
}

// Close close the app.
func (arcApp *VcArcApp) Close(ctx context.Context) error {
	return arcApp.app.Close(ctx, arcApp.cr, arcApp.hasError, arcApp.outDir)
}

func (arcApp *VcArcApp) clickOnButton(ctx context.Context, text string) error {

	filename := arcApp.app.Device.Object(ui.TextContains(text))

	if err := apputil.FindAndClick(filename, 5*time.Second)(ctx); err != nil {
		return err
	}
	return nil
}
