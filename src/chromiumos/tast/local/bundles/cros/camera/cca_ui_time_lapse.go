// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"chromiumos/tast/common/media/caps"
	"chromiumos/tast/local/camera/cca"
	"context"
	"time"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CCAUITimeLapse,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Opens CCA and verifies time-lapse video recording",
		Contacts:     []string{"chromeos-camera-eng@google.com", "kamchonlathorn@chromium.org"},
		Attr:         []string{"group:mainline", "informational", "group:camera-libcamera"},
		SoftwareDeps: []string{"camera_app", "chrome", caps.BuiltinOrVividCamera},
		Timeout:      8 * time.Minute,
		Fixture:      "ccaLaunchedWithTimeLapseOnFakeHALCamera",
		BugComponent: "b:978428",
	})
}

const timeLapseTolerance = 300 * time.Millisecond

func CCAUITimeLapse(ctx context.Context, s *testing.State) {
	app := s.FixtValue().(cca.FixtureData).App()
	testing.ContextLog(ctx, "Switch to time-lapse mode")
	if err := app.SwitchToTimeLapseMode(ctx); err != nil {
		s.Error("Failed to switch to time-lapse mode")
	}
	// TODO(b/236800499): Test pausing/resuming the recording.
	// TODO(b/280392334): Test recording while the window is minimized.
	for _, tc := range []struct {
		name    string
		run     func(context.Context, *cca.App) error
		timeout time.Duration
	}{
		{"testSimpleRecording", testSimpleRecording, 2 * time.Minute},
		{"testAutoSpeedRecording", testAutoSpeedRecording, 4 * time.Minute},
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

// recordTimeLapseFor records in time-lapse mode for |recordTime| and verifies the record time.
func recordTimeLapseFor(ctx context.Context, app *cca.App, recordTime time.Duration) error {
	testing.ContextLogf(ctx, "Recording a time-lapse video for %v seconds", recordTime)
	fileInfo, err := app.RecordVideo(ctx, cca.TimerOff, recordTime)
	if err != nil {
		return errors.Wrap(err, "failed to record a time-lapse video")
	}
	filePath, err := app.FilePathInSavedDir(ctx, fileInfo.Name())
	if err != nil {
		return errors.Wrap(err, "failed to get file path in saved path")
	}
	return validateTimeLapseDuration(ctx, app, filePath, recordTime)
}

// validateTimeLapseDuration validates if the duration of the result video
// matches with the recorded time and the recorded speed.
func validateTimeLapseDuration(ctx context.Context, app *cca.App, path string, recordTime time.Duration) error {
	duration, err := cca.VideoDuration(ctx, path)
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
