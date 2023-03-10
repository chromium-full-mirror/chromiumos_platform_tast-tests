// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"time"

	"chromiumos/tast/common/perf"
	"chromiumos/tast/ctxutil"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/ash"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/browser/browserfixt"
	"chromiumos/tast/local/chrome/lacros"
	"chromiumos/tast/local/chrome/lacros/lacrosfixt"
	"chromiumos/tast/local/cpu"
	"chromiumos/tast/local/power"
	"chromiumos/tast/local/power/setup"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         IdleUI,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Collect power metrics when device is in idle with UI",
		BugComponent: "b:167191",
		Contacts:     []string{"chromeos-power@google.com"},
		// Disabled because this is an example test for other tests to follow.
		// Attr:      []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      3 * time.Minute,
		Params: []testing.Param{{
			Name: "ash",
			Val:  browser.TypeAsh,
		}, {
			Name:              "lacros",
			Val:               browser.TypeLacros,
			ExtraSoftwareDeps: []string{"lacros"},
		}},
	})
}

func IdleUI(ctx context.Context, s *testing.State) {
	// Reserve some time to cleanup, even if it fails due to ctx timeout.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Prepare Chrome browser.
	bt := s.Param().(browser.Type)
	opts := []chrome.Option{
		// --disable-sync disables test account info sync, eg. Wi-Fi credentials,
		// so that each test run does not remember info from last test run.
		chrome.ExtraArgs("--disable-sync"),
		// b/228256145 to avoid powerd restart.
		chrome.DisableFeatures("FirmwareUpdaterApp"),
	}
	cr, err := browserfixt.NewChrome(ctx, bt, lacrosfixt.NewConfig(lacrosfixt.Mode(lacros.LacrosOnly)), opts...)
	if err != nil {
		s.Fatal("Failed to login session: ", err)
	}
	defer cr.Close(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to get ash tconn: ", err)
	}

	// Set up the testing environment.
	su, cleanup := setup.New("power.IdleUI")
	defer cleanup(cleanupCtx)

	dischargeMode := setup.NoBatteryDischarge
	if _, err := power.SysfsBatteryPath(ctx); err == nil {
		dischargeMode = setup.ForceBatteryDischarge
	} else if !errors.Is(err, power.ErrNoBattery) {
		// If it's ErrNoBattery, leave dischargeMode at NoBatteryDischarge.
		s.Log("Unable to determine if a battery exists, do not force discharge: ", err)
	}

	su.Add(setup.PowerTest(ctx, tconn,
		setup.PowerTestOptions{
			Wifi:       setup.DisableWifiInterfaces,
			NightLight: setup.DisableNightLight,
			DarkTheme:  setup.EnableLightTheme,
		},
		setup.NewBatteryDischargeFromMode(dischargeMode),
	))
	if err := su.Check(ctx); err != nil {
		s.Error(err, "Power test setup failed: ", err)
	}

	// Open a window with about:blank tab on the target browser.
	conn, _, cleanup, err := browserfixt.SetUpWithURL(ctx, cr, bt, "about:blank")
	if err != nil {
		s.Fatal("Failed to open a blank new tab: ", err)
	}
	defer cleanup(cleanupCtx)
	defer conn.Close()

	w, err := ash.WaitForAnyWindow(ctx, tconn, ash.BrowserTypeMatch(bt))
	if err != nil {
		s.Fatal("Failed to open a browser window: ", err)
	}
	if err := ash.SetWindowStateAndWait(ctx, tconn, w.ID, ash.WindowStateMaximized); err != nil {
		s.Fatal("Failed to maximize the browser window: ", err)
	}

	// Wait until CPU is cooled down and idle.
	_, err = cpu.WaitUntilCoolDown(ctx, cpu.IdleCoolDownConfig())
	if err != nil {
		s.Error("CPU failed to cool down: ", err)
	}
	if err := cpu.WaitUntilIdle(ctx); err != nil {
		s.Error("CPU failed to idle: ", err)
	}

	metrics, err := perf.NewTimeline(ctx, power.TestMetrics(), perf.Interval(1*time.Second))
	if err != nil {
		s.Fatal("Failed to build metrics: ", err)
	}

	if err := metrics.Start(ctx); err != nil {
		s.Fatal("Failed to start metrics: ", err)
	}

	if err := metrics.StartRecording(ctx); err != nil {
		s.Fatal("Failed to start recording: ", err)
	}

	// Start of main test body. Idle for 10 seconds while reading power metrics
	// every second. This both serves as an example for future power tests and
	// as a light weight test to test the device setup. Replace this chunk of
	// code with functionality code for future power tests.
	if err := testing.Sleep(ctx, 10*time.Second); err != nil {
		s.Fatal("Failed to sleep: ", err)
	}
	// End of main test body.

	p, err := metrics.StopRecording(ctx)
	if err != nil {
		s.Fatal("Error while recording power metrics: ", err)
	}

	if err := power.GeneratePowerLogAndSaveToCrosbolt(ctx, s.OutDir(), s.TestName(), p); err != nil {
		s.Error("Failed to save and upload power metrics: ", err)
	}
}
