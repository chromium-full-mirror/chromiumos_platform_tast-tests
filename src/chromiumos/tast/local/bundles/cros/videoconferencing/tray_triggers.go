// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package videoconferencing

import (
	"context"
	"time"

	"chromiumos/tast/ctxutil"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/apps/thirdparty/zoom"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/browser/browserfixt"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/chrome/uiauto/vctray"
	"chromiumos/tast/local/videoconferencing/fixture"
	"chromiumos/tast/testing"
	"chromiumos/tast/testing/hwdep"
)

type triggerType int

const (
	micTrigger triggerType = iota
	camTrigger
	screenTrigger
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         TrayTriggers,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks VC tray can be triggered by sharing screen",
		Contacts: []string{
			"chrome-knowledge-eng@google.com",
			"shengjun@chromium.org",
		},
		BugComponent: "b:187682",
		Timeout:      3 * time.Minute,
		Attr: []string{
			"group:video_conference", "video_conference_per_build", "group:external-dependency",
		},
		SoftwareDeps: []string{"chrome", "camera_feature_effects"},
		HardwareDeps: hwdep.D(hwdep.SkipOnModel("betty")),
		Params: []testing.Param{
			{
				Name:    "screen",
				Val:     screenTrigger,
				Fixture: fixture.GAIALoggedInWithFakeHALAndEffectsEnabled,
			},
			{
				Name:              "screen_lacros",
				Val:               screenTrigger,
				ExtraSoftwareDeps: []string{"lacros"},
				Fixture:           fixture.GAIALoggedInLacrosWithFakeHALAndEffectsEnabled,
			},
			{
				Name:    "mic",
				Val:     micTrigger,
				Fixture: fixture.GAIALoggedInWithFakeHALAndEffectsEnabled,
			},
			{
				Name:              "mic_lacros",
				Val:               micTrigger,
				ExtraSoftwareDeps: []string{"lacros"},
				Fixture:           fixture.GAIALoggedInLacrosWithFakeHALAndEffectsEnabled,
			},
			{
				Name:    "cam",
				Val:     camTrigger,
				Fixture: fixture.GAIALoggedInWithFakeHALAndEffectsEnabled,
			},
			{
				Name:              "cam_lacros",
				Val:               camTrigger,
				ExtraSoftwareDeps: []string{"lacros"},
				Fixture:           fixture.GAIALoggedInLacrosWithFakeHALAndEffectsEnabled,
			},
		},
	})
}

func TrayTriggers(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
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

	zm, err := zoom.StartNewMeeting(ctx, cr, br, zoom.WithAllPermissions)
	if err != nil {
		s.Fatal("Failed to start meeting: ", err)
	}

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_zoom")

	vcTray := vctray.New(ctx, tconn)
	if isShown, err := vcTray.Exists(ctx); err != nil {
		s.Fatal("Failed to check the existence of vcTray: ", err)
	} else if isShown {
		s.Fatal("vcTray is already shown unexpectedly before trigger")
	}

	// Verify different triggers.
	switch s.Param().(triggerType) {
	case camTrigger:
		verifyCameraTrigger(ctx, s, zm, vcTray)
	case micTrigger:
		verifyMicTrigger(ctx, s, zm, vcTray)
	case screenTrigger:
		verifyScreenTrigger(ctx, s, br, zm, vcTray)
	}

	if err := uiauto.Combine("close Zoom should hide vcTray",
		zm.Close,
		vcTray.WaitUntilGone,
	)(ctx); err != nil {
		s.Fatal("Failed to close the app to hide vcTray: ", err)
	}
}

func verifyScreenTrigger(ctx context.Context, s *testing.State, br *browser.Browser, zm *zoom.Zoom, vcTray *vctray.VCTray) {
	// Create a new tab for sharing screen.
	const (
		newTabURL   = ""
		newTabTitle = "about:blank"
	)

	newTabConn, err := br.NewTab(ctx, newTabURL, browser.WithBackground())
	if err != nil {
		s.Fatal("Failed to create new tab: ", err)
	}
	defer newTabConn.Close()
	defer newTabConn.CloseTarget(ctx)

	if err := uiauto.Combine("verify sharing screen triggers vcTray",
		zm.ShareScreen(newTabTitle),
		vcTray.WaitUntilExists,
		zm.StopShareScreen(),
	)(ctx); err != nil {
		s.Fatal("Failed to verify sharing screen triggers vcTray: ", err)
	}
}

func verifyCameraTrigger(ctx context.Context, s *testing.State, zm *zoom.Zoom, vcTray *vctray.VCTray) {
	if err := uiauto.Combine("verify camera triggers vcTray",
		zm.SwitchVideo(true),
		vcTray.WaitUntilExists,
	)(ctx); err != nil {
		s.Fatal("Failed to verify camera triggers vcTray: ", err)
	}
}

func verifyMicTrigger(ctx context.Context, s *testing.State, zm *zoom.Zoom, vcTray *vctray.VCTray) {
	if err := uiauto.Combine("verify microphone triggers vcTray",
		zm.SetJoinAudio(true),
		vcTray.WaitUntilExists,
	)(ctx); err != nil {
		s.Fatal("Failed to verify microphone triggers vcTray: ", err)
	}
}
