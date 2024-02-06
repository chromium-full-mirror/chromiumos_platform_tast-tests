// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package videoconferencing

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/videoconferencing/effects"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/videoconferencing/fakehtml"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/videoconferencing/fixture"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const defaultResolution = 720
const highResolution = 1080

type simpleParams struct {

	// TODO: Default resolution at 720p - enable higher res. by adding more generic images
	// The resolution that the simple meet is going to run at.
	resolution int
	// Whether to enable platform blur or not.
	platformBlur bool
	// Whether to enable platform relight or not.
	platformRelight bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         BenchmarkingHTML,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Captures performance regression and power metrics for VC effects on a HTML embedded video",
		Contacts: []string{
			"chromeos-platform-ml@google.com",
			"zhaon@google.com",
		},
		BugComponent: "b:1212695",
		Attr:         []string{"group:ml_benchmark", "ml_benchmark_nightly"},
		SoftwareDeps: []string{"chrome", "camera_feature_effects"},
		HardwareDeps: hwdep.D(hwdep.SkipOnModel("betty")),
		Timeout:      15 * time.Minute,
		Vars: []string{
			// How many seconds to capture metrics.
			"videoconferencing.test_duration",
		},
		Data: []string{
			"effects_frame_metrics.js",
			"effects_video_script.html",
		},
		Fixture: fixture.LoggedInAndBenchmarkSetupFixture,
		Params: []testing.Param{
			{
				Name: "no_effects_720p",
				Val: simpleParams{
					resolution: defaultResolution,
				},
			},
			{
				Name: "platform_blur_720p",
				Val: simpleParams{
					platformBlur: true,
					resolution:   defaultResolution,
				},
			},
			{
				Name: "platform_relight_720p",
				Val: simpleParams{
					platformRelight: true,
					resolution:      defaultResolution,
				},
			},
			{
				Name: "platform_blur_relight_720p",
				Val: simpleParams{
					resolution:      defaultResolution,
					platformBlur:    true,
					platformRelight: true,
				},
			},
			{
				Name: "platform_blur_1080p",
				Val: simpleParams{
					resolution:   highResolution,
					platformBlur: true,
				},
			},
			{
				Name: "platform_relight_1080p",
				Val: simpleParams{
					resolution:      highResolution,
					platformRelight: true,
				},
			},
			{
				Name: "platform_blur_relight_1080p",
				Val: simpleParams{
					resolution:      highResolution,
					platformBlur:    true,
					platformRelight: true,
				},
			},
		},
	})
}

func BenchmarkingHTML(ctx context.Context, s *testing.State) {
	// Shorten context to allow for cleanup. Reserve one minute in case of power
	// test.
	closeCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Minute)
	defer cancel()

	param, ok := s.Param().(simpleParams)
	if !ok {
		s.Fatal("Failed to convert test simpleParams")
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

	cleanupApply, err := effects.ApplyPlatformEffects(ctx, param.platformBlur, param.platformRelight, effects.KAuto)
	if err != nil {
		s.Fatal("Failed to apply platform effects: ", err)
	}
	defer func() {
		if err := cleanupApply(ctx); err != nil {
			s.Error("Failed to clean up apply platform effects: ", err)
		}
	}()

	r := power.NewRecorder(ctx, metricInterval, s.OutDir(), s.TestName())
	defer r.Close(closeCtx)

	p := perf.NewValues()
	fixt := s.FixtValue().(fixture.BenchmarkSetUpFixtureData)
	cr := fixt.Chrome

	// Record Memory usage.
	initMemUsage, err := effects.ReadSwapAndRSSBytes(ctx)
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

	//  Open video on simple javascript browser.
	testing.ContextLog(ctx, "Opening Simple Meeting")
	srv := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	url := srv.URL + fakehtml.PageURL + strconv.Itoa(param.resolution)

	defer srv.Close()
	conn, err := cr.NewConn(ctx, url, browser.WithNewWindow())

	if err != nil {
		s.Fatal("Failed to open the simple meeting website: ", err)
	}

	defer faillog.DumpUITreeWithScreenshotOnError(closeCtx, s.OutDir(), s.HasError, cr, "ui_tree")

	fakeHTMLUI := fakehtml.NewUI(fixt.TestAPIConn)
	if err := fakeHTMLUI.MayBeAllowCameraAccess(ctx); err != nil {
		s.Fatal("Failed to allow camera access: ", err)
	}

	if err := fakeHTMLUI.EnterFullScreen(ctx); err != nil {
		s.Fatal("Failed to enter full screen: ", err)
	}

	defer faillog.DumpUITreeWithScreenshotOnError(closeCtx, s.OutDir(), s.HasError, cr, "ui_tree")

	testing.ContextLog(ctx, "Letting things settle for 5 seconds before taking metrics")
	// GoBigSleepLint: Allow power and effects to stabilize before taking metrics.
	if err := testing.Sleep(ctx, 5*time.Second); err != nil {
		s.Fatal("Failed to let things settle: ", err)
	}

	if err := r.Start(ctx); err != nil {
		s.Fatal("Cannot start collecting power metrics: ", err)
	}

	memoryChannel := make(chan effects.PeakMemoryResult)
	go effects.ReadMaxMemoryUsage(ctx, memoryChannel, testDuration, metricInterval)

	// Capture metrics.
	if err = effects.ReportFramePerfMetrics(ctx, p, conn, s.DataPath("effects_frame_metrics.js"), testDuration); err != nil {
		s.Error("Failed to report fps and frame duration metrics: ", err)
	}

	if err := r.Finish(ctx); err != nil {
		s.Error("Cannot finish collecting power metrics: ", err)
	}

	if err = effects.ReportMemoryMetrics(ctx, p, memoryChannel); err != nil {
		s.Error("Failed to report memory metrics: ", err)
	}
	if err := p.Save(s.OutDir()); err != nil {
		s.Error("Cannot save perf data: ", err)
	}

}
