// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/ui/quickcheckcuj"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type quickCheckParam struct {
	tier        cuj.Tier
	scenario    quickcheckcuj.PauseMode
	browserType browser.Type
}

func init() {
	testing.AddTest(&testing.Test{
		// TODO (b/242590511): Deprecated after moving all performance cuj test cases to go.chromium.org/tast-tests/cros/local/bundles/cros/spera directory.
		Func:         QuickCheckCUJ2,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Measures the system performance after login or wakeup by checking common apps",
		Contacts:     []string{"chromeos-perf-reliability-eng@google.com", "cienet-development@googlegroups.com", "alstonhuang@google.com"},
		BugComponent: "b:1025042", // ChromeOS > EngProd > Platform > SPERA > Automation
		SoftwareDeps: []string{"chrome", "arc", "wifi"},
		HardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		Vars: []string{
			"ui.cuj_mode",                 // Optional. Expecting "tablet" or "clamshell".
			"ui.collectTrace",             // Optional. Expecting "enable" or "disable", default is "disable".
			"ui.QuickCheckCUJ2_wait_time", // Optional. Given time for the system to stablize in seconds.
			"ui.QuickCheckCUJ2_wifissid",
			"ui.QuickCheckCUJ2_wifipassword",
		},
		Data: []string{cujrecorder.SystemTraceConfigFile},
		Params: []testing.Param{
			{
				Name:    "basic_unlock",
				Fixture: "loggedInAndKeepState",
				Timeout: 5 * time.Minute,
				Val: quickCheckParam{
					tier:     cuj.Basic,
					scenario: quickcheckcuj.Lock,
				},
			}, {
				Name:              "basic_lacros_unlock",
				Fixture:           "loggedInAndKeepStateLacros",
				Timeout:           5 * time.Minute,
				ExtraSoftwareDeps: []string{"lacros"},
				Val: quickCheckParam{
					tier:        cuj.Basic,
					scenario:    quickcheckcuj.Lock,
					browserType: browser.TypeLacros,
				},
			}, {
				Name:    "basic_wakeup",
				Fixture: "loggedInAndKeepState",
				Timeout: 5 * time.Minute,
				Val: quickCheckParam{
					tier:     cuj.Basic,
					scenario: quickcheckcuj.Suspend,
				},
			}, {
				Name:              "basic_lacros_wakeup",
				Fixture:           "loggedInAndKeepStateLacros",
				Timeout:           5 * time.Minute,
				ExtraSoftwareDeps: []string{"lacros"},
				Val: quickCheckParam{
					tier:        cuj.Basic,
					scenario:    quickcheckcuj.Suspend,
					browserType: browser.TypeLacros,
				},
			},
		},
	})
}

// QuickCheckCUJ2 measures the system performance after login or wakeup by checking common apps
func QuickCheckCUJ2(ctx context.Context, s *testing.State) {
	// The system may be unstable after login, causing suspending DUT operations to fail on some models.
	// Therefore, use a variable to control the startup delay time and find out the estimated time required by the model.
	// See b/234115114 for details.
	if wt, ok := s.Var("ui.QuickCheckCUJ2_wait_time"); ok {
		waitTime, err := strconv.Atoi(wt)
		if err != nil {
			s.Fatal("Failed to convert the ui.QuickCheckCUJ2_wait_time to integer: ", err)
		}
		s.Logf("Given %d seconds for the system to stablize", waitTime)
		// GoBigSleepLint: Wait for the system to stabilize.
		if err := testing.Sleep(ctx, time.Duration(waitTime)*time.Second); err != nil {
			s.Fatalf("Failed to sleep for %d seconds: %v", waitTime, err)
		}
	}

	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	// Shorten context a bit to allow for cleanup if Run fails.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 3*time.Second)
	defer cancel()

	tabletMode, resetTabletMode, err := cuj.EnableTabletMode(ctx, tconn, s.Var, "ui.cuj_mode")
	if err != nil {
		s.Fatal("Failed to enable tablet mode: ", err)
	}
	defer resetTabletMode(cleanupCtx)

	if tabletMode {
		cleanup, err := display.RotateToLandscape(ctx, tconn)
		if err != nil {
			s.Fatal("Failed to rotate display to landscape: ", err)
		}
		defer cleanup(cleanupCtx)
	}

	param := s.Param().(quickCheckParam)
	scenario := param.scenario

	pv := quickcheckcuj.Run(ctx, s, cr, scenario, tabletMode, param.browserType)
	if err := pv.Save(s.OutDir()); err != nil {
		s.Fatal("Failed to saving perf data: ", err)
	}
}
