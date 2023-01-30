// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package mlservice

import (
	"context"
	"time"

	"chromiumos/tast/ctxutil"
	"chromiumos/tast/local/apps/googlemeet"
	"chromiumos/tast/local/bundles/cros/mlservice/commontype"
	"chromiumos/tast/local/bundles/cros/mlservice/fixture"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/browser/browserfixt"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         VCMeet,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks Video Effects in Google Meet",
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
				Val:     commontype.LaunchAppInPWA,
			},
			{
				Name:    "tablet_pwa",
				Fixture: fixture.GAIALoggedInTablet,
				Val:     commontype.LaunchAppInPWA,
			},
			{
				Name:    "clamshell_web",
				Fixture: fixture.GAIALoggedInClamshell,
				Val:     commontype.LaunchAppInWeb,
			},
			{
				Name:    "tablet_web",
				Fixture: fixture.GAIALoggedInTablet,
				Val:     commontype.LaunchAppInWeb,
			},
			{
				Name:    "clamshell_pwa_lacros",
				Fixture: fixture.GAIALoggedInLacrosClamshell,
				Val:     commontype.LaunchAppInPWA,
			},
			{
				Name:    "tablet_pwa_lacros",
				Fixture: fixture.GAIALoggedInLacrosTablet,
				Val:     commontype.LaunchAppInPWA,
			},
			{
				Name:    "clamshell_web_lacros",
				Fixture: fixture.GAIALoggedInLacrosClamshell,
				Val:     commontype.LaunchAppInWeb,
			},
			{
				Name:    "tablet_web_lacros",
				Fixture: fixture.GAIALoggedInLacrosTablet,
				Val:     commontype.LaunchAppInWeb,
			},
		},
	})
}

func VCMeet(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui")

	browserType := s.FixtValue().(fixture.BaseSetupFixtData).BrowserType()

	br, cleanup, err := browserfixt.SetUp(ctx, cr, browserType)
	if err != nil {
		s.Fatal("Failed to launch browser: ", err)
	}
	defer cleanup(cleanupCtx)

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

	sendResolutionName := "High definition (720p)"

	if err := uiauto.Combine("configure Meet",
		gm.EnterFullScreen,
		gm.MuteIfMicAvailable,
		gm.ChangeSettings(
			gm.SetLeaveEmptyCalls(false),
			// Video lighting option is not available on Lacros due to http://b/265954612.
			gm.SetAdjustVideoLighting(true),
			gm.SetSendResolution(sendResolutionName),
		),
		// Video effects are not supported on Lacros on VM due to http://b/265954612.
		gm.ApplyVideoEffects(gm.SetEffectBlur(true)),
	)(ctx); err != nil {
		s.Fatal("Failed to configure Meet: ", err)
	}

	if _, err := gm.ScreenshotCanvas(ctx, cr); err != nil {
		s.Fatal("Failed to take screenshot of canvas: ", err)
	}
}
