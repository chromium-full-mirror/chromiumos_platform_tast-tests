// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package videoconferencing

import (
	"context"
	"strings"
	"time"

	"chromiumos/tast/local/bundles/cros/videoconferencing/common"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/apps/thirdparty/googlemeet"
	"chromiumos/tast/local/chrome/ash"
	"chromiumos/tast/local/chrome/browser/browserfixt"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/chrome/uiauto/vctray"
	"chromiumos/tast/local/screenshot"
	"chromiumos/tast/local/videoconferencing/fixture"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
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
		Timeout:      10 * time.Minute,
		Attr: []string{
			"group:camera_dependent",
			"group:external-dependency",
			"group:video_conference",
			"video_conference_per_build",
		},
		SoftwareDeps: []string{"chrome", "camera_feature_effects"},
		HardwareDeps: hwdep.D(hwdep.SkipOnModel("betty")),
		SearchFlags: []*testing.StringPair{
			{
				// Enable background blur.
				Key:   "feature_id",
				Value: "screenplay-895a7022-c19e-42b9-ad7c-c3e4a0693210",
			},
			{
				// Lighting improvement on visible person.
				Key:   "feature_id",
				Value: "screenplay-b36fac8e-b0d6-4ba4-9c1d-fa172326816d",
			},
			{
				// Blur and lightning improvement.
				Key:   "feature_id",
				Value: "screenplay-1f125a69-4a47-4156-8cb5-97bc6244016b",
			},
		},
		Params: []testing.Param{
			{
				Name:    "pwa",
				Fixture: fixture.GAIALoggedInWithFakeHALAndEffectsEnabled,
				Val:     common.LaunchAppInPWA,
			},
			{
				Name:    "web",
				Fixture: fixture.GAIALoggedInWithFakeHALAndEffectsEnabled,
				Val:     common.LaunchAppInWeb,
			},
			{
				Name:              "pwa_lacros",
				ExtraSoftwareDeps: []string{"lacros"},
				Fixture:           fixture.GAIALoggedInLacrosWithFakeHALAndEffectsEnabled,
				Val:               common.LaunchAppInPWA,
			},
			{
				Name:              "web_lacros",
				ExtraSoftwareDeps: []string{"lacros"},
				Fixture:           fixture.GAIALoggedInLacrosWithFakeHALAndEffectsEnabled,
				Val:               common.LaunchAppInWeb,
			},
		},
		// Each parameterized test contains multiple subtests.
		// Using -var "subtests" to make it possible limiting the subtests to run.
		// e.g. tast run -var=subtests=backgroundblur_off_portraitrelighting_off
		// <dut> videoconferencing.MeetEffects.clamshell_web
		Vars: append(screenshot.ScreenDiffVars, "subtests"),
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

	browserType := s.FixtValue().(fixture.FixtData).BrowserType()

	conn, br, cleanup, err := browserfixt.SetUpWithURL(ctx, cr, browserType, chrome.NewTabURL)
	if err != nil {
		s.Fatal("Failed to launch browser: ", err)
	}
	defer cleanup(cleanupCtx)

	var gm *googlemeet.GoogleMeet

	if s.Param().(common.LaunchAppType) == common.LaunchAppInPWA {
		gm, err = googlemeet.StartNewMeetingUsingPWA(ctx, cr, br, googlemeet.WithAllPermissions)
	} else {
		// Meet can dynamically switch between different segmentation models.
		// Force the same model the platform effects use with the experiment ?e=ForceSegmentationModelVariant::GpuMid.
		gm, err = googlemeet.StartNewMeeting(ctx, cr, br, conn,
			map[string]string{
				"e": "ForceSegmentationModelVariant::GpuMid",
			}, googlemeet.WithAllPermissions)
	}
	if err != nil {
		s.Fatal("Failed to start meeting: ", err)
	}
	defer gm.Close(cleanupCtx)

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_with_meet")

	if err := uiauto.Combine("configure Meet",
		gm.MuteIfMicAvailable,
		gm.ChangeSettings(
			gm.SetSendResolution(googlemeet.ResolutionHD720P),
		),
		gm.SwitchVideo(true),
	)(ctx); err != nil {
		s.Fatal("Failed to configure Meet: ", err)
	}

	// Maximize VC app window in clamshell mode to reduce resolution noises on different devices.
	// Skip Tablet mode as app is full screen by default.
	if inTabletMode, err := ash.TabletModeEnabled(ctx, tconn); err != nil {
		s.Fatal("Failed to get tablet-mode status: ", err)
	} else if !inTabletMode {
		if _, err := ash.MaximizeWindowTitleContains(ctx, tconn, "Meet"); err != nil {
			s.Fatal("Failed to maximize Camera window: ", err)
		}
	}

	vcTray := vctray.New(ctx, tconn)

	// Run subtests to verify video effects are correctly applied.
	// Note: Golden images can be found at https://cros-tast-gold.skia.org/list?corpus=videoconferencing.
	subTests := []struct {
		name               string
		backgroundBlur     vctray.BackgroundBlurLevel
		portraitRelighting bool
	}{
		{
			name:               "backgroundblur_off_portraitrelighting_off",
			backgroundBlur:     vctray.BackgroundBlurOff,
			portraitRelighting: false,
		},
		{
			name:               "backgroundblur_light_portraitrelighting_off",
			backgroundBlur:     vctray.BackgroundBlurLight,
			portraitRelighting: false,
		},
		{
			name:               "backgroundblur_full_portraitrelighting_off",
			backgroundBlur:     vctray.BackgroundBlurFull,
			portraitRelighting: false,
		},
		{
			name:               "backgroundblur_off_portraitrelighting_on",
			backgroundBlur:     vctray.BackgroundBlurOff,
			portraitRelighting: true,
		},
		{
			name:               "backgroundblur_light_portraitrelighting_on",
			backgroundBlur:     vctray.BackgroundBlurLight,
			portraitRelighting: true,
		},
		{
			name:               "backgroundblur_full_portraitrelighting_on",
			backgroundBlur:     vctray.BackgroundBlurFull,
			portraitRelighting: true,
		},
	}

	enabledSubtests := make(map[string]struct{})
	subtestsVar, ok := s.Var("subtests")
	if ok {
		testing.ContextLog(ctx, "Enabled subtests: ", subtestsVar)
		for _, subTest := range strings.Split(subtestsVar, ",") {
			enabledSubtests[subTest] = struct{}{}
		}
	}

	for _, subTest := range subTests {
		// Check whether this subtest is enabled in the test var.
		if len(enabledSubtests) > 0 {
			if _, ok := enabledSubtests[subTest.name]; !ok {
				continue
			}
		}

		s.Run(ctx, subTest.name, func(ctx context.Context, s *testing.State) {
			if err := vcTray.SetCameraEffects(subTest.backgroundBlur, subTest.portraitRelighting)(ctx); err != nil {
				s.Fatalf("Failed to set camera effects to BackgroundBlur %v; PortraitRelighting %v: %v",
					subTest.backgroundBlur, subTest.portraitRelighting, err)
			}

			d, err := screenshot.NewDifferFromChrome(ctx, s, cr,
				screenshot.Config{
					DefaultOptions: screenshot.Options{
						WindowState: ash.WindowStateDefault,
					},
					// This is important to get consistent window size.
					// When using DpiNormalization by default, it resizes the window differently on ash and lacros.
					// Refer to b/275932928.
					SkipDpiNormalization: true,
				})
			if err != nil {
				s.Fatal("Failed to start screen differ: ", err)
			}
			defer d.DieOnFailedDiffs()
			if err := d.Diff(ctx, subTest.name, googlemeet.VideoNode,
				screenshot.Retries(5),
				screenshot.RetryInterval(time.Second),
			)(ctx); err != nil {
				s.Fatal("Failed the skia gold diff: ", err)
			}
		})
	}
}
