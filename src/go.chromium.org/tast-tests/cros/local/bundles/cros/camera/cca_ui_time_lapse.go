// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/local/camera/cca"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CCAUITimeLapse,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Opens CCA and verifies time-lapse video recording",
		Contacts:     []string{"chromeos-camera-eng@google.com", "kamchonlathorn@chromium.org"},
		BugComponent: "b:978428", // ChromeOS > Platform > Technologies > Camera > App & Framework
		Attr:         []string{"group:mainline", "informational", "group:camera-libcamera"},
		SoftwareDeps: []string{"camera_app", "chrome", caps.BuiltinOrVividCamera},
		Timeout:      10 * time.Minute,
		Fixture:      "ccaLaunchedWithFakeHALCamera",
	})
}

const timeLapseTolerance = 500 * time.Millisecond

func CCAUITimeLapse(ctx context.Context, s *testing.State) {
	app := s.FixtValue().(cca.FixtureData).App()
	testing.ContextLog(ctx, "Switch to time-lapse mode")
	if err := app.SwitchToTimeLapseMode(ctx); err != nil {
		s.Error("Failed to switch to time-lapse mode")
	}
	for _, tc := range []struct {
		name    string
		run     func(context.Context, *cca.App) error
		timeout time.Duration
	}{
		{"testSimpleRecording", testSimpleRecording, 2 * time.Minute},
		{"testAutoSpeedRecording", testAutoSpeedRecording, 4 * time.Minute},
		{"testRecordInMinimizedWindow", testRecordInMinimizedWindow, time.Minute},
		{"testPauseResume", testPauseResumeTimeLapse, time.Minute},
	} {
		subTestCtx, cancel := context.WithTimeout(ctx, tc.timeout)
		s.Run(subTestCtx, tc.name, func(ctx context.Context, s *testing.State) {
			if err := tc.run(ctx, app); err != nil {
				s.Errorf("Failed to pass %v subtest: %v", tc.name, err)
			}
		})
		cancel()
	}
}

// testSimpleRecording tests recording without interruption and stopping while
// the video is still using the initial time-lapse speed.
func testSimpleRecording(ctx context.Context, app *cca.App) error {
	return recordTimeLapseFor(ctx, app, 10*time.Second)
}

// testAutoSpeedRecording tests recording for longer time until the time-lapse
// speed is updated and the video duration is kept below the maximum time.
func testAutoSpeedRecording(ctx context.Context, app *cca.App) error {
	return recordTimeLapseFor(ctx, app, 3*time.Minute)
}

// testRecordInMinimizedWindow tests that time-lapse video recording continues
// even when CCA window being minimized and restore.
func testRecordInMinimizedWindow(ctx context.Context, app *cca.App) error {
	const activeRecordTime = 5 * time.Second
	const minimizedRecordTime = 10 * time.Second
	// Starts recording a time-lapse video.
	startTime, err := app.StartRecording(ctx, cca.TimerOff)
	if err != nil {
		return errors.Wrap(err, "failed to start recording a time-lapse video")
	}
	if err := performActivityForDuration(ctx, "Record a time-lapse video", activeRecordTime); err != nil {
		return err
	}
	// Minimizes the window and let the recording continue.
	if err := app.MinimizeWindow(ctx); err != nil {
		return errors.Wrap(err, "failed to minimize the window")
	}
	if err := performActivityForDuration(ctx, "Keep recording in the minimized window", minimizedRecordTime); err != nil {
		return err
	}
	// Restores the window and continues recording.
	if err := app.RestoreWindow(ctx); err != nil {
		return errors.Wrap(err, "failed to restore the window")
	}
	if err := app.Focus(ctx); err != nil {
		return errors.Wrap(err, "failed to focus the window")
	}
	if err := performActivityForDuration(ctx, "Record after the window is restored", activeRecordTime); err != nil {
		return err
	}
	// Stops the recording and verifies the video duration.
	fileInfo, _, err := app.StopRecording(ctx, cca.TimerOff, startTime)
	if err != nil {
		return errors.Wrap(err, "failed to stop recording the time-lapse video")
	}
	expectedDuration := activeRecordTime + minimizedRecordTime + activeRecordTime
	return validateTimeLapseDuration(ctx, app, fileInfo.Name(), expectedDuration)
}

