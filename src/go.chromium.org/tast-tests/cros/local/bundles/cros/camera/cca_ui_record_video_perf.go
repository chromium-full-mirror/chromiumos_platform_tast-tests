// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/camera/cca"
	"go.chromium.org/tast-tests/cros/local/camera/testutil"
	"go.chromium.org/tast-tests/cros/local/cpu"
	mediacpu "go.chromium.org/tast-tests/cros/local/media/cpu"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CCAUIRecordVideoPerf,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Record video and measure the performance including CPU, power and preview FPS",
		Contacts:     []string{"chromeos-camera-eng@google.com", "wtlee@chromium.org"},
		Attr:         []string{"group:crosbolt", "crosbolt_perbuild"},
		SoftwareDeps: []string{"camera_app", "chrome", caps.BuiltinOrVividCamera},
		Timeout:      20 * time.Minute,
		Params: []testing.Param{{
			Name:    "fake_hal",
			Fixture: "ccaTestBridgeReadyWithFakeHALCamera",
		}},
		BugComponent: "b:978428",
	})
}

// CCAUIRecordVideoPerf records video and measure performance.
func CCAUIRecordVideoPerf(ctx context.Context, s *testing.State) {
	startApp := s.FixtValue().(cca.FixtureData).StartApp
	stopApp := s.FixtValue().(cca.FixtureData).StopApp
	perfValues := perf.NewValues()

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cleanUpBenchmark, err := mediacpu.SetUpBenchmark(ctx)
	if err != nil {
		s.Fatal("Failed to set up benchmark: ", err)
	}
	defer cleanUpBenchmark(cleanupCtx)

	if err := cpu.WaitUntilIdle(ctx); err != nil {
		s.Fatal("Failed to wait CPU idle: ", err)
	}

	// Simulates a 4K camera. Need to add other resolutions to satisfy the minimal requirement for camera3 API.
	if err := testutil.WriteFakeHALConfig(ctx, testutil.FakeHALConfig{
		Cameras: []testutil.FakeCameraConfig{
			{ID: 1, Connected: true, SupportedFormats: []*testutil.FakeCameraFormatsConfig{
				{
					Width:      3840,
					Height:     2160,
					FrameRates: []int{60, 30},
				},
				{
					Width:      1920,
					Height:     1080,
					FrameRates: []int{30},
				},
				{
					Width:      1280,
					Height:     960,
					FrameRates: []int{30},
				},
				{
					Width:      1280,
					Height:     720,
					FrameRates: []int{30},
				},
				{
					Width:      640,
					Height:     480,
					FrameRates: []int{30},
				},
				{
					Width:      640,
					Height:     360,
					FrameRates: []int{30},
				},
				{
					Width:      320,
					Height:     240,
					FrameRates: []int{30},
				},
			}},
		},
	}); err != nil {
		s.Fatal("Failed to write fake HAL config: ", err)
	}

	app, err := startApp(ctx)
	if err != nil {
		s.Fatal("Failed to open CCA: ", err)
	}
	defer func(cleanupCtx context.Context) {
		if err := stopApp(cleanupCtx, s.HasError()); err != nil {
			s.Fatal("Failed to close CCA: ", err)
		}
	}(cleanupCtx)

	if err := app.SwitchMode(ctx, cca.Video); err != nil {
		s.Fatal("Failed to switch to video mode: ", err)
	}

	// Switch to the 60 FPS button for 4K resolution.
	if err := switchTo60FPS(ctx, app); err != nil {
		s.Fatal("Failed to switch to 60 fps: ", err)
	}

	baselineFPS := 0.0
	baselineCPU := 0.0
	baselinePower := 0.0
	for _, tc := range []struct {
		name      string
		setupFunc func(context.Context, *cca.App) error
		baseline  bool
	}{
		{"baseline-60fps", setupBaseline, true},
		{"hal-multi-streams-60fps", setupHALMultiStreams, false},
		{"chrome-multi-streams-60fps", setupChromeMultiStreams, false},
	} {
		s.Run(ctx, tc.name, func(ctx context.Context, s *testing.State) {
			if err := tc.setupFunc(ctx, app); err != nil {
				s.Fatal("Failed to run setup function: ", err)
			}

			if err := app.RunThroughCameras(ctx, func(facing cca.Facing) error {
				// Record video and measure CPU usage.
				start, err := app.StartRecording(ctx, cca.TimerOff)
				if err != nil {
					return errors.Wrap(err, "failed to start recording")
				}

				fpsObserver, err := app.FPSObserver(ctx)
				if err != nil {
					return errors.Wrap(err, "failed to get FPS observer")
				}
				defer fpsObserver.Stop(cleanupCtx)

				stabilizationDuration := 5 * time.Second
				testing.ContextLog(ctx, "Sleeping to wait for CPU usage to stabilize for ", stabilizationDuration)
				// GoBigSleepLint: Sleep to stabilize CPU before measuring the CPU usage.
				if err := testing.Sleep(ctx, stabilizationDuration); err != nil {
					return errors.Wrap(err, "failed to sleep for CPU usage to stabilize")
				}

				usage, err := mediacpu.MeasureUsage(ctx, 30*time.Second)
				if err != nil {
					return errors.Wrap(err, "failed to measure CPU usage")
				}

				fps, err := fpsObserver.AverageFPS(ctx)
				if err != nil {
					return errors.Wrap(err, "failed to get average FPS")
				}

				_, _, err = app.StopRecording(ctx, cca.TimerOff, start)
				if err != nil {
					return errors.Wrap(err, "failed to stop recording")
				}

				fpsMetric := fmt.Sprintf("preview-fps-%v-facing-%s", tc.name, facing)
				perfValues.Set(perf.Metric{
					Name:      fpsMetric,
					Unit:      "fps",
					Direction: perf.BiggerIsBetter,
				}, fps)

				if tc.baseline {
					baselineFPS = fps
				} else {
					diffFPS := (fps - baselineFPS) / baselineFPS * 100
					diffFPSMetrics := fmt.Sprintf("diff-preview-fps-%v-facing-%s", tc.name, facing)
					perfValues.Set(perf.Metric{
						Name:      diffFPSMetrics,
						Unit:      "percent",
						Direction: perf.BiggerIsBetter,
					}, diffFPS)
				}

				cpu, _ := usage["cpu"]
				cpuMetric := fmt.Sprintf("cpu-%v-facing-%s", tc.name, facing)
				perfValues.Set(perf.Metric{
					Name:      cpuMetric,
					Unit:      "percent",
					Direction: perf.SmallerIsBetter,
				}, cpu)

				if tc.baseline {
					baselineCPU = cpu
				} else {
					diffCPU := (cpu - baselineCPU) / baselineCPU * 100
					diffCPUMetrics := fmt.Sprintf("diff-cpu-%v-facing-%s", tc.name, facing)
					perfValues.Set(perf.Metric{
						Name:      diffCPUMetrics,
						Unit:      "percent",
						Direction: perf.SmallerIsBetter,
					}, diffCPU)
				}

				if power, ok := usage["power"]; ok {
					powerMetric := fmt.Sprintf("power-%v-facing-%s", tc.name, facing)
					perfValues.Set(perf.Metric{
						Name:      powerMetric,
						Unit:      "Watts",
						Direction: perf.SmallerIsBetter,
					}, power)

					if tc.baseline {
						baselinePower = power
					} else {
						diffPower := (power - baselinePower) / baselinePower * 100
						diffPowerMetrics := fmt.Sprintf("diff-power-%v-facing-%s", tc.name, facing)
						perfValues.Set(perf.Metric{
							Name:      diffPowerMetrics,
							Unit:      "percent",
							Direction: perf.SmallerIsBetter,
						}, diffPower)
					}
				}
				return nil
			}); err != nil {
				s.Fatal("Failed to record video though cameras: ", err)
			}
		})
	}

	if err := perfValues.Save(s.OutDir()); err != nil {
		s.Fatal("Failed to save perf metrics: ", err)
	}
}

