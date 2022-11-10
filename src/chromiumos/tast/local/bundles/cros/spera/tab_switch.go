// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package spera

import (
	"context"
	"time"

	"chromiumos/tast/ctxutil"
	"chromiumos/tast/local/bundles/cros/spera/tabswitch"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/ash"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/display"
	"chromiumos/tast/local/ui/cujrecorder"
	"chromiumos/tast/testing"
)

type tabSwitchParam struct {
	level       tabswitch.Level
	wprProxy    bool
	browserType browser.Type
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         TabSwitch,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Measures the performance of tab-switching, scrolling content with trackpad",
		Contacts:     []string{"abergman@google.com", "tclaiborne@chromium.org", "xliu@cienet.com", "alfredyu@cienet.com"},
		SoftwareDeps: []string{"chrome"},
		Vars: []string{
			"spera.cuj_mute",
			"spera.cuj_mode",     // Expecting "tablet" or "clamshell".
			"spera.collectTrace", // Optional. Expecting "enable" or "disable", default is "disable".
			// WPR addresses are only required when running with WPR Proxy.
			"ui.wpr_http_addr",
			"ui.wpr_https_addr",
		},
		Data: []string{cujrecorder.SystemTraceConfigFile},
		Params: []testing.Param{
			{
				Name:              "essential",
				Timeout:           35 * time.Minute,
				Val:               tabSwitchParam{level: tabswitch.Essential, wprProxy: false},
				Fixture:           "loggedInAndKeepState",
				ExtraSoftwareDeps: []string{"arc"},
			}, {
				Name:              "essential_lacros",
				Timeout:           35 * time.Minute,
				Val:               tabSwitchParam{level: tabswitch.Essential, wprProxy: false, browserType: browser.TypeLacros},
				Fixture:           "loggedInAndKeepStateLacros",
				ExtraSoftwareDeps: []string{"lacros", "arc"},
			}, {
				Name:              "advanced",
				Timeout:           45 * time.Minute,
				Val:               tabSwitchParam{level: tabswitch.Advanced, wprProxy: false},
				Fixture:           "loggedInAndKeepState",
				ExtraSoftwareDeps: []string{"arc"},
			}, {
				Name:              "advanced_lacros",
				Timeout:           45 * time.Minute,
				Val:               tabSwitchParam{level: tabswitch.Advanced, wprProxy: false, browserType: browser.TypeLacros},
				Fixture:           "loggedInAndKeepStateLacros",
				ExtraSoftwareDeps: []string{"lacros", "arc"},
			},
		},
	})
}

// TabSwitch measures the performance of tab switching.
//
// WPR server should be running in a remote server. TabSwitchRecorder case can be used to record
// WPR content for this test in the remote server.
func TabSwitch(ctx context.Context, s *testing.State) {
	p := s.Param().(tabSwitchParam)

	var cr *chrome.Chrome
	if !p.wprProxy {
		cr = s.FixtValue().(chrome.HasChrome).Chrome()
	} else {
		cr = s.PreValue().(*chrome.Chrome)
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 3*time.Second)
	defer cancel()

	var tabletMode bool
	if mode, ok := s.Var("spera.cuj_mode"); ok {
		tabletMode = mode == "tablet"
		cleanup, err := ash.EnsureTabletModeEnabled(ctx, tconn, tabletMode)
		if err != nil {
			s.Fatalf("Failed to enable tablet mode to %v: %v", tabletMode, err)
		}
		defer cleanup(cleanupCtx)
	} else {
		// Use default screen mode of the DUT.
		tabletMode, err = ash.TabletModeEnabled(ctx, tconn)
		if err != nil {
			s.Fatal("Failed to get DUT default screen mode: ", err)
		}
	}
	s.Log("Running test with tablet mode: ", tabletMode)
	if tabletMode {
		cleanup, err := display.RotateToLandscape(ctx, tconn)
		if err != nil {
			s.Fatal("Failed to rotate display to landscape: ", err)
		}
		defer cleanup(cleanupCtx)
	}

	// Shorten context a bit to allow for cleanup if Run fails.
	ctx, cancel = ctxutil.Shorten(ctx, 3*time.Second)
	defer cancel()
	tabswitch.Run(ctx, s, cr, p.level, tabletMode, p.browserType)
}
