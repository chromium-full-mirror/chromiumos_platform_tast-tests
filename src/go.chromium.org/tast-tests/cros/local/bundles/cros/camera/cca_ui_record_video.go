// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/camera/cca"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/input"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CCAUIRecordVideo,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Opens CCA and verifies video recording related use cases",
		Contacts:     []string{"chromeos-camera-eng@google.com", "kamchonlathorn@chromium.org"},
		BugComponent: "b:978428", // ChromeOS > Platform > Technologies > Camera > App & Framework
		Attr:         []string{"group:mainline", "informational", "group:camera-libcamera", "group:intel-gating", "group:intel-nda"},
		SoftwareDeps: []string{"camera_app", "chrome"},
		Timeout:      5 * time.Minute,
		Fixture:      "ccaTestBridgeReadyWithFakeHALCamera",
	})
}

// durationTolerance is the tolerate difference when comparing video duration.
// There is a known issue that pause/resume event might be fired before it
// actually happens so the aggregated duration difference is expected to be
// higher if it involves many pause/resume actions.
// See: https://crbug.com/1397837 for more details.
const durationTolerance = 500 * time.Millisecond

type videoState string

const (
	// videoStateEmpty is the state before video start recording.
	videoStateEmpty videoState = "empty"
	// videoStateRecording is the state when video is recording.
	videoStateRecording = "recording"
	// videoStatePaused is the state when video is paused.
	videoStatePaused = "paused"
	// videoStateStopped is the state when video is finished recording.
	videoStateStopped = "stopped"
)

func checkState(actual videoState, expecteds ...videoState) error {
	found := false
	for _, expected := range expecteds {
		if actual == expected {
			found = true
			break
		}
	}
	if !found {
		return errors.Errorf("Assert video in %v state(s); get %v state", expecteds, actual)
	}
	return nil
}

// video tracks the expected video state across multiple CCA video operations.
type video struct {
	// state is the current recording state of video.
	state videoState
	// last is the last timing of toggling |state|.
	last time.Time
	// d aggregates expected duration of all finished video clips.
	d time.Duration
}

func newVideo() *video {
	return &video{state: videoStateEmpty}
}

func (v *video) start(ctx context.Context, app *cca.App) error {
	if err := checkState(v.state, videoStateEmpty); err != nil {
		return err
	}
	t, err := app.TriggerStateChange(ctx, "recording", true, func() error {
		return app.ClickShutter(ctx)
	})
	if err != nil {
		return err
	}
	v.state = videoStateRecording
	v.last = t
	v.d = time.Duration(0)
	return nil
}

func (v *video) pause(ctx context.Context, app *cca.App) error {
	if err := checkState(v.state, videoStateRecording); err != nil {
		return err
	}
	t, err := app.TriggerStateChange(ctx, "recording-paused", true, func() error {
		if err := app.Click(ctx, cca.VideoPauseResumeButton); err != nil {
			return errors.Wrap(err, "failed to pause recording")
		}
		return nil
	})
	if err != nil {
		return err
	}
	v.d += t.Sub(v.last)
	v.state = videoStatePaused
	v.last = t
	return nil
}

func (v *video) resume(ctx context.Context, app *cca.App) error {
	if err := checkState(v.state, videoStatePaused); err != nil {
		return err
	}
	t, err := app.TriggerStateChange(ctx, "recording-paused", false, func() error {
		if err := app.Click(ctx, cca.VideoPauseResumeButton); err != nil {
			return errors.Wrap(err, "failed to resume recording")
		}
		return nil
	})
	if err != nil {
		return err
	}
	v.state = videoStateRecording
	v.last = t
	return nil
}

func (v *video) stop(ctx context.Context, app *cca.App) error {
	if err := checkState(v.state, videoStateRecording, videoStatePaused); err != nil {
		return err
	}
	info, t, err := app.StopRecording(ctx, cca.TimerOff, v.last)
	if err != nil {
		return errors.Wrap(err, "failed to stop recording")
	}
	if v.state == videoStateRecording {
		v.d += t.Sub(v.last)
	}
	v.state = videoStateStopped
	v.last = t

	// Check duration from result video file with expected duration.
	path, err := app.FilePathInSavedDir(ctx, info.Name())
	if err != nil {
		return errors.Wrap(err, "failed to get file path in saved path")
	}
	duration, err := cca.VideoDurationFromHeader(ctx, path)
	if err != nil {
		return err
	}
	if duration > v.d+durationTolerance || duration < v.d-durationTolerance {
		return errors.Errorf("incorrect result video duration get %v; want %v with tolerance %v", duration, v.d, durationTolerance)
	}
	return nil
}

