// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/camera/cca"
	"go.chromium.org/tast-tests/cros/local/camera/testutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CCAUIExternal,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test external camera connect / disconnect for Chrome Camera App",
		Contacts:     []string{"chromeos-camera-eng@google.com", "pihsun@chromium.org"},
		BugComponent: "b:978428", // ChromeOS > Platform > Technologies > Camera > App & Framework
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"camera_app", "chrome"},
		Fixture:      "ccaLaunchedWithFakeHALCamera",
	})
}

func waitForCameraSwitchState(ctx context.Context, a *cca.App, hasSwitch bool) error {
	return a.WaitForVisibleState(ctx, cca.SwitchDeviceButton, hasSwitch)
}

func takePhotoWithExternalCamera(ctx context.Context, app *cca.App) error {
	if err := testutil.WriteFakeHALConfig(ctx, testutil.FakeHALConfig{
		Cameras: []testutil.FakeCameraConfig{
			{ID: 1, Connected: true},
		},
	}); err != nil {
		return errors.Wrap(err, "failed to write fake HAL config")
	}
	if err := waitForCameraSwitchState(ctx, app, false); err != nil {
		return errors.Wrap(err, "failed to wait for camera switch disappear")
	}
	if err := app.SwitchMode(ctx, cca.Photo); err != nil {
		return errors.Wrap(err, "failed to switch to photo mode")
	}
	if _, err := app.TakeSinglePhoto(ctx, cca.TimerOff); err != nil {
		return errors.Wrap(err, "failed to take photo")
	}

	// Adds a second fake camera.
	if err := testutil.WriteFakeHALConfig(ctx, testutil.FakeHALConfig{
		Cameras: []testutil.FakeCameraConfig{
			{ID: 1, Connected: true},
			{ID: 2, Connected: true},
		},
	}); err != nil {
		return errors.Wrap(err, "failed to write fake HAL config")
	}
	if err := waitForCameraSwitchState(ctx, app, true); err != nil {
		return errors.Wrap(err, "failed to wait for camera switch appear")
	}
	if _, err := app.TakeSinglePhoto(ctx, cca.TimerOff); err != nil {
		return errors.Wrap(err, "failed to take photo")
	}

	// Switches to the second fake camera.
	if err := app.SwitchCamera(ctx); err != nil {
		return errors.Wrap(err, "failed to switch camera")
	}
	if _, err := app.TakeSinglePhoto(ctx, cca.TimerOff); err != nil {
		return errors.Wrap(err, "failed to take photo")
	}

	// Disconnect the first fake camera.
	if err := testutil.WriteFakeHALConfig(ctx, testutil.FakeHALConfig{
		Cameras: []testutil.FakeCameraConfig{
			{ID: 1, Connected: false},
			{ID: 2, Connected: true},
		},
	}); err != nil {
		return errors.Wrap(err, "failed to write fake HAL config")
	}
	if err := waitForCameraSwitchState(ctx, app, false); err != nil {
		return errors.Wrap(err, "failed to wait for camera switch disappear")
	}
	if _, err := app.TakeSinglePhoto(ctx, cca.TimerOff); err != nil {
		return errors.Wrap(err, "failed to take photo")
	}

	// Connects the first fake camera and disconnect the second, this should
	// trigger a reconfiguration since the active camera is disconnected.
	if err := app.TriggerConfiguration(ctx, func() error {
		return testutil.WriteFakeHALConfig(ctx, testutil.FakeHALConfig{
			Cameras: []testutil.FakeCameraConfig{
				{ID: 1, Connected: true},
				{ID: 2, Connected: false},
			},
		})
	}); err != nil {
		return errors.Wrap(err, "failed to write fake HAL config")
	}
	if err := waitForCameraSwitchState(ctx, app, false); err != nil {
		return errors.Wrap(err, "failed to wait for camera switch disappear")
	}
	if _, err := app.TakeSinglePhoto(ctx, cca.TimerOff); err != nil {
		return errors.Wrap(err, "failed to take photo")
	}
	return nil
}

