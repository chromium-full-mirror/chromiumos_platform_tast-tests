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
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/power/util"
	"go.chromium.org/tast-tests/cros/local/uidetection"
)

const (
	// Asphalt8AppName is the app name of Asphalt8.
	Asphalt8AppName = "Asphalt 8"
	// Asphalt8IconGameScene is the icon data of the game scene used in Asphalt8.
	Asphalt8IconGameScene = "gameapp/asphalt8_game_scene.png"
	asphalt8PackageName   = "com.gameloft.android.ANMP.GloftA8HM"
	asphalt8IDPrefix      = asphalt8PackageName + ":id/"
)

// Asphalt8 holds the information for Game App testing.
type Asphalt8 struct {
	a        *arc.ARC
	d        *androidui.Device
	kb       *input.KeyboardEventWriter
	tconn    *chrome.TestConn
	dataPath func(string) string
	launched bool
}

// NewAsphalt8 creates Asphalt8 instance which implements GameApp interface.
func NewAsphalt8(ctx context.Context, kb *input.KeyboardEventWriter, tconn *chrome.TestConn, a *arc.ARC, d *androidui.Device, dataPath func(string) string) GameApp {
	return &Asphalt8{
		a:        a,
		d:        d,
		kb:       kb,
		tconn:    tconn,
		dataPath: dataPath,
	}
}

var _ GameApp = (*Asphalt8)(nil)

// Install installs the Asphalt8 game app via play store.
func (as *Asphalt8) Install(ctx context.Context) error {
	return util.InstallApp(ctx, as.tconn, as.a, as.d, asphalt8PackageName)
}

// Uninstall uninstalls the Asphalt8 game app if it has been installed.
func (as *Asphalt8) Uninstall(ctx context.Context) error {
	return util.UninstallApp(ctx, as.a, asphalt8PackageName)
}

// Launch launches the Asphalt8 game app.
func (as *Asphalt8) Launch(ctx context.Context) error {
	if err := util.LaunchApp(ctx, as.tconn, as.kb, apps.Asphalt8); err != nil {
		return err
	}

	as.launched = true
	return nil
}

// EnterGameScene enters the game scene by keyboard.
func (as *Asphalt8) EnterGameScene(ctx context.Context) error {
	kb := as.kb
	ud := uidetection.NewDefault(as.tconn).WithScreenshotStrategy(uidetection.ImmediateScreenshot)

	notNowButton := as.d.Object(androidui.Text("Not now"), androidui.ClassName("android.widget.Button"))
	actionBarRoot := as.d.Object(androidui.ID(asphalt8IDPrefix + "action_bar_root"))
	gameScene := uidetection.CustomIcon(as.dataPath(Asphalt8IconGameScene), uidetection.MinConfidence(0.65))
	raceNow := uidetection.TextBlock([]string{"RACE", "NOW!"})
	return uiauto.NamedCombine("enter game scene",
		// Dismiss the profile dialog if it exists.
		cuj.ClickIfExist(notNowButton, defaultUITimeout),
		cuj.WaitForExists(actionBarRoot, defaultUITimeout),
		uiauto.NamedAction("press enter to skip animation", kb.AccelAction("Enter")),
		// On low-end devices, wait up to 2 minutes for the 'RACE-NOW' button.
		uiauto.NamedAction("wait 'RACE-NOW' button", ud.WithTimeout(2*time.Minute).WaitUntilExists(raceNow)),
		uiauto.NamedAction("press 'RACE-NOW' button", kb.AccelAction("Enter")),
		// Wait up to 2s for the 'LEARN TO DRIVE' menu.
		uiauto.NamedAction("wait 'LEARN TO DRIVE' menu", uiauto.Sleep(2*time.Second)),
		uiauto.NamedAction("press left to select 'MINI-GAME' button", kb.AccelAction("Left")),
		// Wait up to 2s for the 'MINI-GAME' button.
		uiauto.NamedAction("wait 'MINI-GAME' button", uiauto.Sleep(2*time.Second)),
		uiauto.NamedAction("press enter to enter the game scene", kb.AccelAction("Enter")),
		// On low-end devices, wait up to 3 minutes for entering the game scene.
		uiauto.NamedAction("wait entering the game scene",
			ud.WithTimeout(3*time.Minute).WaitUntilExists(gameScene)),
	)(ctx)
}

// Play plays the game by keyboard for play time.
func (as *Asphalt8) Play(ctx context.Context, totalPlayTime time.Duration) error {
	const (
		key          = "Right+Down"
		startCarTime = 2 * time.Second
	)
	driveInCirclesTime := totalPlayTime - startCarTime

	kb := as.kb
	startCar := uiauto.NamedCombine(fmt.Sprintf("start the car for %v", startCarTime),
		kb.AccelPressAction(key),
		uiauto.Sleep(startCarTime),
		kb.AccelReleaseAction(key))
	driveInCircles := uiauto.NamedCombine(fmt.Sprintf("drive in circles for %v", driveInCirclesTime),
		kb.AccelPressAction(key),
		uiauto.Sleep(driveInCirclesTime),
		kb.AccelReleaseAction(key))

	return uiauto.NamedCombine(fmt.Sprintf("play the Asphalt8 game for %v", totalPlayTime),
		startCar,
		driveInCircles,
	)(ctx)
}

// End closes the game app if it is launched.
func (as *Asphalt8) End(ctx context.Context) error {
	if !as.launched {
		return nil
	}
	return util.CloseApp(ctx, as.tconn, asphalt8PackageName)
}