// CCAUIRecordVideo verifies video recording related functionalities works.
func CCAUIRecordVideo(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(cca.FixtureData).Chrome
	s.FixtValue().(cca.FixtureData).SetDebugParams(cca.DebugParams{SaveCameraFolderWhenFail: true})
	runTestWithApp := s.FixtValue().(cca.FixtureData).RunTestWithApp
	subTestTimeout := 40 * time.Second
	for _, tc := range []struct {
		name  string
		run   func(context.Context, *cca.App) error
		timer cca.TimerState
		mic   cca.MicState
	}{
		{"testRecordVideoWithWindowChanged", testRecordVideoWithWindowChanged, cca.TimerOff, cca.MicOn},
		{"testVideoProfile", testVideoProfile, cca.TimerOff, cca.MicOn},
		{"testRecordVideoWithTimer", testRecordVideoWithTimer, cca.TimerOn, cca.MicOn},
		{"testRecordVideoWithMute", testRecordVideoWithMute, cca.TimerOff, cca.MicOff},
		{"testRecordCancelTimer", testRecordCancelTimer, cca.TimerOn, cca.MicOn},
		{"testVideoSnapshot", testVideoSnapshot, cca.TimerOff, cca.MicOn},
		{"testStopInPause", testStopInPause, cca.TimerOff, cca.MicOn},
		{"testPauseResume", testPauseResume, cca.TimerOff, cca.MicOn},
		{"testVideoSeekability", testVideoSeekability, cca.TimerOff, cca.MicOn},
	} {
		subTestCtx, cancel := context.WithTimeout(ctx, subTestTimeout)
		s.Run(subTestCtx, tc.name, func(ctx context.Context, s *testing.State) {
			if err := runTestWithApp(ctx, func(ctx context.Context, app *cca.App) error {
				testing.ContextLog(ctx, "Switch to video mode")
				if err := app.SwitchMode(ctx, cca.Video); err != nil {
					return errors.Wrap(err, "failed to switch to video mode")
				}
				if err := app.WaitForVideoActive(ctx); err != nil {
					return errors.Wrap(err, "preview is inactive after switch to video mode")
				}
				return app.RunThroughCameras(ctx, func(_ cca.Facing) error {
					if err := app.SetTimerOption(ctx, tc.timer); err != nil {
						return errors.Wrapf(err, "failed to set timer option %v", tc.timer)
					}
					if err := app.SetMicOption(ctx, tc.mic); err != nil {
						return errors.Wrap(err, "failed to set microphone option")
					}
					return tc.run(ctx, app)
				})
			}, cca.TestWithAppParams{}); err != nil {
				s.Errorf("Failed to pass %v subtest: %v", tc.name, err)
			}
		})
		cancel()
	}

	subTestCtx, cancel := context.WithTimeout(ctx, subTestTimeout)
	s.Run(subTestCtx, "testConfirmDialog", func(ctx context.Context, s *testing.State) {
		if err := runTestWithApp(ctx, func(ctx context.Context, app *cca.App) error {
			return testConfirmDialog(ctx, app, cr)
		}, cca.TestWithAppParams{StopAppOnlyIfExist: true}); err != nil {
			s.Error("Failed to pass confirm dialog subtest: ", err)
		}
	})
	cancel()
}

func testRecordVideoWithWindowChanged(ctx context.Context, app *cca.App) error {
	dir, err := app.SavedDir(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get CCA default saved path")
	}
	testing.ContextLog(ctx, "Click on start shutter")
	if err := app.ClickShutter(ctx); err != nil {
		return err
	}
	// TODO(b/278996399): Group all record-related sleep into one function.
	// GoBigSleepLint: Record the video for 1 second.
	if err := testing.Sleep(ctx, time.Second); err != nil {
		return err
	}
	testing.ContextLog(ctx, "Maximizing window")
	if err := app.MaximizeWindow(ctx); err != nil {
		return errors.Wrap(err, "failed to maximize window")
	}
	// GoBigSleepLint: Continue recording in the maximized window for 1 second.
	if err := testing.Sleep(ctx, time.Second); err != nil {
		return err
	}
	testing.ContextLog(ctx, "Restore window")
	if err := app.RestoreWindow(ctx); err != nil {
		return errors.Wrap(err, "failed to restore window")
	}
	// GoBigSleepLint: Continue recording in the restored window for 1 second.
	if err := testing.Sleep(ctx, time.Second); err != nil {
		return err
	}
	start := time.Now()
	testing.ContextLog(ctx, "Click on stop shutter")
	if err := app.ClickShutter(ctx); err != nil {
		return err
	}
	if err := app.WaitForState(ctx, "taking", false); err != nil {
		return errors.Wrap(err, "shutter is not ended")
	}
	if _, err := app.WaitForFileSaved(ctx, dir, cca.VideoPattern, start); err != nil {
		return errors.Wrap(err, "cannot find result video")
	}
	return nil
}

