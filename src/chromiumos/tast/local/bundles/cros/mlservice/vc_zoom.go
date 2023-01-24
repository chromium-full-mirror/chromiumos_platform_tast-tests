// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package mlservice

import (
	"context"
	"time"

	"chromiumos/tast/ctxutil"
	"chromiumos/tast/local/apps/zoom"
	"chromiumos/tast/local/bundles/cros/mlservice/fixture"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/browser/browserfixt"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/testing"
)

type launchZoomType int

const (
	launchZoomWithWeb launchZoomType = iota
	launchZoomWithPWA
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         VCZoom,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks Video Effects in Zoom conference",
		Contacts: []string{
			"chrome-knowledge-eng@google.com",
			"shengjun@chromium.org",
		},
		BugComponent: "b:187682",
		Timeout:      3 * time.Minute,
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{
			{
				Name:    "clamshell_pwa",
				Fixture: fixture.GAIALoggedInClamshell,
				Val:     launchZoomWithPWA,
			},
			{
				Name:    "tablet_pwa",
				Fixture: fixture.GAIALoggedInTablet,
				Val:     launchZoomWithPWA,
			},
			{
				Name:    "clamshell_web",
				Fixture: fixture.GAIALoggedInClamshell,
				Val:     launchZoomWithWeb,
			},
			{
				Name:    "tablet_web",
				Fixture: fixture.GAIALoggedInTablet,
				Val:     launchZoomWithWeb,
			},
			{
				Name:    "clamshell_pwa_lacros",
				Fixture: fixture.GAIALoggedInLacrosClamshell,
				Val:     launchZoomWithPWA,
			},
			{
				Name:    "tablet_pwa_lacros",
				Fixture: fixture.GAIALoggedInLacrosTablet,
				Val:     launchZoomWithPWA,
			},
			{
				Name:    "clamshell_web_lacros",
				Fixture: fixture.GAIALoggedInLacrosClamshell,
				Val:     launchZoomWithWeb,
			},
			{
				Name:    "tablet_web_lacros",
				Fixture: fixture.GAIALoggedInLacrosTablet,
				Val:     launchZoomWithWeb,
			},
		},
	})
}

func VCZoom(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui")

	browserType := s.FixtValue().(fixture.BaseSetupFixtData).BrowserType()

	br, cleanup, err := browserfixt.SetUp(ctx, cr, browserType)
	if err != nil {
		s.Fatal("Failed to launch browser: ", err)
	}
	defer cleanup(cleanupCtx)

	var zm *zoom.Zoom

	if s.Param().(launchZoomType) == launchZoomWithPWA {
		zm, err = zoom.StartNewMeetingUsingPWA(ctx, cr, br)
	} else {
		zm, err = zoom.StartNewMeeting(ctx, cr, br)
	}
	if err != nil {
		s.Fatal("Failed to start meeting: ", err)
	}
	defer zm.Close(cleanupCtx)

	if err := zm.EnterFullScreen(ctx); err != nil {
		s.Fatal("Failed to enter full screen: ", err)
	}

	if err := zm.ExitFullScreen(ctx); err != nil {
		s.Fatal("Failed to exit full screen: ", err)
	}
}