func setupBaseline(ctx context.Context, app *cca.App) error {
	if err := app.SetEnableMultiStreamRecording(ctx, false); err != nil {
		return errors.Wrap(err, "failed to disable multi-stream recording")
	}
	if err := app.SetEnableMultiStreamRecordingChrome(ctx, false); err != nil {
		return errors.Wrap(err, "failed to disable multi-stream-chrome recording")
	}
	return nil
}

func setupHALMultiStreams(ctx context.Context, app *cca.App) error {
	if err := app.SetEnableMultiStreamRecording(ctx, true); err != nil {
		return errors.Wrap(err, "failed to enable multi-stream recording")
	}
	return nil
}

func setupChromeMultiStreams(ctx context.Context, app *cca.App) error {
	if err := app.SetEnableMultiStreamRecordingChrome(ctx, true); err != nil {
		return errors.Wrap(err, "failed to enable multi-stream-chrome recording")
	}
	return nil
}

func switchTo60FPS(ctx context.Context, app *cca.App) error {
	if err := app.OpenSettingMenu(ctx, cca.MainMenu); err != nil {
		return err
	}
	if err := app.OpenSettingMenu(ctx, cca.VideoResolutionMenu); err != nil {
		return err
	}
	if err := app.ClickWithIndex(ctx, cca.FPS60Buttons, 0); err != nil {
		return err
	}
	if err := app.WaitForVisibleState(ctx, cca.SettingsButton, true); err != nil {
		return err
	}
	return nil
}
