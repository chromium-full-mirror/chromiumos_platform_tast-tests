// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/chameleon"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/ui/conference"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/mtbf/youtube"
	"go.chromium.org/tast-tests/cros/local/typecutils"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const googleMeet = "Google Meet"

type extendedDisplayCUJParam struct {
	tier        cuj.Tier
	app         string
	roomType    conference.RoomType
	browserType browser.Type
}

func init() {
	testing.AddTest(&testing.Test{
		// TODO (b/242590511): Deprecated after moving all performance cuj test cases to go.chromium.org/tast-tests/cros/local/bundles/cros/spera directory.
		Func:         ExtendedDisplayCUJ,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Test video entertainment and google meet with extended display",
		Contacts: []string{
			"chromeos-perf-reliability-eng@google.com",
			"cienet-development@googlegroups.com",
			"chicheny@google.com",
		},
		BugComponent: "b:1025042", // ChromeOS > EngProd > Platform > SPERA > Automation
		SoftwareDeps: []string{"chrome", "arc"},
		HardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		Vars: []string{
			"ui.cuj_mode",               // Optional. Use "tablet" to force the tablet mode. Other values will be be taken as "clamshell".
			"ui.chameleon_addr",         // Only needed when using chameleon board as extended display.
			"ui.chameleon_display_port", // The port connected as extended display. Default is 3.
			"ui.collectTrace",           // Optional. Expecting "enable" or "disable", default is "disable".
			// Credentials for BOND API.
			"ui.meet_bond_key",
			// Optional. The total timeout and inteval when bond api fails.
			"ui.meet_retry_timeout",
			"ui.meet_retry_interval",
		},
		Data: []string{cujrecorder.SystemTraceConfigFile},
		Params: []testing.Param{
			{
				Name:    "plus_video_youtube_web",
				Timeout: 12 * time.Minute,
				Fixture: "loggedInAndKeepState",
				Val: extendedDisplayCUJParam{
					tier: cuj.Basic, // Extended display plus tier test uses basic tier test of video CUJ.
					app:  youtube.YoutubeWeb,
				},
			}, {
				Name:              "plus_lacros_video_youtube_web",
				Timeout:           12 * time.Minute,
				Fixture:           "loggedInAndKeepStateLacros",
				ExtraSoftwareDeps: []string{"lacros"},
				Val: extendedDisplayCUJParam{
					tier:        cuj.Basic, // Extended display plus tier test uses basic tier test of video CUJ.
					app:         youtube.YoutubeWeb,
					browserType: browser.TypeLacros,
				},
			},
			{
				Name:    "premium_meet_large",
				Fixture: "loggedInAndKeepStateWithLowResFakeCamera",
				Timeout: 50 * time.Minute,
				Val: extendedDisplayCUJParam{
					// This is a premium test case for extended display CUJ.
					// But this case just calls Google Meet "plus" case, so the given tier
					// is "plus" instead of "premium".
					tier:        cuj.Plus,
					app:         googleMeet,
					roomType:    conference.LargeRoomSize,
					browserType: browser.TypeAsh,
				},
			},
			{
				Name:              "premium_lacros_meet_large",
				Fixture:           "loggedInAndKeepStateLacrosWithLowResFakeCamera",
				Timeout:           50 * time.Minute,
				ExtraSoftwareDeps: []string{"lacros"},
				Val: extendedDisplayCUJParam{
					// This is a premium test case for extended display CUJ.
					// But this case just calls Google Meet "plus" case, so the given tier
					// is "plus" instead of "premium".
					tier:        cuj.Plus,
					app:         googleMeet,
					roomType:    conference.LargeRoomSize,
					browserType: browser.TypeLacros,
				},
			},
		},
	})
}

