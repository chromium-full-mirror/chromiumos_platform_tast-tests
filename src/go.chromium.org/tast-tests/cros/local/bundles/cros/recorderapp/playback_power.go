// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package recorderapp

import (
	"context"
	"os"
	"path"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/recorderapp/data"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/power"
	powersetup "go.chromium.org/tast-tests/cros/local/power/setup"
	"go.chromium.org/tast-tests/cros/local/recorderapp"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"
	"go.chromium.org/tast-tests/cros/local/ui/docscuj"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
	"golang.org/x/sync/errgroup"
)

type playbackPowerTestParam struct {
	// testAction is the feature we want to test repeatedly.
	testAction func(ctx context.Context, app *recorderapp.App) error
	// mainAction controls the scenario and the life span of testAction.
	mainAction   func(ctx context.Context, s *testing.State, start chan struct{}) error
	launchConfig recorderapp.LaunchConfig
	recording    string
	duration     time.Duration
}

const (
	// It takes around 30 minutes to conduct a complete DocsCUJ.
	docsCUJTestTimeout         = 30 * time.Minute
	playbackTestActionDuration = 3 * time.Minute
)

var (
	recordPowerTestTimeout = recorderapp.PowerTimeParams.Total + power.RecorderTimeout + 4*time.Minute
	cbxGen2Models          = []string{"rauru", "navi"}
)

