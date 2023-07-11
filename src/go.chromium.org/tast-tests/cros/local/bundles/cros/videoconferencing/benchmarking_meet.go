// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package videoconferencing

import (
	"context"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/bond"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/videoconferencing/effects"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/apps/thirdparty/googlemeet"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/videoconferencing/fixture"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type meetParams struct {
	appBlur         bool
	appRelight      bool
	platformBlur    bool
	platformRelight bool
	modelType       effects.ModelType
	botCount        int
}

const botDuration = 7 * time.Minute

func init() {
	testing.AddTest(&testing.Test{
		Func:         BenchmarkingMeet,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Captures performance regression and power metrics for VC effects in Meet",
		Contacts: []string{
			"chromeos-platform-ml@google.com",
			"zhaon@google.com",
		},
		BugComponent: "b:260653207",
		SoftwareDeps: []string{"chrome", "camera_feature_effects"},
		HardwareDeps: hwdep.D(hwdep.SkipOnModel("betty")),
		Timeout:      15 * time.Minute,
		Vars: []string{
			// How many minutes to capture metrics.
			"videoconferencing.test_duration",
		},
		VarDeps: []string{
			"ui.bond_credentials",
		},
		Attr: []string{"group:ml_benchmark", "ml_benchmark_nightly"},
		Data: []string{
			"effects_frame_metrics.js",
		},
		Fixture: fixture.GAIALoggedInAndBenchmarkSetupFixture,
		Params: []testing.Param{
			{
				Name: "no_effects_720p",
				Val:  meetParams{},
			},
			{
				Name: "app_blur_720p",
				Val: meetParams{
					appBlur: true,
				},
			},
			{
				Name: "platform_blur_720p",
				Val: meetParams{
					platformBlur: true,
					modelType:    effects.KHd,
				},
			},
			{
				Name: "platform_blur_720p_low_segm",
				Val: meetParams{
					platformBlur: true,
					modelType:    effects.KFull,
				},
			},
			{
				Name: "platform_blur_720p_effnet256",
				Val: meetParams{
					platformBlur: true,
					modelType:    effects.KEffnet256,
				},
			},
			{
				Name: "platform_blur_720p_effnet384",
				Val: meetParams{
					platformBlur: true,
					modelType:    effects.KEffnet384,
				},
			},
			{
				Name: "app_relight_720p",
				Val: meetParams{
					appRelight: true,
				},
			},
			{
				Name: "platform_relight_720p",
				Val: meetParams{
					platformRelight: true,
					modelType:       effects.KHd,
				},
			},
			{
				Name: "platform_relight_720p_low_segm",
				Val: meetParams{
					platformRelight: true,
					modelType:       effects.KFull,
				},
			},
			{
				Name: "app_blur_relight_720p",
				Val: meetParams{
					appBlur:    true,
					appRelight: true,
				},
			},
			{
				Name: "platform_blur_relight_720p",
				Val: meetParams{
					platformBlur:    true,
					platformRelight: true,
					modelType:       effects.KHd,
				},
			},
			{
				Name: "platform_blur_relight_720p_low_segm",
				Val: meetParams{
					platformBlur:    true,
					platformRelight: true,
					modelType:       effects.KFull,
				},
			},
			{
				Name: "platform_blur_relight_720p_effnet256",
				Val: meetParams{
					platformBlur:    true,
					platformRelight: true,
					modelType:       effects.KEffnet256,
				},
			},
			{
				Name: "platform_blur_relight_720p_effnet384",
				Val: meetParams{
					platformBlur:    true,
					platformRelight: true,
					modelType:       effects.KEffnet384,
				},
			},
			{
				Name: "no_effects_720p_4ppl",
				Val: meetParams{
					botCount: 3,
				},
			},
			{
				Name: "app_blur_720p_4ppl",
				Val: meetParams{
					appBlur:  true,
					botCount: 3,
				},
			},
			{
				Name: "platform_relight_720p_4ppl",
				Val: meetParams{
					platformRelight: true,
					modelType:       effects.KHd,
					botCount:        3,
				},
			},
			{
				Name: "platform_blur_720p_4ppl",
				Val: meetParams{
					platformBlur: true,
					modelType:    effects.KHd,
					botCount:     3,
				},
			},
			{
				Name: "platform_blur_relight_720p_4ppl",
				Val: meetParams{
					platformBlur:    true,
					platformRelight: true,
					modelType:       effects.KHd,
					botCount:        3,
				},
			},
			{
				Name: "no_effects_720p_10ppl",
				Val: meetParams{
					botCount: 9,
				},
			},
			{
				Name: "app_blur_720p_10ppl",
				Val: meetParams{
					appBlur:  true,
					botCount: 9,
				},
			},
			{
				Name: "platform_relight_720p_10ppl",
				Val: meetParams{
					platformRelight: true,
					modelType:       effects.KHd,
					botCount:        9,
				},
			},
			{
				Name: "platform_blur_720p_10ppl",
				Val: meetParams{
					platformBlur: true,
					modelType:    effects.KHd,
					botCount:     9,
				},
			},
			{
				Name: "platform_blur_relight_720p_10ppl",
				Val: meetParams{
					platformBlur:    true,
					platformRelight: true,
					modelType:       effects.KHd,
					botCount:        9,
				},
			},
		},
	})
}

func BenchmarkingMeet(ctx context.Context, s *testing.State) {
	// Shorten context to allow for cleanup. Reserve one minute in case of power
	// test.
	closeCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Minute)
	defer cancel()

	param, ok := s.Param().(meetParams)
	if !ok {
		s.Fatal("Failed to convert test meetParams")
	}

	var err error
	testDuration := effects.DefaultTestDuration
	metricInterval := effects.DefaultTimeInterval

	if varValue, ok := s.Var("videoconferencing.test_duration"); ok {
		testDuration, err = strconv.Atoi(varValue)
		if err != nil || testDuration <= 0 {
			s.Fatal("Failed to parse videoconferencing.test_duration: ", err)
		}
		// Interval set to 1 second when using custom test duration.
		metricInterval = 1 * time.Second

	}

	fixt := s.FixtValue().(fixture.BenchmarkSetUpFixtureData)
	cr := fixt.Chrome
	defer faillog.DumpUITreeWithScreenshotOnError(closeCtx, s.OutDir(), s.HasError, cr, "ui_tree")

	r := power.NewRecorder(ctx, metricInterval, s.OutDir(), s.TestName())
	defer r.Close(closeCtx)

	// Record Memory usage.
	p := perf.NewValues()

	initMemUsage, err := effects.GetSwapAndRSSBytes(ctx)
	if err != nil {
		s.Error("Failed to read memory usage: ", err)
	} else {
		p.Set(perf.Metric{
			Name:      "InitialMemoryUsage",
			Unit:      "Byte",
			Direction: perf.SmallerIsBetter,
			Multiple:  false},
			float64(initMemUsage))
		testing.ContextLog(ctx, "Initial Memory usage: ", initMemUsage)
	}

	testing.ContextLog(ctx, "Opening Meet")
	conn, br, cleanup, err := browserfixt.SetUpWithURL(ctx, cr, browser.TypeAsh, chrome.NewTabURL)
	if err != nil {
		s.Fatal("Failed to launch browser: ", err)
	}
	defer cleanup(closeCtx)

	// Create a new meeting with bots.
	var gm *googlemeet.GoogleMeet
	if param.botCount > 0 {
		creds := s.RequiredVar("ui.bond_credentials")
		bc, err := bond.NewClient(ctx, bond.WithCredsJSON([]byte(creds)))
		if err != nil {
			s.Fatal("Failed to create a bond client: ", err)
		}
		defer bc.Close()

		var meetingCode string
		func() {
			sctx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			meetingCode, err = bc.CreateConference(sctx)
			if err != nil {
				s.Fatal("Failed to create a conference room: ", err)
			}
		}()

		testing.ContextLog(ctx, "Meeting created with code: ", meetingCode)

		func() {
			sctx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			_, _, err := bc.AddBots(sctx, meetingCode, param.botCount, botDuration)
			if err != nil {
				s.Fatal("Failed to add bots: ", err)
			}
		}()

		gm, err = googlemeet.JoinMeeting(ctx, cr, br, conn, meetingCode,
			map[string]string{
				"e": "ForceSegmentationModelVariant::GpuMid",
			}, googlemeet.WithAllPermissions)
		if err != nil {
			s.Fatal("Failed to join meeting: ", err)
		}

	} else {
		gm, err = googlemeet.StartNewMeeting(ctx, cr, br, conn,
			map[string]string{
				"e": "ForceSegmentationModelVariant::GpuMid",
			}, googlemeet.WithAllPermissions)
		if err != nil {
			s.Fatal("Failed to start meeting: ", err)
		}
	}

	defer gm.Close(closeCtx)

	// Configure Meeting.
	if err := uiauto.Combine("Configure Google Meet",
		gm.EnterFullScreen,
		gm.MuteIfMicAvailable,
		gm.ChangeSettings(
			gm.SetLeaveEmptyCalls(false),
			gm.SetAdjustVideoLighting(param.appRelight),
			gm.SetSendResolution(googlemeet.ResolutionHD720P),
		),
		gm.ApplyVideoEffects(gm.SetEffectBlur(param.appBlur)),
	)(ctx); err != nil {
		s.Fatal("Failed to configure Meet: ", err)
	}

	cleanupApply, err := effects.ApplyPlatformEffects(ctx, param.platformBlur, param.platformRelight, param.modelType)
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

	if err := r.Start(ctx); err != nil {
		s.Fatal("Cannot start collecting power metrics: ", err)
	}

	// Take initial power snapshot.
	powerStart := time.Now()
	raplEnergyBefore, err := power.NewRAPLSnapshot()
	if err != nil {
		testing.ContextLog(ctx, "RAPL Energy status is not available for this board: ", err)
	}

	memoryChannel := make(chan effects.PeakMemoryResult)
	go effects.GetMaxMemoryUsage(ctx, memoryChannel, testDuration)

	// Capture metrics.
	if err = effects.ReportFramePerfMetrics(ctx, p, gm.Conn(), s.DataPath("effects_frame_metrics.js"), testDuration); err != nil {
		s.Error("Failed to report fps and frame duration metrics: ", err)
	}

	if err := r.Finish(ctx); err != nil {
		s.Error("Cannot finish collecting power metrics: ", err)
	}

	// TODO: Remove manual power metric collection
	powerEnd := time.Now()
	powerDuration := int(powerEnd.Sub(powerStart).Seconds())
	if raplEnergyBefore != nil {
		if effects.ReportPowerDiffMetrics(ctx, p, raplEnergyBefore, powerDuration) != nil {
			s.Error("Failed to report power metrics: ", err)
		}
	}

	if err = effects.ReportMemoryMetrics(ctx, p, memoryChannel); err != nil {
		s.Error("Failed to report memory metrics: ", err)
	}
	if err := p.Save(s.OutDir()); err != nil {
		s.Error("Cannot save perf data: ", err)
	}

}
