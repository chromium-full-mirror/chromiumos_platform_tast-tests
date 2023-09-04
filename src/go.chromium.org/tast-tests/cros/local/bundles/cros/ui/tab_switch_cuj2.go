// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/ui/tabswitchcuj"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"
	"go.chromium.org/tast-tests/cros/local/wpr"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

type tabSwitchParam struct {
	tier        cuj.Tier
	wprProxy    bool
	browserType browser.Type
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         TabSwitchCUJ2,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Measures the performance of tab-switching CUJ, scrolling content with trackpad",
		Contacts:     []string{"chromeos-perf-reliability-eng@google.com", "cienet-development@googlegroups.com", "alstonhuang@google.com"},
		BugComponent: "b:1025042", // ChromeOS > EngProd > Platform > SPERA > Automation
		SoftwareDeps: []string{"chrome"},
		Vars: []string{
			"ui.cuj_mute",
			"ui.cuj_mode",     // Expecting "tablet" or "clamshell".
			"ui.collectTrace", // Optional. Expecting "enable" or "disable", default is "disable".
			// WPR addresses are only required when running with WPR Proxy.
			"ui.wpr_http_addr",
			"ui.wpr_https_addr",
		},
		Data: []string{cujrecorder.SystemTraceConfigFile},
		Params: []testing.Param{
			{
				Name:    "basic",
				Timeout: 30 * time.Minute,
				Val:     tabSwitchParam{tier: cuj.Basic, wprProxy: true},
				Pre:     wpr.RemoteReplayMode(),
			}, {
				Name:    "plus",
				Timeout: 35 * time.Minute,
				Val:     tabSwitchParam{tier: cuj.Plus, wprProxy: true},
				Pre:     wpr.RemoteReplayMode(),
			}, {
				Name:    "premium",
				Timeout: 40 * time.Minute,
				Val:     tabSwitchParam{tier: cuj.Premium, wprProxy: true},
				Pre:     wpr.RemoteReplayMode(),
			}, {
				Name:              "basic_noproxy",
				Timeout:           35 * time.Minute,
				Val:               tabSwitchParam{tier: cuj.Basic, wprProxy: false},
				Fixture:           "loggedInAndKeepState",
				ExtraSoftwareDeps: []string{"arc"},
			}, {
				Name:              "basic_lacros_noproxy",
				Timeout:           35 * time.Minute,
				Val:               tabSwitchParam{tier: cuj.Basic, wprProxy: false, browserType: browser.TypeLacros},
				Fixture:           "loggedInAndKeepStateLacros",
				ExtraSoftwareDeps: []string{"lacros", "arc"},
			}, {
				Name:              "plus_noproxy",
				Timeout:           40 * time.Minute,
				Val:               tabSwitchParam{tier: cuj.Plus, wprProxy: false},
				Fixture:           "loggedInAndKeepState",
				ExtraSoftwareDeps: []string{"arc"},
			}, {
				Name:              "plus_lacros_noproxy",
				Timeout:           40 * time.Minute,
				Val:               tabSwitchParam{tier: cuj.Plus, wprProxy: false, browserType: browser.TypeLacros},
				Fixture:           "loggedInAndKeepStateLacros",
				ExtraSoftwareDeps: []string{"lacros", "arc"},
			}, {
				Name:    "premium_noproxy",
				Timeout: 45 * time.Minute,
				Val:     tabSwitchParam{tier: cuj.Premium, wprProxy: false},

				Fixture:           "loggedInAndKeepState",
				ExtraSoftwareDeps: []string{"arc"},
			}, {
				Name:    "premium_lacros_noproxy",
				Timeout: 45 * time.Minute,
				Val:     tabSwitchParam{tier: cuj.Premium, wprProxy: false, browserType: browser.TypeLacros},

				Fixture:           "loggedInAndKeepStateLacros",
				ExtraSoftwareDeps: []string{"lacros", "arc"},
			},
		},
	})
}

// TabSwitchCUJ2 measures the performance of tab-switching CUJ.
//
// WPR server should be running in a remote server. TabSwitchCUJRecorder2 case can be used to record
// WPR content for this test in the remote server.
func TabSwitchCUJ2(ctx context.Context, s *testing.State) {
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

	// Shorten context a bit to allow for cleanup if Run fails.
	ctx, cancel = ctxutil.Shorten(ctx, 3*time.Second)
	defer cancel()
	tabswitchcuj.Run2(ctx, s, cr, p.tier, p.browserType, tabletMode, false)
}
