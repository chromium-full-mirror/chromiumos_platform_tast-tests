// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/camera/cca"
	"go.chromium.org/tast-tests/cros/local/camera/testutil"
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

// CCAUIExternal checks that CCA behaves as expected when external camera is connected or disconnected.
func CCAUIExternal(ctx context.Context, s *testing.State) {
	app := s.FixtValue().(cca.FixtureData).App()
	s.FixtValue().(cca.FixtureData).SetDebugParams(cca.DebugParams{SaveCameraFolderWhenFail: true})

	if err := app.SwitchMode(ctx, cca.Photo); err != nil {
		s.Fatal("Failed to switch to photo mode: ", err)
	}
	if _, err := app.TakeSinglePhoto(ctx, cca.TimerOff); err != nil {
		s.Fatal("Failed to take photo: ", err)
	}

	// Adds a second fake camera.
	if err := testutil.WriteFakeHALConfig(ctx, testutil.FakeHALConfig{
		Cameras: []testutil.FakeCameraConfig{
			{ID: 1, Connected: true},
			{ID: 2, Connected: true},
		},
	}); err != nil {
		s.Fatal("Failed to write fake HAL config: ", err)
	}

	if err := waitForCameraSwitchState(ctx, app, true); err != nil {
		s.Fatal("Failed to wait for camera switch appear: ", err)
	}
	if _, err := app.TakeSinglePhoto(ctx, cca.TimerOff); err != nil {
		s.Fatal("Failed to take photo: ", err)
	}

	// Switches to the second fake camera.
	if err := app.SwitchCamera(ctx); err != nil {
		s.Fatal("Failed to switch camera: ", err)
	}
	if _, err := app.TakeSinglePhoto(ctx, cca.TimerOff); err != nil {
		s.Fatal("Failed to take photo: ", err)
	}

	// Disconnect the first fake camera.
	if err := testutil.WriteFakeHALConfig(ctx, testutil.FakeHALConfig{
		Cameras: []testutil.FakeCameraConfig{
			{ID: 1, Connected: false},
			{ID: 2, Connected: true},
		},
	}); err != nil {
		s.Fatal("Failed to write fake HAL config: ", err)
	}
	if err := waitForCameraSwitchState(ctx, app, false); err != nil {
		s.Fatal("Failed to wait for camera switch disappear: ", err)
	}
	if _, err := app.TakeSinglePhoto(ctx, cca.TimerOff); err != nil {
		s.Fatal("Failed to take photo: ", err)
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
		s.Fatal("Failed to write fake HAL config: ", err)
	}
	if err := waitForCameraSwitchState(ctx, app, false); err != nil {
		s.Fatal("Failed to wait for camera switch appear: ", err)
	}
	if _, err := app.TakeSinglePhoto(ctx, cca.TimerOff); err != nil {
		s.Fatal("Failed to take photo: ", err)
	}
}
