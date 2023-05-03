// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package videoconferencing

import (
	"context"
	"time"

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
		Func:         MeetEffectsResolution,
		LacrosStatus: testing.LacrosVariantUnneeded,
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
		Vars:         screenshot.ScreenDiffVars,
		Params: []testing.Param{
			// Note: 720p has already been covered in MeetEffects test as default value.
			{
				Name: "1080p",
				// Only a few boards support 1080p in Google Meet.
				ExtraHardwareDeps: hwdep.D(hwdep.Model("guybrush", "skyrim")),
				Fixture:           fixture.GAIALoggedInWithFakeHALAndEffectsEnabled,
				Val:               googlemeet.ResolutionFullHD1080P,
			},
			{
				Name:    "360p",
				Fixture: fixture.GAIALoggedInWithFakeHALAndEffectsEnabled,
				Val:     googlemeet.ResolutionSD360P,
			},
			{
				Name:    "180p",
				Fixture: fixture.GAIALoggedInWithFakeHALAndEffectsEnabled,
				Val:     googlemeet.ResolutionLD180P,
			},
		},
	})
}

func MeetEffectsResolution(ctx context.Context, s *testing.State) {
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

	gm, err := googlemeet.StartNewMeeting(ctx, cr, br, conn,
		map[string]string{
			"e": "ForceSegmentationModelVariant::GpuMid",
		}, googlemeet.WithAllPermissions)
	if err != nil {
		s.Fatal("Failed to start meeting: ", err)
	}
	defer gm.Close(cleanupCtx)

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_with_meet")

	resolution := s.Param().(string)
	if err := uiauto.Combine("configure Meet",
		gm.MuteIfMicAvailable,
		gm.ChangeSettings(
			gm.SetSendResolution(googlemeet.ResolutionOption(resolution)),
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

	// Use backgroundblur_light_portraitrelighting_on as representative effect
	// to verify effects working on different resolution.
	// Similarly backgroundblur_full_portraitrelighting_on is used in cca_effects_retain
	// to avoid exhausted testing. More combinations can be added if required.
	effectName := "backgroundblur_light_portraitrelighting_on"
	if err := vcTray.SetCameraEffects(vctray.BackgroundBlurLight, true)(ctx); err != nil {
		s.Fatal("Failed to set camera effects: ", err)
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
	if err := d.Diff(ctx, effectName, googlemeet.VideoNode,
		screenshot.Retries(5),
		screenshot.RetryInterval(time.Second),
	)(ctx); err != nil {
		s.Fatal("Failed the skia gold diff: ", err)
	}
}