// testPauseResumeTimeLapse tests recording time-lapse with pause/resume
// operations during the recording.
func testPauseResumeTimeLapse(ctx context.Context, app *cca.App) error {
	startTime, err := app.StartRecording(ctx, cca.TimerOff)
	if err != nil {
		return errors.Wrap(err, "failed to start recording a time-lapse video")
	}
	if err := performActivityForDuration(ctx, "Record a time-lapse video", 5*time.Second); err != nil {
		return err
	}
	// Pauses the recording for 10 seconds.
	// TODO(b/281625728): Observe on the button UI instead of the state.
	pausedTime, err := app.TriggerStateChange(ctx, "recording-paused", true, func() error {
		if err := app.Click(ctx, cca.VideoPauseResumeButton); err != nil {
			return errors.Wrap(err, "failed to pause the recording")
		}
		return nil
	})
	if err := performActivityForDuration(ctx, "Keep the recording paused", 10*time.Second); err != nil {
		return err
	}
	// Resumes the recording for 5 more seconds.
	resumedTime, err := app.TriggerStateChange(ctx, "recording-paused", false, func() error {
		if err := app.Click(ctx, cca.VideoPauseResumeButton); err != nil {
			return errors.Wrap(err, "failed to resume the recording")
		}
		return nil
	})
	if err := performActivityForDuration(ctx, "Continues recording after resuming", 5*time.Second); err != nil {
		return err
	}
	// Stops the recording and verifies the video duration.
	fileInfo, endTime, err := app.StopRecording(ctx, cca.TimerOff, startTime)
	if err != nil {
		return errors.Wrap(err, "failed to stop recording the time-lapse video")
	}
	pausedDuration := resumedTime.Sub(pausedTime)
	totalDuration := endTime.Sub(startTime)
	expectedDuration := totalDuration - pausedDuration
	return validateTimeLapseDuration(ctx, app, fileInfo.Name(), expectedDuration)
}

// recordTimeLapseFor records in time-lapse mode for |recordTime| and verifies the record time.
func recordTimeLapseFor(ctx context.Context, app *cca.App, recordTime time.Duration) error {
	testing.ContextLogf(ctx, "Recording a time-lapse video for %v seconds", recordTime)
	fileInfo, err := app.RecordVideo(ctx, cca.TimerOff, recordTime)
	if err != nil {
		return errors.Wrap(err, "failed to record a time-lapse video")
	}
	return validateTimeLapseDuration(ctx, app, fileInfo.Name(), recordTime)
}

// validateTimeLapseDuration validates if the duration of the result video
// matches with the recorded time and the recorded speed.
func validateTimeLapseDuration(ctx context.Context, app *cca.App, fileName string, recordTime time.Duration) error {
	filePath, err := app.FilePathInSavedDir(ctx, fileName)
	if err != nil {
		return errors.Wrap(err, "failed to get file path in saved path")
	}
	duration, err := cca.VideoDurationFromHeader(ctx, filePath)
	if err != nil {
		return err
	}
	expectedDuration, err := app.TimeLapseDuration(ctx, recordTime)
	if err != nil {
		return err
	}
	if (duration - expectedDuration).Abs() > timeLapseTolerance {
		return errors.Errorf("incorrect result video duration get %v; want %v with tolerance %v", duration, expectedDuration, timeLapseTolerance)
	}
	return nil
}

// performActivityForDuration sleeps to perform the activity for the specified |duration|.
func performActivityForDuration(ctx context.Context, activity string, duration time.Duration) error {
	testing.ContextLogf(ctx, "%v for %v", activity, duration)
	// GoBigSleepLint: Perform the activity for |duration|.
	if err := testing.Sleep(ctx, duration); err != nil {
		return errors.Wrapf(err, "failed to sleep to %v", activity)
	}
	return nil
}
