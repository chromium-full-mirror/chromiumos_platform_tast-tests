// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package mlservice

import (
	"context"
	"time"

	"chromiumos/tast/ctxutil"
	"chromiumos/tast/local/apps/zoom"
	"chromiumos/tast/local/bundles/cros/mlservice/commontype"
	"chromiumos/tast/local/bundles/cros/mlservice/fixture"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/browser/browserfixt"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/testing"
	"chromiumos/tast/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         VCZoomEffects,
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
				Fixture: fixture.GAIALoggedInLacrosClamshellWithFakeHALAndEffectsEnabled,
				Val:     commontype.LaunchAppInPWA,
			},
			{
				Name:    "tablet_pwa_lacros",
				Fixture: fixture.GAIALoggedInLacrosTabletWithFakeHALAndEffectsEnabled,
				Val:     commontype.LaunchAppInPWA,
			},
			{
				Name:    "clamshell_web_lacros",
				Fixture: fixture.GAIALoggedInLacrosClamshellWithFakeHALAndEffectsEnabled,
				Val:     commontype.LaunchAppInWeb,
			},
			{
				Name:    "tablet_web_lacros",
				Fixture: fixture.GAIALoggedInLacrosTabletWithFakeHALAndEffectsEnabled,
				Val:     commontype.LaunchAppInWeb,
			},
		},
	})
}

func VCZoomEffects(ctx context.Context, s *testing.State) {
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

	if s.Param().(commontype.LaunchAppType) == commontype.LaunchAppInPWA {
		zm, err = zoom.StartNewMeetingUsingPWA(ctx, cr, br)
	} else {
		zm, err = zoom.StartNewMeeting(ctx, cr, br)
	}
	if err != nil {
		s.Fatal("Failed to start meeting: ", err)
	}
	defer zm.Close(cleanupCtx)

	if err := uiauto.NamedCombine("configure meeting",
		zm.SwitchVideo(true),
		zm.ChangeSettings(zm.SetBackgroundBlur),
		zm.EnterFullScreen,
	)(ctx); err != nil {
		s.Fatal("Failed to configure meeting: ", err)
	}
}
