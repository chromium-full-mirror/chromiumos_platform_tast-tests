// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package spera

import (
	"context"
	"strconv"
	"time"

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

const videoStreamingAppTimeout = 12 * time.Minute

type videoStreamingAppParam struct {
	tier        cuj.Tier
	app         string
	browserType browser.Type
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         VideoStreamingApp,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Measures the smoothness of switch between full screen YouTube video and another browser window",
		Contacts:     []string{"chromeos-perf-reliability-eng@google.com", "cienet-development@googlegroups.com", "alstonhuang@google.com"},
		BugComponent: "b:1025042", // ChromeOS > EngProd > Platform > SPERA > Automation
		SoftwareDeps: []string{"chrome", "arc"},
		HardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		Vars: []string{
			"spera.install_apk",  // Optional. Whether to install the youtube app via apk, the default is "false".
			"spera.apk_url",      // Optional. The url of apk file.
			"spera.cuj_mode",     // Optional. Expecting "tablet" or "clamshell". Other values will be be taken as "clamshell".
			"spera.collectTrace", // Optional. Expecting "enable" or "disable", default is "disable".
			"spera.checkPIP",
		},
		Data: []string{cujrecorder.SystemTraceConfigFile},
		Params: []testing.Param{
			{
				Name:    "essential",
				Fixture: "loggedInAndKeepStateARCSupported",
				Timeout: videoStreamingAppTimeout,
				Val: videoStreamingAppParam{
					tier: cuj.Essential,
					app:  youtube.YoutubeApp,
				},
			}, {
				Name:              "essential_lacros",
				Fixture:           "loggedInAndKeepStateARCSupportedLacros",
				Timeout:           videoStreamingAppTimeout,
				ExtraSoftwareDeps: []string{"lacros"},
				Val: videoStreamingAppParam{
					tier:        cuj.Essential,
					app:         youtube.YoutubeApp,
					browserType: browser.TypeLacros,
				},
			}, {
				Name:    "advanced",
				Fixture: "loggedInAndKeepStateARCSupported",
				Timeout: videoStreamingAppTimeout,
				Val: videoStreamingAppParam{
					tier: cuj.Advanced,
					app:  youtube.YoutubeApp,
				},
			}, {
				Name:              "advanced_lacros",
				Fixture:           "loggedInAndKeepStateARCSupportedLacros",
				Timeout:           videoStreamingAppTimeout,
				ExtraSoftwareDeps: []string{"lacros"},
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
	defer kb.Close(ctx)

	app := videoStreamingAppParams.app
	youtubeApkURL := ""
	if app == youtube.YoutubeApp {
		if v, ok := s.Var("spera.install_apk"); ok {
			installApk, err := strconv.ParseBool(v)
			if err != nil {
				s.Fatalf("Failed to parse spera.installApk value %v: %v", v, err)
			}
			if installApk {
				if v, ok := s.Var("spera.apk_url"); ok {
					youtubeApkURL = v
				} else {
					// If no url is provided, fallback to install the latest version.
					s.Logf("Failed to parse spera.apk_url value %v: %v; will install latest version", v, err)
					installApk = false
				}
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
	tabletMode, resetTabletMode, err := cuj.EnableTabletMode(ctx, tconn, s.Var, "spera.cuj_mode")
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
		YoutubeApkURL:   youtubeApkURL,
	}

	if err := youtube.Run(ctx, testResources, testParams); err != nil {
		s.Fatal("Failed to run video cuj test: ", err)
	}
}
