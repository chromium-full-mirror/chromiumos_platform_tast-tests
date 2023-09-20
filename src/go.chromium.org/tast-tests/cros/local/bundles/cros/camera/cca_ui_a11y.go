// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/camera/cca"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CCAUIA11y,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks Chromevox reads Chrome Camera App elements as expected",
		Contacts:     []string{"chromeos-camera-eng@google.com", "dorahkim@chromium.org"},
		BugComponent: "b:978428", // ChromeOS > Platform > Technologies > Camera > App & Framework
		Attr:         []string{"group:mainline", "informational", "group:camera-libcamera"},
		SoftwareDeps: []string{"camera_app", "chrome"},
		Fixture:      "ccaLaunchedWithFakeHALCamera",
	})
}

// CCAUIA11y checks that take photo button can be navigated to by tab, and
// pressing space takes a photo.
func CCAUIA11y(ctx context.Context, s *testing.State) {
	app := s.FixtValue().(cca.FixtureData).App()

	// Shorten deadline to leave time for cleanup.
	ctx, cancel := ctxutil.Shorten(ctx, 3*time.Second)
	defer cancel()

	ew, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to create EventWriter from keyboard")
	}
	defer ew.Close(ctx)

	tab := "Tab"
	tookPhoto := false

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		visited, err := app.CheckFocusedElementVisited(ctx)
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to get a focused node"))
		}
		if visited {
			return testing.PollBreak(errors.New("traversed all elements, no element with 'Take photo' aria-label found"))
		}

		ariaLabel, err := app.ReturnFocusedElementAriaLabel(ctx)
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to get an ariaLabel of the current focused node"))
		}

		if ariaLabel == "Take photo" {
			if err := takePictureByKeyboard(ctx, ew, app); err != nil {
				return testing.PollBreak(errors.Wrap(err, "failed to take a picture"))
			}
			tookPhoto = true
		}

		if err = ew.Accel(ctx, tab); err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to press tab key"))
		}
		return nil
	}, &testing.PollOptions{Interval: 1 * time.Second, Timeout: time.Minute}); err != nil {
		s.Error("Failed to check all the elements: ", err)
	}

	if !tookPhoto {
		s.Fatal("The loop finished without taking a photo")
	}
}

func takePictureByKeyboard(ctx context.Context, ew *input.KeyboardEventWriter, app *cca.App) error {
	dir, err := app.SavedDir(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get result saved directory")
	}

	start := time.Now()

	space := "Space"
	if err = ew.Accel(ctx, space); err != nil {
		return errors.Wrap(err, "failed to press space key")
	}

	if err := app.WaitForState(ctx, "taking", false); err != nil {
		return errors.Wrap(err, "shutter is not ended")
	}

	if _, err := app.WaitForFileSaved(ctx, dir, cca.PhotoPattern, start); err != nil {
		return errors.Wrap(err, "cannot find captured result file")
	}

	return nil
}
