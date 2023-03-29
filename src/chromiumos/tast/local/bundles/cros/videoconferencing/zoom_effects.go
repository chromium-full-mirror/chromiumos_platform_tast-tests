// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package videoconferencing

import (
	"context"
	"time"

	"chromiumos/tast/ctxutil"
	"chromiumos/tast/local/bundles/cros/videoconferencing/commontype"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/apps/thirdparty/zoom"
	"chromiumos/tast/local/chrome/browser/browserfixt"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/chrome/uiauto/vctray"
	"chromiumos/tast/local/videoconferencing/fixture"
	"chromiumos/tast/testing"
	"chromiumos/tast/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ZoomEffects,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks Video Effects in Zoom conference",
		Contacts: []string{
			"chrome-knowledge-eng@google.com",
			"shengjun@chromium.org",
		},
		BugComponent: "b:187682",
		Timeout:      3 * time.Minute,
		Attr: []string{
			"group:mainline", "informational", "group:ml_service",
		},
		SoftwareDeps: []string{"chrome", "camera_feature_effects"},
		HardwareDeps: hwdep.D(hwdep.SkipOnModel("betty")),
		Params: []testing.Param{
			{
				Name:    "pwa",
				Fixture: fixture.GAIALoggedInWithFakeHALAndEffectsEnabled,
				Val:     commontype.LaunchAppInPWA,
			},
			{
				Name:    "web",
				Fixture: fixture.GAIALoggedInWithFakeHALAndEffectsEnabled,
				Val:     commontype.LaunchAppInWeb,
			},
			{
				Name:              "pwa_lacros",
				ExtraSoftwareDeps: []string{"lacros"},
				Fixture:           fixture.GAIALoggedInLacrosWithFakeHALAndEffectsEnabled,
				Val:               commontype.LaunchAppInPWA,
			},
			{
				Name:              "web_lacros",
				ExtraSoftwareDeps: []string{"lacros"},
				Fixture:           fixture.GAIALoggedInLacrosWithFakeHALAndEffectsEnabled,
				Val:               commontype.LaunchAppInWeb,
			},
		},
	})
}

func ZoomEffects(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui")

	browserType := s.FixtValue().(fixture.FixtData).BrowserType()

	br, cleanup, err := browserfixt.SetUp(ctx, cr, browserType)
	if err != nil {
		s.Fatal("Failed to launch browser: ", err)
	}
	defer cleanup(cleanupCtx)

	var zm *zoom.Zoom

	if s.Param().(commontype.LaunchAppType) == commontype.LaunchAppInPWA {
		zm, err = zoom.StartNewMeetingUsingPWA(ctx, cr, br, zoom.WithAllPermissions)
	} else {
		zm, err = zoom.StartNewMeeting(ctx, cr, br, zoom.WithAllPermissions)
	}
	if err != nil {
		s.Fatal("Failed to start meeting: ", err)
	}
	defer zm.Close(cleanupCtx)

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_zoom")

	if err := zm.SwitchVideo(true)(ctx); err != nil {
		s.Fatal("Failed to switch on camera: ", err)
	}

	vcTray := vctray.New(ctx, tconn)

	if err := uiauto.Combine("configure effects via mcpanel",
		vcTray.ExpandPanel,
		vcTray.SetBackgroundBlur(vctray.BackgroundBlurFull),
		vcTray.SwitchPortraitRelighting(),
		vcTray.CollapsePanel,
	)(ctx); err != nil {
		s.Fatal("Failed to configure effects: ", err)
	}

	if err := zm.EnterFullScreen(ctx); err != nil {
		s.Fatal("Failed to enter full screen: ", err)
	}
}
