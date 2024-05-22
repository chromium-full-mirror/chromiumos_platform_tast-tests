// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/local/camera/cca"
	"go.chromium.org/tast-tests/cros/local/cpu"
	mediacpu "go.chromium.org/tast-tests/cros/local/media/cpu"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type param struct {
	subtestTimeout  time.Duration
	measureDuration time.Duration
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         CCAUIPerf,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Opens CCA and measures the UI performance including CPU and power usage",
		Contacts:     []string{"chromeos-camera-eng@google.com", "wtlee@chromium.org"},
		BugComponent: "b:978428", // ChromeOS > Platform > Technologies > Camera > App & Framework
		Attr:         []string{"group:crosbolt"},
		SoftwareDeps: []string{"camera_app", "chrome", caps.BuiltinOrVividCamera},
		Params: []testing.Param{{
			ExtraAttr: []string{"crosbolt_perbuild"},
			Fixture:   "ccaTestBridgeReady",
			// Six subtests each have 5 mins timeout. 5mins * 6 = 30 minutes.
			Timeout: 30 * time.Minute,
			Val: param{
				subtestTimeout:  5 * time.Minute,
				measureDuration: 20 * time.Second,
			},
		}, {
			Name:      "long",
			ExtraAttr: []string{"crosbolt_nightly"},
			Fixture:   "ccaTestBridgeReady",
			// Six subtests each have 20 mins timeout. 20mins * 6 = 120 minutes.
			Timeout: 120 * time.Minute,
			Val: param{
				subtestTimeout:  20 * time.Minute,
				measureDuration: 5 * time.Minute,
			},
		}, {
			Name:      "long_vcd_utility",
			ExtraAttr: []string{"crosbolt_nightly"},
			Fixture:   "ccaTestBridgeReadyWithVCDInUtilityProcess",
			// Six subtests each have 20 mins timeout. 20mins * 6 = 120 minutes.
			Timeout: 120 * time.Minute,
			Val: param{
				subtestTimeout:  20 * time.Minute,
				measureDuration: 5 * time.Minute,
			},
		}},
	})
}

// CCAUIPerf measure cold/warm start time of CCA and also measure its
// performance through some UI operations.
func CCAUIPerf(ctx context.Context, s *testing.State) {
	p := s.Param().(param)
	perfData := cca.NewPerfData()
	resetChrome := s.FixtValue().(cca.FixtureData).ResetChrome

	// App launch tests.
	startApp := s.FixtValue().(cca.FixtureData).StartApp
	stopApp := s.FixtValue().(cca.FixtureData).StopApp
	appLaunchTestCtx, cancel := context.WithTimeout(ctx, p.subtestTimeout)
	s.Run(appLaunchTestCtx, "testAppLaunch", func(ctx context.Context, s *testing.State) {
		if err := testAppLaunch(ctx, resetChrome, startApp, stopApp, perfData); err != nil {
			s.Error("Failed to pass testAppLaunch subtest: ", err)
		}
	})
	cancel()

	runTestWithApp := s.FixtValue().(cca.FixtureData).RunTestWithApp

	for _, tst := range []struct {
		name     string
		testFunc func(context.Context, *cca.App, *cca.PerfData, time.Duration) error
	}{{
		"testPreviewPerformance",
		testPreviewPerformance,
	}, {
		"testQRPreviewPerformance",
		testQRPreviewPerformance,
	}, {
		"testRecordingPerformance",
		testRecordingPerformance,
	}, {
		"testTakingPicturePerformance",
		testTakingPicturePerformance,
	}, {
		"testGifRecordingPerformance",
		testGifRecordingPerformance,
	}, {
		"testTimeLapseRecordingPerformance",
		testTimeLapseRecordingPerformance,
	}} {
		subTestCtx, cancel := context.WithTimeout(ctx, p.subtestTimeout)
		s.Run(subTestCtx, tst.name, func(ctx context.Context, s *testing.State) {
			if err := preparePerfTest(ctx, resetChrome, func(ctx context.Context) error {
				return runTestWithApp(ctx, func(ctx context.Context, app *cca.App) error {
					testing.ContextLog(ctx, "Fullscreening window")
					if err := app.FullscreenWindow(ctx); err != nil {
						return errors.Wrap(err, "failed to fullscreen window")
					}
					if err := app.WaitForVideoActive(ctx); err != nil {
						return errors.Wrap(err, "preview is inactive after fullscreening window")
					}
					if err := tst.testFunc(ctx, app, perfData, p.measureDuration); err != nil {
						return err
					}
					if err := app.CollectPerfEvents(ctx, perfData); err != nil {
						return errors.Wrap(err, "failed to collect perf events")
					}

					return nil
				}, cca.TestWithAppParams{})
			}); err != nil {
				s.Errorf("Failed to pass %v subtest: %v", tst.name, err)
			}
		})
		cancel()
	}

	if err := perfData.Save(s.OutDir()); err != nil {
		s.Fatal("Failed to save perf metrics: ", err)
	}
}

