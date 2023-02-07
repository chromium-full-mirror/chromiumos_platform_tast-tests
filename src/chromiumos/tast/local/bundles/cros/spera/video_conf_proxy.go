// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package spera

import (
	"context"
	"time"

	"chromiumos/tast/ctxutil"
	"chromiumos/tast/local/audio/crastestclient"
	"chromiumos/tast/local/bundles/cros/spera/videoconfproxy"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/cuj"
	"chromiumos/tast/local/chrome/display"
	"chromiumos/tast/local/ui/cujrecorder"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         VideoConfProxy,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "A test case that simulates the video call testing",
		Contacts:     []string{"jane.yang@cienet.com", "cienet-development@googlegroups.com"},
		BugComponent: "b:259504099",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		Vars: []string{
			"spera.cuj_mode",     // Optional. Expecting "tablet" or "clamshell".
			"spera.collectTrace", // Optional. Expecting "enable" or "disable", default is "disable".
		},
		Data: []string{cujrecorder.SystemTraceConfigFile},
		Params: []testing.Param{
			{
				Name:    "vp9_essential",
				Fixture: "loggedInAndKeepStateWithFakeCamera",
				Timeout: 5 * time.Minute,
				Val: videoconfproxy.TestParams{
					BrowserType:  browser.TypeAsh,
					VideoCallURL: cuj.VideoCallEssentialURL,
				},
			},
			{
				Name:    "vp9_advanced",
				Fixture: "loggedInAndKeepStateWithFakeCamera",
				Timeout: 5 * time.Minute,
				Val: videoconfproxy.TestParams{
					BrowserType:  browser.TypeAsh,
					VideoCallURL: cuj.VideoCallAdvancedURL,
				},
			},
			{
				Name:              "vp9_essential_lacros",
				Fixture:           "loggedInAndKeepStateLacrosWithFakeCamera",
				Timeout:           5 * time.Minute,
				ExtraSoftwareDeps: []string{"lacros"},
				Val: videoconfproxy.TestParams{
					BrowserType:  browser.TypeLacros,
					VideoCallURL: cuj.VideoCallEssentialURL,
				},
			},
			{
				Name:              "vp9_advanced_lacros",
				Fixture:           "loggedInAndKeepStateLacrosWithFakeCamera",
				Timeout:           5 * time.Minute,
				ExtraSoftwareDeps: []string{"lacros"},
				Val: videoconfproxy.TestParams{
					BrowserType:  browser.TypeLacros,
					VideoCallURL: cuj.VideoCallAdvancedURL,
				},
			},
			{
				Name:    "vp9_grid_essential",
				Fixture: "loggedInAndKeepStateWithFakeCamera",
				Timeout: 5 * time.Minute,
				Val: videoconfproxy.TestParams{
					BrowserType:  browser.TypeAsh,
					VideoCallURL: cuj.VideoCallGridEssentialURL,
				},
			},
			{
				Name:    "vp9_grid_advanced",
				Fixture: "loggedInAndKeepStateWithFakeCamera",
				Timeout: 5 * time.Minute,
				Val: videoconfproxy.TestParams{
					BrowserType:  browser.TypeAsh,
					VideoCallURL: cuj.VideoCallGridAdvancedURL,
				},
			},
			{
				Name:              "vp9_grid_essential_lacros",
				Fixture:           "loggedInAndKeepStateLacrosWithFakeCamera",
				Timeout:           5 * time.Minute,
				ExtraSoftwareDeps: []string{"lacros"},
				Val: videoconfproxy.TestParams{
					BrowserType:  browser.TypeLacros,
					VideoCallURL: cuj.VideoCallGridEssentialURL,
				},
			},
			{
				Name:              "vp9_grid_advanced_lacros",
				Fixture:           "loggedInAndKeepStateLacrosWithFakeCamera",
				Timeout:           5 * time.Minute,
				ExtraSoftwareDeps: []string{"lacros"},
				Val: videoconfproxy.TestParams{
					BrowserType:  browser.TypeLacros,
					VideoCallURL: cuj.VideoCallGridAdvancedURL,
				},
			},
		},
	})
}

func VideoConfProxy(ctx context.Context, s *testing.State) {
	p := s.Param().(videoconfproxy.TestParams)
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

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
	p.TabletMode = tabletMode
	p.OutDir = s.OutDir()

	if collect, ok := s.Var("spera.collectTrace"); ok && collect == "enable" {
		p.TraceConfigPath = s.DataPath(cujrecorder.SystemTraceConfigFile)
	}
	if err := videoconfproxy.Run(ctx, cr, p); err != nil {
		if err := crastestclient.DumpAudioDiagnostics(cleanupCtx, s.OutDir()); err != nil {
			s.Error("Failed to dump audio diagnostics: ", err)
		}
		s.Fatal("Failed to run video conference proxy cuj: ", err)
	}
}
