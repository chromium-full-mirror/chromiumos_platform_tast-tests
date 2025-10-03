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
	"go.chromium.org/tast/core/testing/hwdep"
)

type gamingAppParams struct {
	game       func(ctx context.Context, kb *input.KeyboardEventWriter, tconn *chrome.TestConn, a *arc.ARC, d *androidui.Device, dataPath func(string) string) gameapp.GameApp
	timeParams power.TimeParams
}

const (
	gamingAppPrepareTimeout = 10 * time.Minute

	shortPlayTime        = 6 * time.Minute
	asphalt8PlayTime     = 60 * time.Minute
	superTuxKartPlayTime = 30 * time.Minute

	shortTimeout        = gamingAppPrepareTimeout + shortPlayTime + power.RecorderTimeout
	asphalt8Timeout     = gamingAppPrepareTimeout + asphalt8PlayTime + power.RecorderTimeout
	superTuxKartTimeout = gamingAppPrepareTimeout + superTuxKartPlayTime + power.RecorderTimeout

	superTuxKartAPKURLVar = "power.super_tux_kart_apk_url"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: GamingApp,
		Desc: "Collect power metrics of gaming app",
		Contacts: []string{
			"chromeos-power-team@google.com",
			"cienet-development@googlegroups.com",
			"chicheny@google.com",
		},
		BugComponent: "b:1361410", // ChromeOS > Platform > System > Core Power
		// TODO(b/289855454): Add Attr after using uidetection.
		// Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome", "arc"},
		Vars:         []string{superTuxKartAPKURLVar},
		Fixture:      "powerAshARC",
		Params: []testing.Param{
			{
				Name: "asphalt8",
				Val: gamingAppParams{
					game: gameapp.NewAsphalt8,
					timeParams: power.TimeParams{
						Total: asphalt8PlayTime,
					},
				},
				Timeout:   asphalt8Timeout + setup.BatteryPreparationTimeout,
				ExtraData: []string{gameapp.Asphalt8IconGameScene},
				ExtraAttr: []string{
					"group:power",
					"power_regression",
					"group:release-health",
					"release-health_power",
				},
				ExtraHardwareDeps: hwdep.D(hwdep.MinStorage(17)), // 16Gb devices may not have enough space to install Asphalt 8.
			},
			{
				Name: "asphalt8_short",
				Val: gamingAppParams{
					game: gameapp.NewAsphalt8,
					timeParams: power.TimeParams{
						Total: shortPlayTime,
					},
				},
				Timeout:   shortTimeout + setup.BatteryPreparationTimeout,
				ExtraData: []string{gameapp.Asphalt8IconGameScene},
				ExtraAttr: []string{
					"group:power",
					"power_daily",
					"group:release-health",
					"release-health_power",
				},
				ExtraHardwareDeps: hwdep.D(hwdep.MinStorage(17)), // 16Gb devices may not have enough space to install Asphalt 8.
			},
			{
				Name: "super_tux_kart",
				Val: gamingAppParams{
					game: gameapp.NewSuperTuxKart,
					timeParams: power.TimeParams{
						Total: superTuxKartPlayTime,
					},
				},
				Timeout:   superTuxKartTimeout + setup.BatteryPreparationTimeout,
				ExtraData: []string{gameapp.SuperTuxKartIconGameScene},
				ExtraAttr: []string{
					"group:power",
					"power_regression",
					"group:release-health",
					"release-health_power",
				},
			},
			{
				Name: "super_tux_kart_short",
				Val: gamingAppParams{
					game: gameapp.NewSuperTuxKart,
					timeParams: power.TimeParams{
						Total: shortPlayTime,
					},
				},
				Timeout:   shortTimeout + setup.BatteryPreparationTimeout,
				ExtraData: []string{gameapp.SuperTuxKartIconGameScene},
				ExtraAttr: []string{
					"group:power",
					"power_daily",
					"group:release-health",
					"release-health_power",
				},
			},
		},
	})
}

// GamingApp collects power metrics of gaming app.
func GamingApp(ctx context.Context, s *testing.State) {

	discharge := s.FixtValue().(setup.PowerUIFixtureData).Discharge
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

	totalTime := s.Param().(gamingAppParams).timeParams.Total
	batteryCleanup, err := setup.Battery(ctx, totalTime, discharge)
	if err != nil {
		s.Fatal("Setup battery failed: ", err)
	}
	defer batteryCleanup(cleanupCtx)

	game := s.Param().(gamingAppParams).game(ctx, kb, tconn, a, d, s.DataPath)
	if superTuxKart, isSuperTuxKart := game.(*gameapp.SuperTuxKart); isSuperTuxKart {
		if url, ok := s.Var(superTuxKartAPKURLVar); ok {
			superTuxKart.SetAPKURL(url)
		}
	}
	// Run the app and collect the power data in the meantime.
	if err := gameapp.Run(ctx, cr, a, d, game, s.OutDir(), s.TestName(), totalTime, discharge); err != nil {
		s.Fatal("Failed to run game app: ", err)
	}
}
