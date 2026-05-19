// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gameapp

import (
	"context"
	"fmt"
	"time"

	androidui "go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/power/util"
	"go.chromium.org/tast-tests/cros/local/uidetection"
)

const (
	// SuperTuxKartAppName is the app name of SuperTuxKart game.
	SuperTuxKartAppName = "SuperTuxKart"
	// SuperTuxKartIconGameScene is the icon data used in SuperTuxKart game.
	SuperTuxKartIconGameScene = "gameapp/supertuxkart_game_scene.png"
	superTuxKartPackageName   = "org.supertuxkart.stk"
	superTuxKartIDPrefix      = superTuxKartPackageName + ":id/"

	// defaultSuperTuxKartAPKURL is the URL base for the SuperTuxKart APK
	// with the default version 1.4 on the Google Cloud bucket.
	// The official APK can be found under the Assets
	// on https://github.com/supertuxkart/stk-code/releases/download/1.4/SuperTuxKart-1.4.apk.
	defaultSuperTuxKartAPKURL = "https://storage.googleapis.com/chromiumos-test-assets-public/tast/cros/power/super-tux-kart/v1.4/SuperTuxKart-1.4.apk"
)

// SuperTuxKart holds the information for Game App testing.
type SuperTuxKart struct {
	a        *arc.ARC
	d        *androidui.Device
	kb       *input.KeyboardEventWriter
	tconn    *chrome.TestConn
	dataPath func(string) string
	launched bool
	apkURL   string
}

// NewSuperTuxKart creates SuperTuxKart instance which implements GameApp interface.
func NewSuperTuxKart(ctx context.Context, kb *input.KeyboardEventWriter, tconn *chrome.TestConn, a *arc.ARC, d *androidui.Device, dataPath func(string) string) GameApp {
	return &SuperTuxKart{
		a:        a,
		d:        d,
		kb:       kb,
		tconn:    tconn,
		dataPath: dataPath,
		apkURL:   defaultSuperTuxKartAPKURL,
	}
}

var _ GameApp = (*SuperTuxKart)(nil)

// Install installs the SuperTuxKart game app via play store.
func (s *SuperTuxKart) Install(ctx context.Context) error {
	return util.InstallAppFromAPKURL(ctx, s.a, s.d, superTuxKartPackageName, s.apkURL)
}

// Uninstall uninstalls the SuperTuxKart game app if it has been installed.
func (s *SuperTuxKart) Uninstall(ctx context.Context) error {
	return util.UninstallApp(ctx, s.a, superTuxKartPackageName)
}

// Launch launches the SuperTuxKart game app.
func (s *SuperTuxKart) Launch(ctx context.Context) error {
	if err := util.LaunchApp(ctx, s.tconn, s.kb, apps.SuperTuxKart); err != nil {
		return err
	}

	s.launched = true
	return nil
}

// IsLaunched returns whether the SuperTuxKart game app is launched.
func (s *SuperTuxKart) IsLaunched() bool {
	return s.launched
}

// EnterGameScene enters the game scene by keyboard.
func (s *SuperTuxKart) EnterGameScene(ctx context.Context) error {
	const (
		okButton       = "'OK' button"
		applyButton    = "'Apply' button"
		yesButton      = "'Yes' button"
		buttonWaitTime = 2 * time.Second
	)
	kb := s.kb
	ui := uiauto.New(s.tconn)
	gotItButton := nodewith.Name("Got it").Role(role.Button)
	ud := uidetection.NewDefault(s.tconn).WithScreenshotStrategy(uidetection.ImmediateScreenshot)
	applyWord := uidetection.Word("Apply")
	gameScene := uidetection.CustomIcon(s.dataPath(SuperTuxKartIconGameScene))

	enterControllerSelectionScene := ui.WithTimeout(2*time.Minute).RetryUntil(kb.AccelAction("Enter"),
		ud.WithTimeout(15*time.Second).WaitUntilExists(applyWord))
	goThroughConfirmation := uiauto.NamedCombine("go through confirmation",
		uiauto.NamedAction("press enter for "+applyButton, kb.AccelAction("Enter")),
		// Wait up to 2s for the 'Yes' button.
		uiauto.NamedAction("wait "+yesButton, uiauto.Sleep(buttonWaitTime)),
		uiauto.NamedAction("press enter for "+yesButton, kb.AccelAction("Enter")),
		// Wait up to 2s for the 'Yes' button.
		uiauto.NamedAction("wait "+yesButton, uiauto.Sleep(buttonWaitTime)),
		uiauto.NamedAction("press enter for "+yesButton, kb.AccelAction("Enter")),
	)
	return uiauto.NamedCombine("enter game scene",
		uiauto.IfSuccessThen(ui.WithTimeout(5*time.Second).WaitUntilExists(gotItButton), ui.LeftClick(gotItButton)),
		// The DUT might get into the game scene when trying to enter the
		// controller selection scene.
		// Directly wait for the game scene if |enterControllerSelectionScene| fails.
		uiauto.IfSuccessThen(
			enterControllerSelectionScene,
			goThroughConfirmation,
		),
		uiauto.NamedAction("wait to enter the game scene", ud.WaitUntilExists(gameScene)),
	)(ctx)
}

// Play plays the game by keyboard for play time.
func (s *SuperTuxKart) Play(ctx context.Context, totalPlayTime time.Duration) error {
	const forwardTime = 5 * time.Second
	playTime := totalPlayTime - forwardTime

	kb := s.kb
	driveForward := uiauto.NamedCombine(fmt.Sprintf("drive forward for %v", forwardTime),
		kb.AccelPressAction("Up"),
		uiauto.Sleep(forwardTime),
		kb.AccelReleaseAction("Up"))
	driveCircle := uiauto.NamedCombine(fmt.Sprintf("drive circle for %v", playTime),
		kb.AccelPressAction("Up"),
		kb.AccelPressAction("Right"),
		uiauto.Sleep(playTime),
		kb.AccelReleaseAction("Up"),
		kb.AccelReleaseAction("Right"))

	return uiauto.NamedCombine(fmt.Sprintf("play the SuperTuxKart game for %v", totalPlayTime),
		driveForward,
		driveCircle,
	)(ctx)
}

// End closes the game app if it is launched.
func (s *SuperTuxKart) End(ctx context.Context) error {
	if !s.launched {
		return nil
	}
	if err := util.CloseApp(ctx, s.tconn, superTuxKartPackageName); err != nil {
		return err
	}
	s.launched = false
	return nil
}

// SetAPKURL sets the APK URL of SuperTuxKart.
func (s *SuperTuxKart) SetAPKURL(apkURL string) {
	s.apkURL = apkURL
}
