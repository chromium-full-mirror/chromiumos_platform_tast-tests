// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package videoconferencing

import (
	"context"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/videoconferencing/effects"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/apps/thirdparty/zoom"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/videoconferencing/fixture"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type zoomParams struct {
	appBlur         bool
	platformBlur    bool
	platformRelight bool
}

func init() {
	// Note: BenchmarkingZoom is currently for manual testing.
	testing.AddTest(&testing.Test{
		Func:         BenchmarkingZoom,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Captures FPS metrics for VC effects in Zoom",
		Contacts: []string{
			"chromeos-platform-ml@google.com",
			"zhaon@google.com",
		},
		BugComponent: "b:1212695",
		SoftwareDeps: []string{"chrome", "camera_feature_effects"},
		HardwareDeps: hwdep.D(hwdep.SkipOnModel("betty")),
		Timeout:      35 * time.Minute,
		Vars: []string{
			// How many seconds to sample FPS for.
			"videoconferencing.test_duration",
		},
		Data: []string{
			"effects_zoom_fps.js",
		},
		Fixture: fixture.GAIALoggedInAndBenchmarkSetupFixture,
		Params: []testing.Param{
			{
				Name: "no_effects",
				Val:  zoomParams{},
			},
			{
				Name: "app_blur",
				Val: zoomParams{
					appBlur: true,
				},
			},
			{
				Name: "platform_blur",
				Val: zoomParams{
					platformBlur: true,
				},
			},
			{
				Name: "platform_relight",
				Val: zoomParams{
					platformRelight: true,
				},
			},
			{
				Name: "platform_blur_relight",
				Val: zoomParams{
					platformBlur:    true,
					platformRelight: true,
				},
			},
		},
	})
}

func BenchmarkingZoom(ctx context.Context, s *testing.State) {
	// Shorten context to allow for cleanup. Reserve one minute in case of power
	// test.
	closeCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Minute)
	defer cancel()

	param, ok := s.Param().(zoomParams)
	if !ok {
		s.Fatal("Failed to convert test zoomParams")
	}

	var err error
	testDuration := effects.DefaultTestDuration

	if varValue, ok := s.Var("videoconferencing.test_duration"); ok {
		testDuration, err = strconv.Atoi(varValue)
		if err != nil || testDuration <= 0 {
			s.Fatal("Failed to parse videoconferencing.test_duration: ", err)
		}

	}

	fixt := s.FixtValue().(fixture.BenchmarkSetUpFixtureData)
	cr := fixt.Chrome

	conn, br, cleanup, err := browserfixt.SetUpWithURL(ctx, cr, browser.TypeAsh, chrome.NewTabURL)
	if err != nil {
		s.Fatal("Failed to launch browser: ", err)
	}
	defer cleanup(closeCtx)
	defer conn.Close()
	defer conn.CloseTarget(closeCtx)

	zm, err := zoom.StartNewMeeting(ctx, cr, br, conn, zoom.WithAllPermissions)
	if err != nil {
		s.Fatal("Failed to start meeting: ", err)
	}
	defer zm.Close(closeCtx)

	// Set up video and enter full screen.
	if err := uiauto.NamedCombine("configure meeting",
		zm.SwitchVideo(true),
		zm.EnterFullScreen,
	)(ctx); err != nil {
		s.Fatal("Failed to configure meeting: ", err)
	}

	backgroundSettingAction := zm.SetBackgroundNone
	backgroundOption := "None"
	if param.appBlur {
		backgroundSettingAction = zm.SetBackgroundBlur
		backgroundOption = "Blur"
	}

	if err := zm.ChangeSettings(backgroundSettingAction)(ctx); err != nil {
		s.Fatalf("Failed to configure background %q: %v", backgroundOption, err)
	}

	cleanupApply, err := effects.ApplyPlatformEffects(ctx, param.platformBlur, param.platformRelight, effects.KAuto)
	if err != nil {
		s.Fatal("Failed to apply platform effects: ", err)
	}
	if cleanupApply != nil {
		defer func() {
			if err := cleanupApply(ctx); err != nil {
				s.Error("Failed to clean up apply platform effects: ", err)
			}
		}()
	}

	testing.ContextLog(ctx, "Letting things settle for 5 seconds")
	// GoBigSleepLint: Allow power and effects to stabilize before taking metrics.
	if err := testing.Sleep(ctx, 5*time.Second); err != nil {
		s.Fatal("Failed to let things settle: ", err)
	}

	p := perf.NewValues()

	// Capture FPS.
	if err = effects.ReportFramePerfMetrics(ctx, p, zm.Conn(), s.DataPath("effects_zoom_fps.js"), testDuration); err != nil {
		s.Fatal("Failed to report fps and frame duration metrics: ", err)
	}
	if err := p.Save(s.OutDir()); err != nil {
		s.Error("Cannot save perf data: ", err)
	}

}
