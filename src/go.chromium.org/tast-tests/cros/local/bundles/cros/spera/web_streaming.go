// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package spera

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/spera/webstreaming"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         WebStreaming,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "A test case that simulates the web streaming testing",
		Contacts:     []string{"chromeos-perf-reliability-eng@google.com", "cienet-development@googlegroups.com", "chicheny@google.com"},
		BugComponent: "b:1025042", // ChromeOS > EngProd > Platform > SPERA > Automation
		SoftwareDeps: []string{"chrome"},
		Vars: []string{
			"spera.cuj_mode",     // Optional. Expecting "tablet" or "clamshell".
			"spera.collectTrace", // Optional. Expecting "enable" or "disable", default is "disable".
		},
		Data: []string{cujrecorder.SystemTraceConfigFile},
		Params: []testing.Param{
			{
				Name:    "h264_essential",
				Fixture: "loggedInAndKeepState",
				Timeout: 20 * time.Minute,
				Val: webstreaming.TestParams{
					BrowserType: browser.TypeAsh,
					VideoOption: webstreaming.H264DASH1080P60FPS,
				},
			},
			{
				Name:    "h264_advanced",
				Fixture: "loggedInAndKeepState",
				Timeout: 20 * time.Minute,
				Val: webstreaming.TestParams{
					BrowserType: browser.TypeAsh,
					VideoOption: webstreaming.H264DASH4K60FPS,
				},
			},
			{
				Name:              "h265_essential",
				Fixture:           "loggedInAndKeepState",
				Timeout:           20 * time.Minute,
				ExtraSoftwareDeps: []string{caps.HWDecodeHEVC60},
				Val: webstreaming.TestParams{
					BrowserType: browser.TypeAsh,
					VideoOption: webstreaming.HEVC1080P60FPS,
				},
			},
			{
				Name:              "h265_advanced",
				Fixture:           "loggedInAndKeepState",
				Timeout:           20 * time.Minute,
				ExtraSoftwareDeps: []string{caps.HWDecodeHEVC4K60},
				Val: webstreaming.TestParams{
					BrowserType: browser.TypeAsh,
					VideoOption: webstreaming.HEVC4K60FPS,
				},
			},
			{
				Name:    "vp9_essential",
				Fixture: "loggedInAndKeepState",
				Timeout: 20 * time.Minute,
				Val: webstreaming.TestParams{
					BrowserType: browser.TypeAsh,
					VideoOption: webstreaming.VP9DASH1080P60FPS,
				},
			},
			{
				Name:    "vp9_advanced",
				Fixture: "loggedInAndKeepState",
				Timeout: 20 * time.Minute,
				Val: webstreaming.TestParams{
					BrowserType: browser.TypeAsh,
					VideoOption: webstreaming.VP9DASH4K60FPS,
				},
			},
			{
				Name:    "av1_advanced",
				Fixture: "loggedInAndKeepState",
				Timeout: 20 * time.Minute,
				Val: webstreaming.TestParams{
					BrowserType: browser.TypeAsh,
					VideoOption: webstreaming.AV1DASH60FPS,
				},
			},
			{
				Name:              "h264_essential_lacros",
				Fixture:           "loggedInAndKeepStateLacros",
				Timeout:           20 * time.Minute,
				ExtraSoftwareDeps: []string{"lacros"},
				Val: webstreaming.TestParams{
					BrowserType: browser.TypeLacros,
					VideoOption: webstreaming.H264DASH1080P60FPS,
				},
			},
			{
				Name:              "h264_advanced_lacros",
				Fixture:           "loggedInAndKeepStateLacros",
				Timeout:           20 * time.Minute,
				ExtraSoftwareDeps: []string{"lacros"},
				Val: webstreaming.TestParams{
					BrowserType: browser.TypeLacros,
					VideoOption: webstreaming.H264DASH4K60FPS,
				},
			},
			{
				Name:              "h265_essential_lacros",
				Fixture:           "loggedInAndKeepStateLacros",
				Timeout:           20 * time.Minute,
				ExtraSoftwareDeps: []string{"lacros", caps.HWDecodeHEVC60},
				Val: webstreaming.TestParams{
					BrowserType: browser.TypeLacros,
					VideoOption: webstreaming.HEVC1080P60FPS,
				},
			},
			{
				Name:              "h265_advanced_lacros",
				Fixture:           "loggedInAndKeepStateLacros",
				Timeout:           20 * time.Minute,
				ExtraSoftwareDeps: []string{"lacros", caps.HWDecodeHEVC4K60},
				Val: webstreaming.TestParams{
					BrowserType: browser.TypeLacros,
					VideoOption: webstreaming.HEVC4K60FPS,
				},
			},
			{
				Name:              "vp9_essential_lacros",
				Fixture:           "loggedInAndKeepStateLacros",
				Timeout:           20 * time.Minute,
				ExtraSoftwareDeps: []string{"lacros"},
				Val: webstreaming.TestParams{
					BrowserType: browser.TypeLacros,
					VideoOption: webstreaming.VP9DASH1080P60FPS,
				},
			},
			{
				Name:              "vp9_advanced_lacros",
				Fixture:           "loggedInAndKeepStateLacros",
				Timeout:           20 * time.Minute,
				ExtraSoftwareDeps: []string{"lacros"},
				Val: webstreaming.TestParams{
					BrowserType: browser.TypeLacros,
					VideoOption: webstreaming.VP9DASH4K60FPS,
				},
			},
			{
				Name:              "av1_advanced_lacros",
				Fixture:           "loggedInAndKeepStateLacros",
				Timeout:           20 * time.Minute,
				ExtraSoftwareDeps: []string{"lacros"},
				Val: webstreaming.TestParams{
					BrowserType: browser.TypeLacros,
					VideoOption: webstreaming.AV1DASH60FPS,
				},
			},
		},
	})
}

func WebStreaming(ctx context.Context, s *testing.State) {
	p := s.Param().(webstreaming.TestParams)
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
	traceConfigPath := ""
	if collect, ok := s.Var("spera.collectTrace"); ok && collect == "enable" {
		traceConfigPath = s.DataPath(cujrecorder.SystemTraceConfigFile)
	}
	if err := webstreaming.Run(ctx, cr, s.OutDir(), traceConfigPath, tabletMode, p.BrowserType, p.VideoOption); err != nil {
		s.Fatal("Failed to run web streaming test: ", err)
	}
}
