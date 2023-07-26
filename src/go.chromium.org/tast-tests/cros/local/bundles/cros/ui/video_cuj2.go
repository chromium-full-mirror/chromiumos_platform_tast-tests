// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/cros/ui/setup"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/mtbf/youtube"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type videoCUJParam struct {
	tier        cuj.Tier
	app         string
	browserType browser.Type
}

func init() {
	testing.AddTest(&testing.Test{
		// TODO (b/242590511): Deprecated after moving all performance cuj test cases to go.chromium.org/tast-tests/cros/local/bundles/cros/spera directory.
		Func:         VideoCUJ2,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Measures the smoothness of switch between full screen YouTube video and another browser window",
		Contacts:     []string{"chromeos-perf-reliability-eng@google.com", "cienet-development@googlegroups.com", "alstonhuang@google.com"},
		BugComponent: "b:1025042", // ChromeOS > EngProd > Platform > SPERA > Automation
		SoftwareDeps: []string{"chrome", "arc"},
		HardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		Vars: []string{
			"ui.install_apk",  // Optional. Whether to install the youtube app via apk, the default is "false".
			"ui.apk_url",      // Optional. The url of apk file.
			"ui.cuj_mode",     // Optional. Expecting "tablet" or "clamshell". Other values will be be taken as "clamshell".
			"ui.collectTrace", // Optional. Expecting "enable" or "disable", default is "disable".
			"ui.checkPIP",
		},
		Data: []string{cujrecorder.SystemTraceConfigFile},
		Params: []testing.Param{
			{
				Name:    "basic_youtube_web",
				Fixture: "loggedInAndKeepState",
				Timeout: 12 * time.Minute,
				Val: videoCUJParam{
					tier: cuj.Basic,
					app:  youtube.YoutubeWeb,
				},
			}, {
				Name:              "basic_lacros_youtube_web",
				Fixture:           "loggedInAndKeepStateLacros",
				Timeout:           12 * time.Minute,
				ExtraSoftwareDeps: []string{"lacros"},
				Val: videoCUJParam{
					tier:        cuj.Basic,
					app:         youtube.YoutubeWeb,
					browserType: browser.TypeLacros,
				},
			}, {
				Name:              "basic_youtube_web_crosbolt",
				Fixture:           "loggedInAndKeepState",
				Timeout:           10 * time.Minute,
				ExtraAttr:         []string{"group:crosbolt", "crosbolt_perbuild"},
				ExtraHardwareDeps: hwdep.D(setup.PerfCUJDevices()),
				Val: videoCUJParam{
					tier: cuj.Basic,
					app:  youtube.YoutubeWeb,
				},
			}, {
				Name:    "premium_youtube_web",
				Fixture: "loggedInAndKeepState",
				Timeout: 12 * time.Minute,
				Val: videoCUJParam{
					tier: cuj.Premium,
					app:  youtube.YoutubeWeb,
				},
			}, {
				Name:              "premium_lacros_youtube_web",
				Fixture:           "loggedInAndKeepStateLacros",
				Timeout:           12 * time.Minute,
				ExtraSoftwareDeps: []string{"lacros"},
				Val: videoCUJParam{
					tier:        cuj.Premium,
					app:         youtube.YoutubeWeb,
					browserType: browser.TypeLacros,
				},
			}, {
				Name:              "plus_youtube_web_crosbolt",
				Fixture:           "loggedInAndKeepState",
				Timeout:           10 * time.Minute,
				ExtraAttr:         []string{"group:crosbolt", "crosbolt_perbuild"},
				ExtraHardwareDeps: hwdep.D(setup.PerfCUJDevices()),
				Val: videoCUJParam{
					tier: cuj.Plus,
					app:  youtube.YoutubeWeb,
				},
			}, {
				Name:    "basic_youtube_app",
				Fixture: "loggedInAndKeepStateARCSupported",
				Timeout: 10 * time.Minute,
				Val: videoCUJParam{
					tier: cuj.Basic,
					app:  youtube.YoutubeApp,
				},
			}, {
				Name:              "basic_lacros_youtube_app",
				Fixture:           "loggedInAndKeepStateARCSupportedLacros",
				Timeout:           12 * time.Minute,
				ExtraSoftwareDeps: []string{"lacros"},
				Val: videoCUJParam{
					tier:        cuj.Basic,
					app:         youtube.YoutubeApp,
					browserType: browser.TypeLacros,
				},
			}, {
				Name:              "basic_youtube_app_crosbolt",
				Fixture:           "loggedInAndKeepStateARCSupported",
				Timeout:           10 * time.Minute,
				ExtraAttr:         []string{"group:crosbolt", "crosbolt_perbuild"},
				ExtraHardwareDeps: hwdep.D(setup.PerfCUJDevices()),
				Val: videoCUJParam{
					tier: cuj.Basic,
					app:  youtube.YoutubeApp,
				},
			}, {
				Name:    "premium_youtube_app",
				Fixture: "loggedInAndKeepStateARCSupported",
				Timeout: 10 * time.Minute,
				Val: videoCUJParam{
					tier: cuj.Premium,
					app:  youtube.YoutubeApp,
				},
			}, {
				Name:              "premium_lacros_youtube_app",
				Fixture:           "loggedInAndKeepStateARCSupportedLacros",
				Timeout:           12 * time.Minute,
				ExtraSoftwareDeps: []string{"lacros"},
				Val: videoCUJParam{
					tier:        cuj.Premium,
					app:         youtube.YoutubeApp,
					browserType: browser.TypeLacros,
				},
			}, {
				Name:              "plus_youtube_app_crosbolt",
				Fixture:           "loggedInAndKeepStateARCSupported",
				Timeout:           10 * time.Minute,
				ExtraAttr:         []string{"group:crosbolt", "crosbolt_perbuild"},
				ExtraHardwareDeps: hwdep.D(setup.PerfCUJDevices()),
				Val: videoCUJParam{
					tier: cuj.Plus,
					app:  youtube.YoutubeApp,
				},
			},
		},
	})
}

