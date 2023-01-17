// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package mlservice

import (
	"context"
	"time"

	"chromiumos/tast/common/action"
	"chromiumos/tast/ctxutil"
	"chromiumos/tast/local/apps/googlemeet"
	"chromiumos/tast/local/bundles/cros/mlservice/fixture"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         VCMeet,
		LacrosStatus: testing.LacrosVariantNeeded,
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
				Val:     true,
			},
			{
				Name:    "tablet_pwa",
				Fixture: fixture.GAIALoggedInTablet,
				Val:     true,
			},
			{
				Name:    "clamshell_web",
				Fixture: fixture.GAIALoggedInClamshell,
				Val:     false,
			},
			{
				Name:    "tablet_web",
				Fixture: fixture.GAIALoggedInTablet,
				Val:     false,
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

	var gm *googlemeet.GoogleMeet
	var err error
	var cleanup action.Action

	if s.Param().(bool) {
		gm, cleanup, err = googlemeet.StartNewMeetingUsingPWA(ctx, cr, browserType)
	} else {
		// Meet can dynamically switch between different segmentation models.
		// Force the same model the platform effects use with the experiment ?e=ForceSegmentationModelVariant::GpuMid.
		gm, cleanup, err = googlemeet.StartNewMeeting(ctx, cr, browserType,
			map[string]string{
				"e": "ForceSegmentationModelVariant::GpuMid",
			})
	}
	if err != nil {
		s.Fatal("Failed to start meeting: ", err)
	}
	defer cleanup(cleanupCtx)

	sendResolutionName := "High definition (720p)"

	if err := uiauto.Combine("configure Meet",
		gm.EnterFullScreen,
		gm.MuteIfMicAvailable,
		gm.ChangeSettings(
			gm.SetLeaveEmptyCalls(false),
			gm.SetAdjustVideoLighting(true),
			gm.SetSendResolution(sendResolutionName),
		),
		gm.ApplyVideoEffects(gm.SetEffectBlur(true)),
	)(ctx); err != nil {
		s.Fatal("Failed to configure Meet: ", err)
	}
}
