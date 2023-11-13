// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/audio/crastestclient"
	"go.chromium.org/tast-tests/cros/local/camera/cca"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CCAUISound,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Ensure the sound playing functionality works in CCA",
		Contacts: []string{
			"chromeos-camera-eng@google.com",
			"wtlee@chromium.org",
		},
		BugComponent: "b:978428", // ChromeOS > Platform > Technologies > Camera > App & Framework
		Attr:         []string{"group:mainline", "informational", "group:camera-libcamera"},
		SoftwareDeps: []string{"camera_app", "chrome"},
		Fixture:      "ccaLaunchedAudioLoopback",
	})
}

func CCAUISound(ctx context.Context, s *testing.State) {
	app := s.FixtValue().(cca.FixtureData).App()

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	s.AttachErrorHandlers(
		func(_ string) {
			crastestclient.DumpAudioDiagnostics(cleanupCtx, s.OutDir())
		},
		nil)

	testing.ContextLog(ctx, "Entering photo mode")
	if err := app.SwitchMode(ctx, cca.Photo); err != nil {
		s.Fatal("Failed to switch to photo mode: ", err)
	}
	testing.ContextLog(ctx, "Verifying if shutter sound can be played")
	if err := clickShutterAndVerifySound(ctx, app); err != nil {
		s.Error("Failed to verify the shutter sound is played after taking a photo: ", err)
	}

	testing.ContextLog(ctx, "Turning on the 10s timer")
	if err := app.Click(ctx, cca.OpenTimerPanelButton); err != nil {
		s.Fatal("Failed to open timer option panel: ", err)
	}
	if err := app.Click(ctx, cca.TimerOption10Seconds); err != nil {
		s.Fatal("Failed to click the 10s-timer timer button: ", err)
	}
	if err := app.WaitForState(ctx, "timer-10s", true); err != nil {
		s.Fatal("Failed to wait for 10s-timer being active: ", err)
	}

	testing.ContextLog(ctx, "Verifying the timer ticking sound can be played")
	if err := clickShutterAndVerifySound(ctx, app); err != nil {
		s.Error("Failed to verify the timer ticking sound is played after starting the timer: ", err)
	}

	testing.ContextLog(ctx, "Cancelling the capture")
	if err := app.ClickShutter(ctx); err != nil {
		s.Fatal("Failed to cancel capture: ", err)
	}

	testing.ContextLog(ctx, "Turning off the timer")
	if err := app.SetTimerOption(ctx, cca.TimerOff); err != nil {
		s.Fatal("Failed to turn off the timer: ", err)
	}

	testing.ContextLog(ctx, "Entering video mode")
	if err := app.SwitchMode(ctx, cca.Video); err != nil {
		s.Fatal("Failed to switch to video mode: ", err)
	}

	testing.ContextLog(ctx, "Verifying the recorder starting sound can be played")
	if err := clickShutterAndVerifySound(ctx, app); err != nil {
		s.Error("Failed to verify the recorder starting sound is played after clicking the shutter button: ", err)
	}
	if err := app.WaitForState(ctx, "recording", true); err != nil {
		s.Fatal("Recording is not started: ", err)
	}

	testing.ContextLog(ctx, "Verifying the recorder pausing sound can be played")
	if played, err := cca.VerifySound(ctx, func(ctx context.Context) error {
		if _, err := app.TriggerStateChange(ctx, "recording-paused", true, func() error {
			if err := app.Click(ctx, cca.VideoPauseResumeButton); err != nil {
				return errors.Wrap(err, "failed to pause recording")
			}
			return nil
		}); err != nil {
			return err
		}
		return nil
	}); err != nil {
		s.Error("Failed to verify sound when pausing recording: ", err)
	} else if !played {
		s.Fatal("Failed to verify the recorder pausing sound is played after clicking the pause button")
	}

	testing.ContextLog(ctx, "Verifying the recorder stopping sound can be played")
	if err := clickShutterAndVerifySound(ctx, app); err != nil {
		s.Error("Failed to verify the recorder stopping sound is played after clicking the shutter button: ", err)
	}
}

// clickShutterAndVerifySound clicks the shutter button and verifies if the
// sound played successfully.
// Either shutter/recorder/timer sound should be played.
func clickShutterAndVerifySound(ctx context.Context, app *cca.App) error {
	if played, err := cca.VerifySound(ctx, func(ctx context.Context) error {
		if err := app.ClickShutter(ctx); err != nil {
			return errors.Wrap(err, "failed to click shutter button")
		}
		return nil
	}); err != nil {
		return err
	} else if !played {
		return errors.New("no sound is played after clicking the shutter button")
	}
	return nil
}
