// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package spera

import (
	"context"
	"strconv"
	"time"

	"chromiumos/tast/ctxutil"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/ash"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/cuj"
	"chromiumos/tast/local/chrome/display"
	"chromiumos/tast/local/input"
	"chromiumos/tast/local/mtbf/youtube"
	"chromiumos/tast/local/ui/cujrecorder"
	"chromiumos/tast/testing"
	"chromiumos/tast/testing/hwdep"
)

const youtubeApkName = "youtube_1531188672.apk"

type videoStreamingAppParam struct {
	tier        cuj.Tier
	app         string
	browserType browser.Type
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         VideoStreamingApp,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Measures the smoothess of switch between full screen YouTube video and another browser window",
		Contacts:     []string{"xliu@cienet.com", "alston.huang@cienet.com", "cienet-development@googlegroups.com"},
		SoftwareDeps: []string{"chrome", "arc"},
		HardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		Vars: []string{
			"spera.install_apk",  // Optional. Whether to install the youtube app via apk, the default is "false".
			"spera.cuj_mode",     // Optional. Expecting "tablet" or "clamshell". Other values will be be taken as "clamshell".
			"spera.collectTrace", // Optional. Expecting "enable" or "disable", default is "disable".
			"spera.checkPIP",
		},
		Data: []string{cujrecorder.SystemTraceConfigFile},
		Params: []testing.Param{
			{
				Name:      "essential",
				Fixture:   "loggedInAndKeepState",
				Timeout:   10 * time.Minute,
				ExtraData: []string{youtubeApkName},
				Val: videoStreamingAppParam{
					tier: cuj.Essential,
					app:  youtube.YoutubeApp,
				},
			}, {
				Name:              "essential_lacros",
				Fixture:           "loggedInAndKeepStateLacros",
				Timeout:           10 * time.Minute,
				ExtraSoftwareDeps: []string{"lacros"},
				ExtraData:         []string{youtubeApkName},
				Val: videoStreamingAppParam{
					tier:        cuj.Essential,
					app:         youtube.YoutubeApp,
					browserType: browser.TypeLacros,
				},
			}, {
				Name:      "advanced",
				Fixture:   "loggedInAndKeepState",
				Timeout:   10 * time.Minute,
				ExtraData: []string{youtubeApkName},
				Val: videoStreamingAppParam{
					tier: cuj.Advanced,
					app:  youtube.YoutubeApp,
				},
			}, {
				Name:              "advanced_lacros",
				Fixture:           "loggedInAndKeepStateLacros",
				Timeout:           10 * time.Minute,
				ExtraSoftwareDeps: []string{"lacros"},
				ExtraData:         []string{youtubeApkName},
				Val: videoStreamingAppParam{
					tier:        cuj.Advanced,
					app:         youtube.YoutubeApp,
					browserType: browser.TypeLacros,
				},
			},
		},
	})
}

// VideoStreamingApp performs the video test on youtube app.
func VideoStreamingApp(ctx context.Context, s *testing.State) {
	videoStreamingAppParams := s.Param().(videoStreamingAppParam)
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	a := s.FixtValue().(cuj.FixtureData).ARC

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to open the keyboard: ", err)
	}
	defer kb.Close()

	app := videoStreamingAppParams.app
	youtubeApkPath := ""
	if app == youtube.YoutubeApp {
		if v, ok := s.Var("spera.install_apk"); ok {
			installApk, err := strconv.ParseBool(v)
			if err != nil {
				s.Fatalf("Failed to parse spera.installApk value %v: %v", v, err)
			}
			if installApk {
				youtubeApkPath = s.DataPath(youtubeApkName)
			}
		}
	}

	var checkPIP bool
	if v, ok := s.Var("spera.checkPIP"); ok {
		checkPIP, err = strconv.ParseBool(v)
		if err != nil {
			s.Fatalf("Failed to parse spera.checkPIP value %v: %v", v, err)
		}
	}
	var tabletMode bool
	if mode, ok := s.Var("spera.cuj_mode"); ok {
		tabletMode = mode == "tablet"
		cleanup, err := ash.EnsureTabletModeEnabled(ctx, tconn, tabletMode)
		if err != nil {
			s.Fatalf("Failed to enable tablet mode to %v: %v", tabletMode, err)
		}
		defer cleanup(cleanupCtx)
	} else {
		// Use default screen mode of the DUT.
		tabletMode, err = ash.TabletModeEnabled(ctx, tconn)
		if err != nil {
			s.Fatal("Failed to get DUT default screen mode: ", err)
		}
	}
	s.Log("Running test with tablet mode: ", tabletMode)
	var uiHandler cuj.UIActionHandler
	if tabletMode {
		cleanup, err := display.RotateToLandscape(ctx, tconn)
		if err != nil {
			s.Fatal("Failed to rotate display to landscape: ", err)
		}
		defer cleanup(cleanupCtx)
		if uiHandler, err = cuj.NewTabletActionHandler(ctx, tconn); err != nil {
			s.Fatal("Failed to create tablet action handler: ", err)
		}
	} else {
		if uiHandler, err = cuj.NewClamshellActionHandler(ctx, tconn); err != nil {
			s.Fatal("Failed to create clamshell action handler: ", err)
		}
	}
	defer uiHandler.Close()

	traceConfigPath := ""
	if collect, ok := s.Var("spera.collectTrace"); ok && collect == "enable" {
		traceConfigPath = s.DataPath(cujrecorder.SystemTraceConfigFile)
	}
	testResources := youtube.TestResources{
		Cr:        cr,
		Tconn:     tconn,
		Bt:        videoStreamingAppParams.browserType,
		A:         a,
		Kb:        kb,
		UIHandler: uiHandler,
	}
	testParams := youtube.TestParams{
		Tier:            videoStreamingAppParams.tier,
		App:             app,
		OutDir:          s.OutDir(),
		TabletMode:      tabletMode,
		ExtendedDisplay: false,
		CheckPIP:        checkPIP,
		TraceConfigPath: traceConfigPath,
		YoutubeApkPath:  youtubeApkPath,
	}

	if err := youtube.Run(ctx, testResources, testParams); err != nil {
		s.Fatal("Failed to run video cuj test: ", err)
	}
}
