// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/power/setup"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SetupCheck,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Check the health of power test library setup",
		BugComponent: "b:1361410",
		Contacts: []string{
			"chromeos-platform-power@google.com",
			"yanyeli@google.com",
		},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		Attr:         []string{"group:mainline", "informational"},
		Timeout:      3 * time.Minute,
	})
}

func SetupCheck(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 1*time.Minute)
	defer cancel()

	// Only Ash tconn is used by some setups.
	cr, err := browserfixt.NewChrome(ctx, browser.TypeAsh, lacrosfixt.NewConfig())
	if err != nil {
		s.Fatal("Failed to login session: ", err)
	}
	defer cr.Close(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to get ash tconn: ", err)
	}

	su, cleanup := setup.New("setup check")
	defer cleanup(cleanupCtx)

	discharge := false
	if _, err := power.SysfsBatteryPath(ctx); err == nil {
		discharge = true
	} else if errors.Is(err, power.ErrNoBattery) {
		// If it's ErrNoBattery, leave dischargeMode at NoBatteryDischarge.
		testing.ContextLog(ctx, "Unable to find battery, do not force discharge: ", err)
	} else {
		testing.ContextLog(ctx, "Unable to determine if a battery exists, do not force discharge: ", err)
	}
	// Change options that by default do nothing.
	testOptions := setup.PowerTestOptions{
		Wifi:       setup.DisableWifiInterfaces,
		NightLight: setup.EnableNightLight,
		DarkTheme:  setup.EnableDarkTheme,
		Ramfs:      setup.SetupRamfs,
	}
	if setup.ChargeLimitEnabled(ctx) {
		testOptions.ChargeLimit = setup.DisableChargeLimit
	}
	// Make sure disable ui happens after ui-related setup.
	su.Add(setup.PowerTest(ctx, tconn, testOptions, setup.NewBatteryDischarge(discharge, false /*ignoreErr*/, setup.DefaultDischargeThreshold)))
	su.Add(setup.DisableServiceIfExists(ctx, "ui"))
	if err := su.Check(ctx); err != nil {
		s.Error("Setup check failed: ", err)
	}
}
