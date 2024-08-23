// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package recorderapp

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/recorderapp/data"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/power"
	powersetup "go.chromium.org/tast-tests/cros/local/power/setup"
	"go.chromium.org/tast-tests/cros/local/recorderapp"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type playbackPowerTestParam struct {
	action func(ctx context.Context, app *recorderapp.App) error
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         PlaybackPower,
		Desc:         "Collect power metrics for playback-related use cases in Recorder App",
		Contacts:     []string{"chromeos-recorder-app@google.com", "kamchonlathorn@chromium.org"},
		BugComponent: "b:1522466", // ChromeOS > Platform > Technologies > Audio > Recorder App
		Timeout:      recorderapp.PowerTimeParams.Total + power.RecorderTimeout + 2*time.Minute,
		Attr:         []string{"group:crosbolt", "crosbolt_perbuild"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "powerAshWithRecorderApp",
		Data:         []string{data.ShortNewsRecording},
		Params: []testing.Param{
			{
				Name: "idle",
				Val: playbackPowerTestParam{
					action: testIdle,
				},
			},
			{
				Name: "title_suggestion",
				Val: playbackPowerTestParam{
					action: testSuggestTitle,
				},
				ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(1)), // Only enable on CBX devices.
			},
			{
				Name: "summarization",
				Val: playbackPowerTestParam{
					action: testSummarize,
				},
				ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(1)), // Only enable on CBX devices.
			},
		},
	})
}

func PlaybackPower(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(powersetup.PowerUIFixtureData).Cr
	testAction := s.Param().(playbackPowerTestParam).action

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	r := power.NewRecorder(ctx, recorderapp.PowerTimeParams.Interval, s.OutDir(), s.TestName())
	defer r.Close(cleanupCtx)

	// Cool down the test device.
	if err := r.Cooldown(ctx); err != nil {
		s.Error("Cooldown failed: ", err)
	}

	// Start collecting power metrics.
	if err := r.Start(ctx); err != nil {
		s.Fatal("Failed to start collecting power metrics: ", err)
	}

	// Start the Recorder App.
	setup := recorderapp.Setup{
		Config:          recorderapp.LaunchConfig{SummaryForceEnabled: true},
		PreloadDataPath: s.DataPath(data.ShortNewsRecording),
	}
	app, err := recorderapp.StartAppWithSetup(ctx, cr, setup)
	if err != nil {
		s.Fatal("Failed to launch Recorder App: ", err)
	}
	defer app.Close(cleanupCtx)

	// Repeatedly perform the test action until exceed the record duration.
	recordDuration := recorderapp.PowerTimeParams.Total
	var iteration = 0

	for endTime := time.Now().Add(recordDuration); time.Now().Before(endTime); {
		testing.ContextLogf(ctx, "Running iteration #%d", iteration)
		if err := app.PlayFirstRecording()(ctx); err != nil {
			s.Fatal("Failed to play the first recording: ", err)
		}

		// Perform the action based on the test parameters.
		if err := testAction(ctx, app); err != nil {
			s.Fatal("Failed to perform the action: ", err)
		}

		// Wait until the playback is finish. Use the recording duration with
		// some buffer as timeout.
		playbackTimeout := data.ShortNewsRecordingDuration + 5*time.Second
		if err := app.WaitUntilPlaybackFinished(playbackTimeout)(ctx); err != nil {
			s.Fatal("Failed to wait until the playback finished: ", err)
		}

		if err := app.GoBackToMainPage()(ctx); err != nil {
			s.Fatal("Failed to go back to main page: ", err)
		}

		iteration++
	}

	// Finish the power measurement and collect the metrics.
	if err := r.Finish(ctx); err != nil {
		s.Error("Failed to collect power metrics: ", err)
	}
}

func testIdle(_ context.Context, _ *recorderapp.App) error {
	// In idle test, we only wait until the playback finish.
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
