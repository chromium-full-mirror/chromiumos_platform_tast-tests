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
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/cuj"
	"chromiumos/tast/local/chrome/display"
	"chromiumos/tast/local/ui/cujrecorder"
	"chromiumos/tast/testing"
)

type tabSwitchParam struct {
	tier        cuj.Tier
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
			"spera.web_source",   // Optional. Expecting "google" or "external", default is "external".
			// WPR addresses are only required when running with WPR Proxy.
			"ui.wpr_http_addr",
			"ui.wpr_https_addr",
		},
		Data: []string{cujrecorder.SystemTraceConfigFile},
		Params: []testing.Param{
			{
				Name:    "essential",
				Timeout: 35 * time.Minute,
				Val: tabSwitchParam{
					tier:     cuj.Essential,
					wprProxy: false,
				},
				Fixture:           "loggedInAndKeepState",
				ExtraSoftwareDeps: []string{"arc"},
			}, {
				Name:    "essential_lacros",
				Timeout: 35 * time.Minute,
				Val: tabSwitchParam{
					tier:        cuj.Essential,
					wprProxy:    false,
					browserType: browser.TypeLacros,
				},
				Fixture:           "loggedInAndKeepStateLacros",
				ExtraSoftwareDeps: []string{"lacros", "arc"},
			}, {
				Name:    "advanced",
				Timeout: 45 * time.Minute,
				Val: tabSwitchParam{
					tier:     cuj.Advanced,
					wprProxy: false,
				},
				Fixture:           "loggedInAndKeepState",
				ExtraSoftwareDeps: []string{"arc"},
			}, {
				Name:    "advanced_lacros",
				Timeout: 45 * time.Minute,
				Val: tabSwitchParam{
					tier:        cuj.Advanced,
					wprProxy:    false,
					browserType: browser.TypeLacros,
				},
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

	tabletMode, resetTabletMode, err := cuj.EnableTabletMode(ctx, tconn, s.Var, "spera.cuj_mode")
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

	// Shorten context a bit to allow for cleanup if Run fails.
	ctx, cancel = ctxutil.Shorten(ctx, 3*time.Second)
	defer cancel()
	tabswitch.Run(ctx, s, cr, p.tier, p.browserType, tabletMode, false)
}