func init() {
	// Test timeout depends on mainAction.
	testing.AddTest(&testing.Test{
		Func:         PlaybackPower,
		Desc:         "Collect power metrics for playback-related use cases in Recorder App",
		Contacts:     []string{"chromeos-recorder-app@google.com", "kamchonlathorn@chromium.org"},
		BugComponent: "b:1522466", // ChromeOS > Platform > Technologies > Audio > Recorder App
		Attr:         []string{"group:crosbolt", "crosbolt_perbuild"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "powerAshWithRecorderApp",
		Data:         []string{cujrecorder.SystemTraceConfigFile},
		Params: []testing.Param{
			{
				Name: "idle",
				Val: playbackPowerTestParam{
					testAction:   testIdle,
					mainAction:   recordPower,
					launchConfig: recorderapp.LaunchConfig{},
					duration:     data.ShortNewsRecordingDuration,
					recording:    data.ShortNewsRecording,
				},
				ExtraData: []string{data.ShortNewsRecording},
				Timeout:   recordPowerTestTimeout,
			},
			{
				Name: "idle_with_1k_tokens_input",
				Val: playbackPowerTestParam{
					testAction:   testIdle,
					mainAction:   runDocsCUJ,
					launchConfig: recorderapp.LaunchConfig{SummaryForceEnabled: true},
					duration:     data.Input1KRecordingDuration,
					recording:    data.Input1KRecording,
				},
				ExtraData: []string{data.Input1KRecording},
				Timeout:   docsCUJTestTimeout,
			},
			{
				Name: "idle_with_3k_tokens_input",
				Val: playbackPowerTestParam{
					testAction:   testIdle,
					mainAction:   runDocsCUJ,
					launchConfig: recorderapp.LaunchConfig{SummaryForceEnabled: true},
					duration:     data.Input3KRecordingDuration,
					recording:    data.Input3KRecording,
				},
				ExtraData: []string{data.Input3KRecording},
				Timeout:   docsCUJTestTimeout,
			},
			{
				Name: "idle_with_7k5_tokens_input",
				Val: playbackPowerTestParam{
					testAction:   testIdle,
					mainAction:   runDocsCUJ,
					launchConfig: recorderapp.LaunchConfig{SummaryForceEnabled: true},
					duration:     data.Input7K5RecordingDuration,
					recording:    data.Input7K5Recording,
				},
				ExtraData: []string{data.Input7K5Recording},
				Timeout:   docsCUJTestTimeout,
			},
			{
				Name: "idle_with_11k_tokens_input",
				Val: playbackPowerTestParam{
					testAction:   testIdle,
					mainAction:   runDocsCUJ,
					launchConfig: recorderapp.LaunchConfig{SummaryForceEnabled: true},
					duration:     data.Input11KRecordingDuration,
					recording:    data.Input11KRecording,
				},
				ExtraData: []string{data.Input11KRecording},
				Timeout:   docsCUJTestTimeout,
			},
			{
				Name: "title_suggestion",
				Val: playbackPowerTestParam{
					testAction:   testSuggestTitle,
					mainAction:   recordPower,
					launchConfig: recorderapp.LaunchConfig{SummaryForceEnabled: true},
					duration:     data.ShortNewsRecordingDuration,
					recording:    data.ShortNewsRecording,
				},
				ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(1)), // Only enable on CBX devices.
				ExtraData:         []string{data.ShortNewsRecording},
				Timeout:           recordPowerTestTimeout,
			},
			{
				Name: "title_suggestion_with_1k_tokens_input",
				Val: playbackPowerTestParam{
					testAction:   testSuggestTitle,
					mainAction:   runDocsCUJ,
					launchConfig: recorderapp.LaunchConfig{SummaryForceEnabled: true},
					duration:     data.Input1KRecordingDuration,
					recording:    data.Input1KRecording,
				},
				ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(1)), // Only enable on CBX devices.
				ExtraData:         []string{data.Input1KRecording},
				Timeout:           docsCUJTestTimeout,
			},
			{
				Name: "title_suggestion_with_3k_tokens_input",
				Val: playbackPowerTestParam{
					testAction:   testSuggestTitle,
					mainAction:   runDocsCUJ,
					launchConfig: recorderapp.LaunchConfig{SummaryForceEnabled: true},
					duration:     data.Input3KRecordingDuration,
					recording:    data.Input3KRecording,
				},
				ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(1)), // Only enable on CBX devices.
				ExtraData:         []string{data.Input3KRecording},
				Timeout:           docsCUJTestTimeout,
			},
			{
				Name: "title_suggestion_with_7k5_tokens_input",
				Val: playbackPowerTestParam{
					testAction:   testSuggestTitle,
					mainAction:   runDocsCUJ,
					launchConfig: recorderapp.LaunchConfig{SummaryForceEnabled: true},
					duration:     data.Input7K5RecordingDuration,
					recording:    data.Input7K5Recording,
				},
				ExtraHardwareDeps: hwdep.D(hwdep.Model(cbxGen2Models...)), // Only enable on CBX gen2 devices.
				ExtraData:         []string{data.Input7K5Recording},
				Timeout:           docsCUJTestTimeout,
			},
			{
				Name: "title_suggestion_with_11k_tokens_input",
				Val: playbackPowerTestParam{
					testAction:   testSuggestTitle,
					mainAction:   runDocsCUJ,
					launchConfig: recorderapp.LaunchConfig{SummaryForceEnabled: true},
					duration:     data.Input11KRecordingDuration,
					recording:    data.Input11KRecording,
				},
				ExtraHardwareDeps: hwdep.D(hwdep.Model(cbxGen2Models...)), // Only enable on CBX gen2 devices.
				ExtraData:         []string{data.Input11KRecording},
				Timeout:           docsCUJTestTimeout,
			},
			{
				Name: "summarization",
				Val: playbackPowerTestParam{
					testAction:   testSummarize,
					mainAction:   recordPower,
					launchConfig: recorderapp.LaunchConfig{SummaryForceEnabled: true},
					duration:     data.ShortNewsRecordingDuration,
					recording:    data.ShortNewsRecording,
				},
				ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(1)), // Only enable on CBX devices.
				ExtraData:         []string{data.ShortNewsRecording},
				Timeout:           recordPowerTestTimeout,
			},
			{
				Name: "summarization_with_1k_tokens_input",
				Val: playbackPowerTestParam{
					testAction:   testSummarize,
					mainAction:   runDocsCUJ,
					launchConfig: recorderapp.LaunchConfig{SummaryForceEnabled: true},
					duration:     data.Input1KRecordingDuration,
					recording:    data.Input1KRecording,
				},
				ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(1)), // Only enable on CBX devices.
				ExtraData:         []string{data.Input1KRecording},
				Timeout:           docsCUJTestTimeout,
			},
			{
				Name: "summarization_with_3k_tokens_input",
				Val: playbackPowerTestParam{
					testAction:   testSummarize,
					mainAction:   runDocsCUJ,
					launchConfig: recorderapp.LaunchConfig{SummaryForceEnabled: true},
					duration:     data.Input3KRecordingDuration,
					recording:    data.Input3KRecording,
				},
				ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(1)), // Only enable on CBX devices.
				ExtraData:         []string{data.Input3KRecording},
				Timeout:           docsCUJTestTimeout,
			},
			{
				Name: "summarization_with_7k5_tokens_input",
				Val: playbackPowerTestParam{
					testAction:   testSummarize,
					mainAction:   runDocsCUJ,
					launchConfig: recorderapp.LaunchConfig{SummaryForceEnabled: true},
					duration:     data.Input7K5RecordingDuration,
					recording:    data.Input7K5Recording,
				},
				ExtraHardwareDeps: hwdep.D(hwdep.Model(cbxGen2Models...)), // Only enable on CBX gen2 devices.
				ExtraData:         []string{data.Input7K5Recording},
				Timeout:           docsCUJTestTimeout,
			},
			{
				Name: "summarization_with_11k_tokens_input",
				Val: playbackPowerTestParam{
					testAction:   testSummarize,
					mainAction:   runDocsCUJ,
					launchConfig: recorderapp.LaunchConfig{SummaryForceEnabled: true},
					duration:     data.Input11KRecordingDuration,
					recording:    data.Input11KRecording,
				},
				ExtraHardwareDeps: hwdep.D(hwdep.Model(cbxGen2Models...)), // Only enable on CBX gen2 devices.
				ExtraData:         []string{data.Input11KRecording},
				Timeout:           docsCUJTestTimeout,
			},
		},
	})
}