// ExtendedDisplayCUJ performs the video cuj (youtube web) test on extended display.
// Known issues: b:187165216 describes an issue that click event cannot be executed
// on extended display on certain models.
func ExtendedDisplayCUJ(ctx context.Context, s *testing.State) {
	p := s.Param().(extendedDisplayCUJParam)
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	if chameleonAddr, ok := s.Var("ui.chameleon_addr"); ok {
		// Use chameleon board as extended display. Make sure chameleon is connected.
		che, err := chameleon.New(ctx, chameleonAddr)
		if err != nil {
			s.Fatal("Failed to connect to chameleon board: ", err)
		}
		defer che.Close(cleanupCtx)

		portID := 3 // Use default port 3 for display.
		if port, ok := s.Var("ui.chameleon_display_port"); ok {
			portID, err = strconv.Atoi(port)
			if err != nil {
				s.Fatalf("Failed to parse chameleon display port %q: %v", port, err)
			}
		}

		dp, err := che.NewPort(ctx, portID)
		if err != nil {
			s.Fatalf("Failed to create chameleon port %d: %v", portID, err)
		}
		if err := dp.Plug(ctx); err != nil {
			s.Fatal("Failed to plug chameleon port: ", err)
		}
		defer dp.Unplug(cleanupCtx)
		// Wait for DUT to detect external display.
		if err := dp.WaitVideoInputStable(ctx, 10*time.Second); err != nil {
			s.Fatal("Failed to wait for video input on chameleon board: ", err)
		}
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}
	// Unset mirrored display so two displays can show different information.
	if err := typecutils.SetMirrorDisplay(ctx, tconn, false); err != nil {
		s.Fatal("Failed to unset mirror display: ", err)
	}
	// Make sure there are two displays on DUT.
	// This procedure must be performed after display mirror is unset. Otherwise we can only
	// get one display info.
	infos, err := display.GetInfo(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get display info: ", err)
	}
	if len(infos) != 2 {
		s.Fatalf("DUT connected with incorrect nubmer of displays - want 2, got %d: %v", len(infos), err)
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to open the keyboard: ", err)
	}
	defer kb.Close(ctx)

	tabletMode, resetTabletMode, err := cuj.EnableTabletMode(ctx, tconn, s.Var, "ui.cuj_mode")
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

	// The external display is not able to be controlled by touch event,
	// so we'll test extended display with keyboard/mouse even on tablet mode.
	uiHandler, err := cuj.NewClamshellActionHandler(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to create clamshell action handler: ", err)
	}
	defer uiHandler.Close(ctx)

	traceConfigPath := ""
	if collect, ok := s.Var("ui.collectTrace"); ok && collect == "enable" {
		traceConfigPath = s.DataPath(cujrecorder.SystemTraceConfigFile)
	}

	param := s.Param().(extendedDisplayCUJParam)

	if param.app == googleMeet {
		meetConfig, err := conference.GetGoogleMeetConfig(ctx, s, param.roomType)
		if err != nil {
			s.Fatal("Failed to get meet config: ", err)
		}

		testParams := &conference.TestParams{
			Cr:              cr,
			Tier:            param.tier,
			BrowserType:     param.browserType,
			RoomType:        param.roomType,
			OutDir:          s.OutDir(),
			TraceConfigPath: traceConfigPath,
			TabletMode:      tabletMode,
			ExtendedDisplay: true,
		}

		if err := conference.RunWithGoogleConfig(ctx, tconn, meetConfig, testParams); err != nil {
			s.Fatal("Failed to run google meet with extended display: ", err)
		}
	} else {
		a := s.FixtValue().(cuj.FixtureData).ARC
		testResources := youtube.TestResources{
			Cr:        cr,
			Tconn:     tconn,
			Bt:        p.browserType,
			A:         a,
			Kb:        kb,
			UIHandler: uiHandler,
		}
		testParams := youtube.TestParams{
			Tier:            param.tier,
			App:             param.app,
			OutDir:          s.OutDir(),
			TabletMode:      tabletMode,
			ExtendedDisplay: true,
			TraceConfigPath: traceConfigPath,
		}

		if err := youtube.Run(ctx, testResources, testParams); err != nil {
			s.Fatal("Failed to run youtube web with extended display: ", err)
		}
	}
}
