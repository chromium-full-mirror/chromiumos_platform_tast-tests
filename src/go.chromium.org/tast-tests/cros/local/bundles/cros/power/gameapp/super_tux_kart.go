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
)

const (
	// SuperTuxKartAppName is the app name of SuperTuxKart game.
	SuperTuxKartAppName     = "SuperTuxKart"
	superTuxKartPackageName = "org.supertuxkart.stk"
	superTuxKartIDPrefix    = superTuxKartPackageName + ":id/"
)

// SuperTuxKart holds the information for Game App testing.
type SuperTuxKart struct {
	a        *arc.ARC
	d        *androidui.Device
	kb       *input.KeyboardEventWriter
	tconn    *chrome.TestConn
	launched bool
}

// NewSuperTuxKart creates SuperTuxKart instance which implements GameApp interface.
func NewSuperTuxKart(ctx context.Context, kb *input.KeyboardEventWriter, tconn *chrome.TestConn, a *arc.ARC, d *androidui.Device) GameApp {
	return &SuperTuxKart{
		a:     a,
		d:     d,
		kb:    kb,
		tconn: tconn,
	}
}

var _ GameApp = (*SuperTuxKart)(nil)

// Install installs the SuperTuxKart game app via play store.
func (s *SuperTuxKart) Install(ctx context.Context) error {
	return util.InstallApp(ctx, s.tconn, s.a, s.d, superTuxKartPackageName)
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

// EnterGameScene enters the game scene by keyboard.
func (s *SuperTuxKart) EnterGameScene(ctx context.Context) error {
	const (
		okButton    = "'OK' button"
		applyButton = "'Apply' button"
		yesButton   = "'Yes' button"
	)
	kb := s.kb
	ui := uiauto.New(s.tconn)
	gotItButton := nodewith.Name("Got it").Role(role.Button)

	return uiauto.NamedCombine("enter game scene",
		uiauto.IfSuccessThen(ui.WithTimeout(5*time.Second).WaitUntilExists(gotItButton), ui.LeftClick(gotItButton)),
		uiauto.NamedAction("press enter to skip animation", kb.AccelAction("Enter")),
		// Most UI elements in the game don't have a resource-id, so currently
		// we're using sleep to wait for the UI element to appear, while this
		// is well known as the source of flakiness for short term.
		// TODO(b/289855454): Use uidetection to wait the event properly to
		// stabilize the tests. Until that, we expect tests using this will not
		// run in any of automated suite without manual triages of failures.
		// On low-end devices, wait up to 40s for the 'OK' button.
		// TODO(b/289855454): Use uidetection to wait for the 'OK' button.
		uiauto.NamedAction("wait "+okButton, uiauto.Sleep(40*time.Second)),
		uiauto.NamedAction("press enter for "+okButton, kb.AccelAction("Enter")),
		// On low-end devices, wait up to 2s for the 'Apply' button.
		// TODO(b/289855454): Use uidetection to wait for the 'Apply' button.
		uiauto.NamedAction("wait "+applyButton, uiauto.Sleep(2*time.Second)),
		uiauto.NamedAction("press enter for "+applyButton, kb.AccelAction("Enter")),
		// On low-end devices, wait up to 2s for the 'Yes' button.
		// TODO(b/289855454): Use uidetection to wait for the 'Yes' button.
		uiauto.NamedAction("wait "+yesButton, uiauto.Sleep(2*time.Second)),
		uiauto.NamedAction("press enter for "+yesButton, kb.AccelAction("Enter")),
		// On low-end devices, wait up to 2s for the 'Yes' button.
		// TODO(b/289855454): Use uidetection to wait for the 'Yes' button.
		uiauto.NamedAction("wait "+yesButton, uiauto.Sleep(2*time.Second)),
		uiauto.NamedAction("press enter for "+yesButton, kb.AccelAction("Enter")),
		// On low-end devices, wait up to 15s for entering the game scene.
		// TODO(b/289855454): Use uidetection to verify entering the game scene.
		uiauto.NamedAction("wait entering the game scene", uiauto.Sleep(15*time.Second)))(ctx)
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
	return util.CloseApp(ctx, s.tconn, superTuxKartPackageName)
}
