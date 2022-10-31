// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package spera

import (
	"context"
	"time"

	"chromiumos/tast/ctxutil"
	"chromiumos/tast/local/bundles/cros/spera/videoconfproxy"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/ash"
	"chromiumos/tast/local/chrome/browser"
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
					VideoCallURL: videoconfproxy.VP9720P30FPS,
				},
			},
			{
				Name:    "vp9_advanced",
				Fixture: "loggedInAndKeepStateWithFakeCamera",
				Timeout: 5 * time.Minute,
				Val: videoconfproxy.TestParams{
					BrowserType:  browser.TypeAsh,
					VideoCallURL: videoconfproxy.VP91080P30FPS,
				},
			},
			{
				Name:              "vp9_essential_lacros",
				Fixture:           "loggedInAndKeepStateLacrosWithFakeCamera",
				Timeout:           5 * time.Minute,
				ExtraSoftwareDeps: []string{"lacros"},
				Val: videoconfproxy.TestParams{
					BrowserType:  browser.TypeLacros,
					VideoCallURL: videoconfproxy.VP9720P30FPS,
				},
			},
			{
				Name:              "vp9_advanced_lacros",
				Fixture:           "loggedInAndKeepStateLacrosWithFakeCamera",
				Timeout:           5 * time.Minute,
				ExtraSoftwareDeps: []string{"lacros"},
				Val: videoconfproxy.TestParams{
					BrowserType:  browser.TypeLacros,
					VideoCallURL: videoconfproxy.VP91080P30FPS,
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
	p.TabletMode = tabletMode
	p.OutDir = s.OutDir()
	if collect, ok := s.Var("spera.collectTrace"); ok && collect == "enable" {
		p.TraceConfigPath = s.DataPath(cujrecorder.SystemTraceConfigFile)
	}
	if err := videoconfproxy.Run(ctx, cr, p); err != nil {
		s.Fatal("Failed to run video conference proxy cuj: ", err)
	}
}