func testVideoProfile(ctx context.Context, app *cca.App) error {
	file, err := app.RecordVideo(ctx, cca.TimerOn, time.Second)
	if err != nil {
		return err
	}

	path, err := app.FilePathInSavedDir(ctx, file.Name())
	if err != nil {
		return err
	}

	return cca.CheckVideoProfile(path, cca.ProfileH264High)
}

func testRecordVideoWithTimer(ctx context.Context, app *cca.App) error {
	_, err := app.RecordVideo(ctx, cca.TimerOn, time.Second)
	return err
}

func testRecordVideoWithMute(ctx context.Context, app *cca.App) error {
	info, err := app.RecordVideo(ctx, cca.TimerOff, time.Second)
	if err != nil {
		return errors.Wrap(err, "failed to record muted video")
	}

	path, err := app.FilePathInSavedDir(ctx, info.Name())
	if err != nil {
		return errors.Wrap(err, "failed to get file path in saved path")
	}
	if err := cca.CheckVideoMuted(ctx, path); err != nil {
		return errors.Wrap(err, "failed to check if the recorded video is muted")
	}
	return nil
}

func testRecordCancelTimer(ctx context.Context, app *cca.App) error {
	testing.ContextLog(ctx, "Click on start shutter")
	if err := app.ClickShutter(ctx); err != nil {
		return err
	}
	// GoBigSleepLint: Wait for 1 second before canceling the timer.
	if err := testing.Sleep(ctx, time.Second); err != nil {
		return err
	}
	testing.ContextLog(ctx, "Click on cancel shutter")
	if err := app.ClickShutter(ctx); err != nil {
		return err
	}
	if err := app.WaitForState(ctx, "taking", false); err != nil {
		return err
	}
	return nil
}

func testVideoSnapshot(ctx context.Context, app *cca.App) error {
	testing.ContextLog(ctx, "Click on start shutter")
	startTime := time.Now()
	if err := app.ClickShutter(ctx); err != nil {
		return err
	}
	if err := app.WaitForState(ctx, "recording", true); err != nil {
		return errors.Wrap(err, "recording is not started")
	}

	// GoBigSleepLint: Ensure video have at least 1s duration.
	if err := testing.Sleep(ctx, time.Second); err != nil {
		return errors.Wrap(err, "failed to sleep in video duration")
	}

	// Take a video snapshot.
	if err := app.Click(ctx, cca.VideoSnapshotButton); err != nil {
		return err
	}
	dir, err := app.SavedDir(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get saved directories")
	}
	if _, err := app.WaitForFileSaved(ctx, dir, cca.PhotoPattern, startTime); err != nil {
		return errors.Wrap(err, "failed to find saved video snapshot file")
	}

	if _, _, err := app.StopRecording(ctx, cca.TimerOff, startTime); err != nil {
		return errors.Wrap(err, "failed to stop recording")
	}
	return nil
}

// startRecordAndPause starts recording for 1 second and pauses the recording.
func startRecordAndPause(ctx context.Context, app *cca.App) (*video, error) {
	v := newVideo()
	if err := v.start(ctx, app); err != nil {
		return nil, err
	}
	// GoBigSleepLint: Record the video for 1 second.
	if err := testing.Sleep(ctx, time.Second); err != nil {
		return nil, err
	}
	if err := v.pause(ctx, app); err != nil {
		return nil, err
	}
	return v, nil
}

func testStopInPause(ctx context.Context, app *cca.App) error {
	v, err := startRecordAndPause(ctx, app)
	if err != nil {
		return errors.Wrap(err, "failed to start and pause recording")
	}

	// GoBigSleepLint: Pause the video for 1 second.
	if err := testing.Sleep(ctx, time.Second); err != nil {
		return err
	}

	return v.stop(ctx, app)
}