func recordVideoWithExternalCamera(ctx context.Context, app *cca.App) error {
	// Reset FakeHAL config.
	if err := testutil.WriteFakeHALConfig(ctx, testutil.FakeHALConfig{
		Cameras: []testutil.FakeCameraConfig{
		},
	}); err != nil {
		return errors.Wrap(err, "failed to write fake HAL config")
	}

	// Adds one fake camera.
	if err := testutil.WriteFakeHALConfig(ctx, testutil.FakeHALConfig{
		Cameras: []testutil.FakeCameraConfig{
			{ID: 1, Connected: true},
		},
	}); err != nil {
		return errors.Wrap(err, "failed to write fake HAL config")
	}
	if err := app.SwitchMode(ctx, cca.Video); err != nil {
		return errors.Wrap(err, "failed to switch to video mode")
	}
	if _, err := app.RecordVideo(ctx, cca.TimerOff, time.Second); err != nil {
		return errors.Wrap(err, "failed to record video")
	}

	// Adds a second fake camera.
	if err := testutil.WriteFakeHALConfig(ctx, testutil.FakeHALConfig{
		Cameras: []testutil.FakeCameraConfig{
			{ID: 1, Connected: true},
			{ID: 2, Connected: true},
		},
	}); err != nil {
		return errors.Wrap(err, "failed to write fake HAL config")
	}
	if err := waitForCameraSwitchState(ctx, app, true); err != nil {
		return errors.Wrap(err, "failed to wait for camera switch appear")
	}
	startTime, err := app.StartRecording(ctx, cca.TimerOff)
	if err != nil {
		return errors.Wrap(err, "failed to start recording")
	}
	// GoBigSleepLint: Record the video for 1 second.
	if err := testing.Sleep(ctx, time.Second); err != nil {
		return errors.Wrap(err, "failed to sleep for one second")
	}
	// Remove the fake camera while it is recording. This should trigger a
	// reconfiguration since the active camera is disconnected.
	if err := app.TriggerConfiguration(ctx, func() error {
		return testutil.WriteFakeHALConfig(ctx, testutil.FakeHALConfig{
			Cameras: []testutil.FakeCameraConfig{
				{ID: 1, Connected: false},
				{ID: 2, Connected: true},
			},
		})
	}); err != nil {
		return errors.Wrap(err, "failed to write fake HAL config")
	}
	if err := app.WaitForState(ctx, "taking", false); err != nil {
		return errors.Wrap(err, "capturing is not ended")
	}
	if err := waitForCameraSwitchState(ctx, app, false); err != nil {
		return errors.Wrap(err, "failed to wait for camera switch disappear")
	}
	dir, err := app.SavedDir(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get saved dir path")
	}
	if _, err = app.WaitForFileSaved(ctx, dir, cca.VideoPattern, startTime); err != nil {
		return errors.Wrap(err, "cannot find recording video")
	}

	// Plug in a new fake camera while recording.
	startTime, err = app.StartRecording(ctx, cca.TimerOff)
	if err != nil {
		return errors.Wrap(err, "failed to start recording")
	}
	// GoBigSleepLint: Record the video for 1 second.
	if err := testing.Sleep(ctx, time.Second); err != nil {
		return errors.Wrap(err, "failed to sleep for one second")
	}
	// Adds a fake camera during recording.
	if err := testutil.WriteFakeHALConfig(ctx, testutil.FakeHALConfig{
		Cameras: []testutil.FakeCameraConfig{
			{ID: 1, Connected: true},
			{ID: 2, Connected: true},
		},
	}); err != nil {
		return errors.Wrap(err, "failed to write fake HAL config")
	}
	if _, _, err := app.StopRecording(ctx, cca.TimerOff, startTime); err != nil {
		return errors.Wrap(err, "failed to stop recording")
	}
	if err := waitForCameraSwitchState(ctx, app, true); err != nil {
		return errors.Wrap(err, "failed to wait for camera switch appear")
	}
	return nil
}

// CCAUIExternal checks that CCA behaves as expected when external camera is connected or disconnected.
func CCAUIExternal(ctx context.Context, s *testing.State) {
	s.FixtValue().(cca.FixtureData).SetDebugParams(cca.DebugParams{SaveCameraFolderWhenFail: true})
	app := s.FixtValue().(cca.FixtureData).App()

	if err := takePhotoWithExternalCamera(ctx, app); err != nil {
		s.Fatal("Failed to take photo with external camera: ", err)
	}
	if err := recordVideoWithExternalCamera(ctx, app); err != nil {
		s.Fatal("Failed to record video with external camera: ", err)
	}
}