func PlaybackPower(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(powersetup.PowerUIFixtureData).Cr
	testAction := s.Param().(playbackPowerTestParam).testAction

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Start the Recorder App.
	setup := recorderapp.Setup{
		Config:          s.Param().(playbackPowerTestParam).launchConfig,
		PreloadDataPath: s.DataPath(s.Param().(playbackPowerTestParam).recording),
	}
	app, err := recorderapp.StartAppWithSetup(ctx, cr, setup)
	if err != nil {
		s.Fatal("Failed to launch Recorder App: ", err)
	}
	defer app.Close(cleanupCtx)

	app.SetWindowStateAndWait(ctx, ash.WindowStateFloated)

	// Make sure testAction can be triggered repeatedly.
	// Run in standalone goroutine and wait main thread ready to start.
	// Before exit, main thread will wait for test action finish to prevent UI error.
	start := make(chan struct{})
	done := make(chan struct{})
	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		iteration := 0
		<-start
		testing.ContextLog(ctx, "Test action started")
		for {
			select {
			case <-done:
				testing.ContextLog(ctx, "Main thread done")
				return nil
			default:
				testing.ContextLogf(ctx, "Running iteration #%d", iteration)
				if err := app.PlayFirstRecording()(ctx); err != nil {
					return errors.Wrap(err, "failed to play the first recording")
				}

				now := time.Now()
				// Perform the action based on the test parameters.
				if err := testAction(ctx, app); err != nil {
					return errors.Wrap(err, "failed to perform testAction")
				}

				// Run playback for some period of time to collect metrics.
				// We don't need to wait the entire playback finished since
				// some playback run too long. (e.g. above 30 minutes)
				leftTime := playbackTestActionDuration - time.Since(now)
				if leftTime > 0 {
					playbackTimeout := minDuration(leftTime,
						s.Param().(playbackPowerTestParam).duration)
					// GoBigSleepLint: run playback for some period of time.
					testing.Sleep(ctx, playbackTimeout)
				}

				if err := app.GoBackToMainPage()(ctx); err != nil {
					return errors.Wrap(err, "failed to go back to the main page")
				}

				iteration++
			}
		}
	})

	if err := s.Param().(playbackPowerTestParam).mainAction(ctx, s, start); err != nil {
		s.Fatal("Main action failed: ", err)
	}

	close(done)
	if err := eg.Wait(); err != nil {
		s.Fatal("Test action failed: ", err)
	}
}