func testPauseResume(ctx context.Context, app *cca.App) error {
	v, err := startRecordAndPause(ctx, app)
	if err != nil {
		return errors.Wrap(err, "failed to start and pause recording")
	}
	// GoBigSleepLint: Pause the video for 1 second.
	if err := testing.Sleep(ctx, time.Second); err != nil {
		return errors.Wrap(err, "failed to sleep 1 second in pausing state")
	}
	if err := v.resume(ctx, app); err != nil {
		return err
	}
	// GoBigSleepLint: Continue recording the video for 1 second.
	if err := testing.Sleep(ctx, time.Second); err != nil {
		return errors.Wrap(err, "failed to sleep 1 second after resuming")
	}

	return v.stop(ctx, app)
}

// testVideoSeekability tests whether the recorded video can be seeked by
// checking the number of key frames in the recorded video. It is expected that
// the video recorded for |recordTime| will have at least |minKeyFrames|.
func testVideoSeekability(ctx context.Context, app *cca.App) error {
	// TODO(kamchonlathorn): Check if current video resolution is not smaller than 480p.

	// By default, the video should have a key frame for every 100 frames, which
	// is around every 4 seconds for 30 fps video. So, we expect at least 3 key
	// frames for a video recorded for 10 seconds.
	recordTime := 10 * time.Second
	minKeyFrames := 3

	fileInfo, err := app.RecordVideo(ctx, cca.TimerOff, recordTime)
	if err != nil {
		return errors.Wrap(err, "failed to record a video")
	}
	path, err := app.FilePathInSavedDir(ctx, fileInfo.Name())
	if err != nil {
		return errors.Wrap(err, "failed to get file path in saved path")
	}
	numKeyFrames, err := cca.NumberOfKeyFrames(ctx, path)
	if err != nil {
		return err
	}

	if numKeyFrames < minKeyFrames {
		return errors.Errorf("failed to seek a video, the video has %d key frames, expected at least %d", numKeyFrames, minKeyFrames)
	}
	return nil
}

func testConfirmDialog(ctx context.Context, app *cca.App, cr *chrome.Chrome) error {
	if err := app.TriggerConfiguration(ctx, func() error {
		testing.ContextLog(ctx, "Switch to video mode")
		if err := app.SwitchMode(ctx, cca.Video); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return err
	}

	testing.ContextLog(ctx, "Start Recording")
	startTime := time.Now()
	if err := app.ClickShutter(ctx); err != nil {
		return err
	}
	if err := app.WaitForState(ctx, "recording", true); err != nil {
		return errors.Wrap(err, "recording is not started")
	}

	testing.ContextLog(ctx, "Try to close camera app")
	keyboard, err := input.Keyboard(ctx)
	if err != nil {
		return err
	}
	defer keyboard.Close(ctx)

	// Try to close the camera app.
	if err := keyboard.Accel(ctx, "ctrl+W"); err != nil {
		return err
	}

	// It is expected that the camera app is not closed.
	errTimeout := errors.New("CCA exists after timeout")
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		appExist, err := cca.InstanceExists(ctx, cr)
		if err != nil {
			return testing.PollBreak(err)
		}
		if !appExist {
			return testing.PollBreak(errors.New("CCA is unexpectedly closed"))
		}
		return errTimeout
	}, &testing.PollOptions{Timeout: 3 * time.Second}); !errors.Is(err, errTimeout) {
		return err
	}

	// Dismiss the confirm dialog.
	if err := keyboard.Accel(ctx, "esc"); err != nil {
		return err
	}

	testing.ContextLog(ctx, "Stop recording")
	if _, _, err := app.StopRecording(ctx, cca.TimerOff, startTime); err != nil {
		return errors.Wrap(err, "failed to stop recording")
	}

	testing.ContextLog(ctx, "Try to close camera app")
	// Try to close the camera app again.
	if err := keyboard.Accel(ctx, "ctrl+W"); err != nil {
		return err
	}

	// Now the camera app is closable.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		appExist, err := cca.InstanceExists(ctx, cr)
		if err != nil {
			return testing.PollBreak(err)
		}
		if appExist {
			return errors.New("CCA is not closed")
		}
		return nil
	}, &testing.PollOptions{Timeout: 3 * time.Second}); err != nil {
		return err
	}
	return nil
}
