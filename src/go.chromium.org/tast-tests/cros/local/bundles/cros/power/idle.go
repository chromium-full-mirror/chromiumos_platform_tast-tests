// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/bluetooth/bluez"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/power/setup"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

var idleTimeParams = power.TimeParams{Interval: 20 * time.Second, Total: 4 * time.Minute}
var idleFastTimeParams = power.TimeParams{Interval: 10 * time.Second, Total: 20 * time.Second}

var displayOffBTOff = power.IdleParams{
	DisplayPower:   false,
	BluetoothPower: false,
	IdleTimeParams: idleTimeParams}
var displayOnBTOff = power.IdleParams{
	DisplayPower:   true,
	BluetoothPower: false,
	IdleTimeParams: idleTimeParams}
var displayOnBTOn = power.IdleParams{
	DisplayPower:   true,
	BluetoothPower: true,
	IdleTimeParams: idleTimeParams}
var displayOffBTOn = power.IdleParams{
	DisplayPower:   false,
	BluetoothPower: true,
	IdleTimeParams: idleTimeParams}

var defaultFast = power.IdleParams{
	DisplayPower:   true,
	BluetoothPower: true,
	IdleTimeParams: idleFastTimeParams}

func init() {
	testing.AddTest(&testing.Test{
		Func:         Idle,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Collects data on idle with Chrome logged in",
		BugComponent: "b:1361410",
		Contacts:     []string{"chromeos-platform-power@google.com", "jingmuli@google.com"},
		Attr:         []string{"group:crosbolt", "crosbolt_perbuild"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      10*time.Minute + power.RecorderTimeout,
		Params: []testing.Param{{
			Name:    "display_off_bt_off_ash",
			Fixture: "powerAsh",
			Val:     displayOffBTOff,
		}, {
			Name:    "display_on_bt_off_ash",
			Fixture: "powerAsh",
			Val:     displayOnBTOff,
		}, {
			Name:    "display_on_bt_on_ash",
			Fixture: "powerAsh",
			Val:     displayOnBTOn,
		}, {
			Name:    "display_off_bt_on_ash",
			Fixture: "powerAsh",
			Val:     displayOffBTOn,
		}, {
			Name:    "default_ash",
			Fixture: "powerAsh",
			Val:     displayOnBTOn,
		}, {
			Name:    "default_fast_ash",
			Fixture: "powerAsh",
			Val:     defaultFast,
		}, {
			Name:              "display_off_bt_off_lacros",
			Fixture:           "powerLacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               displayOffBTOff,
		}, {
			Name:              "display_on_bt_off_lacros",
			Fixture:           "powerLacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               displayOnBTOff,
		}, {
			Name:              "display_on_bt_on_lacros",
			Fixture:           "powerLacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               displayOnBTOn,
		}, {
			Name:              "display_off_bt_on_lacros",
			Fixture:           "powerLacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               displayOffBTOn,
		}, {
			Name:              "default_lacros",
			Fixture:           "powerLacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               displayOnBTOn,
		}, {
			Name:              "default_fast_lacros",
			Fixture:           "powerLacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               defaultFast,
		}},
	})
}

func Idle(ctx context.Context, s *testing.State) {
	// Reserve some time to cleanup, even if it fails due to ctx timeout.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	bt := s.FixtValue().(setup.PowerUIFixtureData).Bt
	cr := s.FixtValue().(setup.PowerUIFixtureData).Cr

	bts, err := bluez.Adapters(ctx)
	if err != nil {
		s.Fatal("Bluetooth adapters fail to be created: ", err)
	}
	setBluetoothPower := func(enabled bool) {
		for _, bt := range bts {
			if err := bt.SetPowered(ctx, enabled); err != nil {
				s.Fatalf("Failed to set powered to bluetooth %s to %v: %v", bt.DBusObject().ObjectPath(), enabled, err)
			}
		}
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to get ash tconn: ", err)
	}

	// Open a window with about:blank tab on the target browser.
	conn, _, cleanup, err := browserfixt.SetUpWithURL(ctx, cr, bt, "about:blank")
	if err != nil {
		s.Fatal("Failed to open a blank new tab: ", err)
	}
	defer cleanup(cleanupCtx)
	defer conn.Close()
	defer conn.CloseTarget(cleanupCtx)

	w, err := ash.WaitForAnyWindow(ctx, tconn, ash.BrowserTypeMatch(bt))
	if err != nil {
		s.Fatal("Failed to open a browser window: ", err)
	}
	if err := ash.SetWindowStateAndWait(ctx, tconn, w.ID, ash.WindowStateMaximized); err != nil {
		s.Fatal("Failed to maximize the browser window: ", err)
	}

	r := power.NewRecorder(ctx, idleTimeParams.Interval, s.OutDir(), s.TestName())
	defer r.Close(cleanupCtx)

	var params = s.Param().(power.IdleParams)

	s.Logf("Display is on %t, BT is on %t", params.DisplayPower, params.BluetoothPower)
	if params.DisplayPower {
		if err := power.SetDisplayPower(ctx, power.DisplayPowerAllOn); err != nil {
			s.Fatal("Failed to turn on display: ", err)
		}
	} else {
		if err := power.SetDisplayPower(ctx, power.DisplayPowerAllOff); err != nil {
			s.Fatal("Failed to turn off display: ", err)
		}
	}
	setBluetoothPower(params.BluetoothPower)

	if err := r.Cooldown(ctx); err != nil {
		s.Error("Cooldown failed: ", err)
	}

	if err := r.Start(ctx); err != nil {
		s.Fatal("Cannot start collecting power metrics: ", err)
	}
	// GoBigSleepLint: sleep to let the device idle.
	if err := testing.Sleep(ctx, time.Duration(params.IdleTimeParams.Total)); err != nil {
		s.Fatal("Failed to wait idling: ", err)
	}

	if err := r.Finish(ctx); err != nil {
		s.Error("Cannot finish collecting power metrics: ", err)
	}
}
