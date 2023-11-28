// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"time"

	androidui "go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/power/gameapp"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/power/setup"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

type gamingAppParams struct {
	game func(ctx context.Context, kb *input.KeyboardEventWriter, tconn *chrome.TestConn, a *arc.ARC, d *androidui.Device, dataPath func(string) string) gameapp.GameApp
}

const (
	gamingAppPrepareTimeout = 10 * time.Minute
	gamingAppPlayTime       = 3 * time.Minute
	gamingAppTimeout        = gamingAppPrepareTimeout + gamingAppPlayTime + power.RecorderTimeout

	superTuxKartAPKURLVar = "power.super_tux_kart_apk_url"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         GamingApp,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Collect power metrics of gaming app",
		Contacts: []string{
			"chromeos-platform-power@google.com",
			"cienet-development@googlegroups.com",
			"chicheny@google.com",
		},
		BugComponent: "b:1361410", // ChromeOS > Platform > System > Core Power
		// TODO(b/289855454): Add Attr after using uidetection.
		// Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome", "arc"},
		Vars:         []string{superTuxKartAPKURLVar},
		Fixture:      "powerAshARC",
		Timeout:      gamingAppTimeout,
		Params: []testing.Param{
			{
				Name: "asphalt8",
				Val: gamingAppParams{
					game: gameapp.NewAsphalt8,
				},
				ExtraData: []string{gameapp.Asphalt8IconGameScene},
				ExtraAttr: []string{"group:power", "power_regression"},
			},
			{
				Name: "super_tux_kart",
				Val: gamingAppParams{
					game: gameapp.NewSuperTuxKart,
				},
				ExtraData: []string{gameapp.SuperTuxKartIconGameScene},
				ExtraAttr: []string{"group:power", "power_regression"},
			},
		},
	})
}

// GamingApp collects power metrics of gaming app.
func GamingApp(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(setup.PowerUIFixtureData).Cr
	a := s.FixtValue().(setup.PowerUIFixtureData).ARC

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()
	d, err := a.NewUIDevice(ctx)
	if err != nil {
		s.Fatal("Failed initializing UI Automator: ", err)
	}
	defer d.Close(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}
	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get the keyboard: ", err)
	}
	defer kb.Close(cleanupCtx)

	game := s.Param().(gamingAppParams).game(ctx, kb, tconn, a, d, s.DataPath)
	if superTuxKart, isSuperTuxKart := game.(*gameapp.SuperTuxKart); isSuperTuxKart {
		if url, ok := s.Var(superTuxKartAPKURLVar); ok {
			superTuxKart.SetAPKURL(url)
		}
	}
	// Run the app and collect the power data in the meantime.
	if err := gameapp.Run(ctx, cr, a, d, game, s.OutDir(), s.TestName(), gamingAppPlayTime); err != nil {
		s.Fatal("Failed to run game app: ", err)
	}
}