func runDocsCUJ(ctx context.Context, s *testing.State, start chan struct{}) error {
	traceConfigPath := s.DataPath(cujrecorder.SystemTraceConfigFile)
	folder := path.Join(s.OutDir(), "docs_cuj")
	if err := os.MkdirAll(folder, 0755); err != nil && !os.IsExist(err) {
		return errors.Wrap(err, "failed to create folder for DocsCUJ")
	}
	close(start)

	cr := s.FixtValue().(powersetup.PowerUIFixtureData).Cr
	if _, err := docscuj.Run(ctx, cr, docscuj.TestParam{}, folder, traceConfigPath, s.TestName(), cujrecorder.RecorderOptions{
		// Boot and shutdown metrics are not important in the test and
		// it is error-prone when run test at local.
		SkipBootShutdownMetrics: true,
	}); err != nil {
		return errors.Wrap(err, "failed to run DocsCUJ")
	}

	// We don't need to save recorder here,
	// since the cujrecorder include power metrics already.
	return nil
}

func recordPower(ctx context.Context, s *testing.State, start chan struct{}) error {
	r := power.NewRecorder(ctx, recorderapp.PowerTimeParams.Interval, s.OutDir(), s.TestName())
	defer r.Close(ctx)
	// Cooldown to have more accurate result.
	if err := r.Cooldown(ctx); err != nil {
		return errors.Wrap(err, "failed to cooldown")
	}

	// Start collecting power metrics.
	if err := r.Start(ctx); err != nil {
		return errors.Wrap(err, "failed to start recorder")
	}
	close(start)

	recordDuration := recorderapp.PowerTimeParams.Total
	// GoBigSleepLint: wait for metrics collection.
	testing.Sleep(ctx, recordDuration)

	// Finish the power measurement and collect the metrics.
	if err := r.Finish(ctx); err != nil {
		return errors.Wrap(err, "failed to finish recorder")
	}

	return nil
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

func testIdle(ctx context.Context, _ *recorderapp.App) error {
	return nil
}

func testSuggestTitle(ctx context.Context, app *recorderapp.App) error {
	testing.ContextLog(ctx, "Requesting title suggestions")
	runDuration, err := runAndMeasureTime(ctx, app.RequestTitleSuggestions())
	if err != nil {
		return errors.Wrap(err, "failed to request the recording title suggestions")
	}
	testing.ContextLogf(ctx, "Title suggestions completed in %d milliseconds", runDuration.Milliseconds())
	return nil
}

func testSummarize(ctx context.Context, app *recorderapp.App) error {
	testing.ContextLog(ctx, "Requesting summary")
	runDuration, err := runAndMeasureTime(ctx, app.RequestRecordingSummary())
	if err != nil {
		return errors.Wrap(err, "failed to request the recording summary")
	}
	testing.ContextLogf(ctx, "Summarization completed in %d milliseconds", runDuration.Milliseconds())
	return nil
}

func runAndMeasureTime(ctx context.Context, action uiauto.Action) (time.Duration, error) {
	startTime := time.Now()
	if err := action(ctx); err != nil {
		return 0, err
	}
	return time.Since(startTime), nil
}