func testAppLaunch(ctx context.Context, resetChrome cca.ResetChromeFunc,
	startApp cca.StartAppFunc, stopApp cca.StopAppFunc, perfData *cca.PerfData) error {
	return preparePerfTest(ctx, resetChrome, func(ctx context.Context) (retErr error) {
		// Open/close app twice to collect app launched from cold/warm start.
		for i := 0; i < 2; i++ {
			app, err := startApp(ctx)
			if err != nil {
				return errors.Wrap(err, "failed to open CCA")
			}
			if err := app.CollectPerfEvents(ctx, perfData); err != nil {
				return errors.Wrap(err, "failed to collect perf events")
			}
			if err := stopApp(ctx, retErr != nil); err != nil {
				return errors.Wrap(retErr, err.Error())
			}
		}
		return nil
	})
}

func preparePerfTest(ctx context.Context, resetChrome cca.ResetChromeFunc, testBody func(ctx context.Context) error) error {
	// Reset chrome to clean cached web assembly compilation result.
	if err := resetChrome(ctx); err != nil {
		return errors.Wrap(err, "failed to reset chrome before running performance test")
	}

	cleanUpBenchmark, err := mediacpu.SetUpBenchmark(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to set up benchmark")
	}
	defer cleanUpBenchmark(ctx)
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Prevents the CPU usage measurements from being affected by any previous tests.
	if err := cpu.WaitUntilIdle(ctx); err != nil {
		return errors.Wrap(err, "failed to idle")
	}

	return testBody(ctx)
}

func testPreviewPerformance(ctx context.Context, app *cca.App, perfData *cca.PerfData, measureDuration time.Duration) error {
	return app.RunThroughCameras(ctx, func(facing cca.Facing) error {
		return cca.MeasurePreviewPerformance(ctx, app, perfData, facing, measureDuration)
	})
}

func testQRPreviewPerformance(ctx context.Context, app *cca.App, perfData *cca.PerfData, measureDuration time.Duration) error {
	return app.RunThroughCameras(ctx, func(facing cca.Facing) error {
		return cca.MeasureQRPreviewPerformance(ctx, app, perfData, facing, measureDuration)
	})
}

func testRecordingPerformance(ctx context.Context, app *cca.App, perfData *cca.PerfData, measureDuration time.Duration) error {
	return app.RunThroughCameras(ctx, func(facing cca.Facing) error {
		return cca.MeasureRecordingPerformance(ctx, app, perfData, facing, measureDuration)
	})
}

func testTakingPicturePerformance(ctx context.Context, app *cca.App, perfData *cca.PerfData, measureDuration time.Duration) error {
	return app.RunThroughCameras(ctx, func(facing cca.Facing) error {
		return cca.MeasureTakingPicturePerformance(ctx, app, measureDuration)
	})
}

func testGifRecordingPerformance(ctx context.Context, app *cca.App, perfData *cca.PerfData, measureDuration time.Duration) error {
	// TODO(b/201335131): Measure performance of per camera facing test
	// without cached web assembly result.
	return cca.MeasureGifRecordingPerformance(ctx, app, measureDuration)
}

func testTimeLapseRecordingPerformance(ctx context.Context, app *cca.App, perfData *cca.PerfData, measureDuration time.Duration) error {
	return app.RunThroughCameras(ctx, func(facing cca.Facing) error {
		return cca.MeasureTimeLapsePerformance(ctx, app, perfData, facing, measureDuration)
	})
}