// VideoCUJ2 performs the video cases including youtube web, and youtube app.
func VideoCUJ2(ctx context.Context, s *testing.State) {
	p := s.Param().(videoCUJParam)
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
	defer kb.Close(ctx)

	videoCUJParams := s.Param().(videoCUJParam)
	app := videoCUJParams.app
	youtubeApkURL := ""
	if app == youtube.YoutubeApp {
		if v, ok := s.Var("ui.install_apk"); ok {
			installApk, err := strconv.ParseBool(v)
			if err != nil {
				s.Fatalf("Failed to parse ui.installApk value %v: %v", v, err)
			}
			if installApk {
				if v, ok := s.Var("ui.apk_url"); ok {
					youtubeApkURL = v
				} else {
					// If no url is provided, fallback to install the latest version.
					s.Logf("Failed to parse ui.apk_url value %v: %v; will install latest version", v, err)
					installApk = false
				}
			}
		}
	}

	var checkPIP bool
	if v, ok := s.Var("ui.checkPIP"); ok {
		checkPIP, err = strconv.ParseBool(v)
		if err != nil {
			s.Fatalf("Failed to parse ui.checkPIP value %v: %v", v, err)
		}
	}

	tabletMode, resetTabletMode, err := cuj.EnableTabletMode(ctx, tconn, s.Var, "ui.cuj_mode")
	if err != nil {
		s.Fatal("Failed to enable tablet mode: ", err)
	}
	defer resetTabletMode(cleanupCtx)

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
	defer uiHandler.Close(ctx)

	traceConfigPath := ""
	if collect, ok := s.Var("ui.collectTrace"); ok && collect == "enable" {
		traceConfigPath = s.DataPath(cujrecorder.SystemTraceConfigFile)
	}
	testResources := youtube.TestResources{
		Cr:        cr,
		Tconn:     tconn,
		Bt:        p.browserType,
		A:         a,
		Kb:        kb,
		UIHandler: uiHandler,
	}

	testParams := youtube.TestParams{
		Tier:            videoCUJParams.tier,
		App:             videoCUJParams.app,
		OutDir:          s.OutDir(),
		TabletMode:      tabletMode,
		ExtendedDisplay: false,
		CheckPIP:        checkPIP,
		TraceConfigPath: traceConfigPath,
		YoutubeApkURL:   youtubeApkURL,
	}

	if err := youtube.Run(ctx, testResources, testParams); err != nil {
		s.Fatal("Failed to run video cuj test: ", err)
	}
}
