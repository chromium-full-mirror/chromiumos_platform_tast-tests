// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package videoconferencing

import (
	"context"
	"time"

	"chromiumos/tast/ctxutil"
	"chromiumos/tast/local/apps/googlemeet"
	"chromiumos/tast/local/bundles/cros/videoconferencing/commontype"
	"chromiumos/tast/local/bundles/cros/videoconferencing/fixture"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/browser/browserfixt"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/chrome/uiauto/vctray"
	"chromiumos/tast/testing"
	"chromiumos/tast/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         MeetEffects,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks Video Effects in Google Meet",
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
				Name:    "clamshell_pwa",
				Fixture: fixture.GAIALoggedInClamshellWithFakeHALAndEffectsEnabled,
				Val:     commontype.LaunchAppInPWA,
			},
			{
				Name:    "tablet_pwa",
				Fixture: fixture.GAIALoggedInTabletWithFakeHALAndEffectsEnabled,
				Val:     commontype.LaunchAppInPWA,
			},
			{
				Name:    "clamshell_web",
				Fixture: fixture.GAIALoggedInClamshellWithFakeHALAndEffectsEnabled,
				Val:     commontype.LaunchAppInWeb,
			},
			{
				Name:    "tablet_web",
				Fixture: fixture.GAIALoggedInTabletWithFakeHALAndEffectsEnabled,
				Val:     commontype.LaunchAppInWeb,
			},
			{
				Name:    "clamshell_pwa_lacros",
				Fixture: fixture.GAIALoggedInClamshellWithFakeHALAndEffectsEnabled,
				Val:     commontype.LaunchAppInPWA,
			},
			{
				Name:    "tablet_pwa_lacros",
				Fixture: fixture.GAIALoggedInTabletWithFakeHALAndEffectsEnabled,
				Val:     commontype.LaunchAppInPWA,
			},
			{
				Name:    "clamshell_web_lacros",
				Fixture: fixture.GAIALoggedInClamshellWithFakeHALAndEffectsEnabled,
				Val:     commontype.LaunchAppInWeb,
			},
			{
				Name:    "tablet_web_lacros",
				Fixture: fixture.GAIALoggedInTabletWithFakeHALAndEffectsEnabled,
				Val:     commontype.LaunchAppInWeb,
			},
		},
	})
}

func MeetEffects(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui")

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}

	browserType := s.FixtValue().(fixture.BaseSetupFixtData).BrowserType()

	br, cleanup, err := browserfixt.SetUp(ctx, cr, browserType)
	if err != nil {
		s.Fatal("Failed to launch browser: ", err)
	}
	defer cleanup(cleanupCtx)

	if err := googlemeet.GrantPermissions(ctx, br); err != nil {
		s.Fatal("Failed to grant permissions to Meet: ", err)
	}

	var gm *googlemeet.GoogleMeet

	if s.Param().(commontype.LaunchAppType) == commontype.LaunchAppInPWA {
		gm, err = googlemeet.StartNewMeetingUsingPWA(ctx, cr, br)
	} else {
		// Meet can dynamically switch between different segmentation models.
		// Force the same model the platform effects use with the experiment ?e=ForceSegmentationModelVariant::GpuMid.
		gm, err = googlemeet.StartNewMeeting(ctx, cr, br,
			map[string]string{
				"e": "ForceSegmentationModelVariant::GpuMid",
			})
	}
	if err != nil {
		s.Fatal("Failed to start meeting: ", err)
	}
	defer gm.Close(cleanupCtx)

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_with_meet")

	if err := uiauto.Combine("configure Meet",
		gm.MuteIfMicAvailable,
		gm.SwitchVideo(true),
	)(ctx); err != nil {
		s.Fatal("Failed to configure Meet: ", err)
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

	if err := gm.EnterFullScreen(ctx); err != nil {
		s.Fatal("Failed to enter full screen: ", err)
	}
}
