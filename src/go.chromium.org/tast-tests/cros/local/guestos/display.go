// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package guestos

import (
	"bytes"
	"context"
	"image/color"
	"image/png"
	"math"
	"os"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/colorcmp"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/screenshot"
	"go.chromium.org/tast-tests/cros/local/vm"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// PollWindowSize returns the the width and the height of the window in pixels
// with polling to wait for asynchronous rendering on the DUT.
func PollWindowSize(ctx context.Context, tconn *chrome.TestConn, name string, timeout time.Duration) (sz coords.Size, err error) {
	// Allow up to 10 seconds for the target screen to render.
	err = testing.Poll(ctx, func(ctx context.Context) error {
		var err error
		sz, err = windowSize(ctx, tconn, name)
		return err
	}, &testing.PollOptions{Timeout: timeout})
	return sz, err
}

// windowSize returns the the width and the height of the window in pixels.
func windowSize(ctx context.Context, tconn *chrome.TestConn, name string) (sz coords.Size, err error) {
	ui := uiauto.New(tconn)
	appWindow := nodewith.Name(name).First()
	if err := ui.WithTimeout(30 * time.Second).WaitUntilExists(appWindow)(ctx); err != nil {
		return coords.Size{}, errors.Wrap(err, "failed to locate the app window")
	}

	// Apps can open extra "degenerate" windows. We look for the first window with
	// a client view that has a non-empty location node.
	for i := 0; i < 4; i++ {
		view := nodewith.ClassName("ClientView").Nth(i)
		loc, err := ui.WithTimeout(15*time.Second).Location(ctx, view)
		if err == nil {
			if loc.Empty() {
				continue
			}
			return loc.Size(), nil
		}
	}

	return coords.Size{}, errors.Wrap(err, "failed to find client view location node")
}

// MatchScreenshotDominantColor takes a screenshot and attempts to verify if it
// mostly (>= 1/2) contains the expected color (allowing down to 50% brightness
// scaling to handle screen dimming / overlays). Will retry for up to 30 seconds
// if it fails. For logging purposes, the screenshot will be saved at the given
// path.
func MatchScreenshotDominantColor(ctx context.Context, cr *chrome.Chrome, expectedColor color.Color, screenshotPath string) error {
	if !strings.HasSuffix(screenshotPath, ".png") {
		return errors.New("Screenshots must have the '.png' extension, got: " + screenshotPath)
	}
	// Largest differing per-channel component diff after scaling.
	const maxKnownColorDiff = 2
	const minBrightnessScale = 0.50
	const maxBrightnessScale = 1.05

	// Allow up to 30 seconds for the target screen to render.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if err := screenshot.CaptureChrome(ctx, cr, screenshotPath); err != nil {
			return err
		}
		f, err := os.Open(screenshotPath)
		if err != nil {
			return errors.Wrapf(err, "failed opening the screenshot image %v", screenshotPath)
		}
		defer f.Close()
		im, err := png.Decode(f)
		if err != nil {
			return errors.Wrapf(err, "failed decoding the screenshot image %v", screenshotPath)
		}
		color, ratio := colorcmp.DominantColor(im)
		if ratio >= 0.5 && colorsMatchScaled(color, expectedColor, minBrightnessScale, maxBrightnessScale, maxKnownColorDiff) {
			return nil
		}
		return errors.Errorf("screenshot did not have matching dominant color, got %v at ratio %0.2f but expected %v",
			colorcmp.ColorStr(color), ratio, colorcmp.ColorStr(expectedColor))
	}, &testing.PollOptions{Timeout: 30 * time.Second}); err != nil {
		return err
	}
	return nil
}

// colorsMatchScaled checks if actual matches expected scaled by any brightness
// factor within [minScale, maxScale] with per-channel difference <= maxDiff.
func colorsMatchScaled(actual, expected color.Color, minScale, maxScale float64, maxDiff uint8) bool {
	an := color.NRGBAModel.Convert(actual).(color.NRGBA)
	en := color.NRGBAModel.Convert(expected).(color.NRGBA)

	ar, ag, ab := float64(an.R), float64(an.G), float64(an.B)
	er, eg, eb := float64(en.R), float64(en.G), float64(en.B)

	denom := er*er + eg*eg + eb*eb
	if denom == 0 {
		return colorcmp.ColorsMatch(actual, expected, maxDiff)
	}

	scale := (ar*er + ag*eg + ab*eb) / denom
	if scale < minScale || scale > maxScale {
		return false
	}

	near := func(a, e float64) bool {
		return math.Abs(a-scale*e) <= float64(maxDiff)
	}
	// Both expected (from colorcmp.RGB) and actual (from screenshot PNG) are
	// opaque RGB colors with A=255, so we only need to compare R, G, and B.
	return near(ar, er) && near(ag, eg) && near(ab, eb)
}

// RunWindowedApp Runs the command cmdline in the guest, waits for the window
// windowName to open, sends it a key press event, runs condition, and then
// closes all open windows. Note that this will close windows other then the
// one with title windowName! The return value is a string containing the what
// program wrote to stdout. The intended use of condition is to delay closing
// the application window until some event has occurred. If condition returns
// an error then the call will be considered a failure and the error will be
// propagated.
func RunWindowedApp(ctx context.Context, tconn *chrome.TestConn, guest vm.Guest, keyboard *input.KeyboardEventWriter, timeout time.Duration, condition func(context.Context) error, closeWindow bool, windowName string, cmdline []string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	testing.ContextLogf(ctx, "Starting %v application", windowName)
	cmd := guest.Command(ctx, cmdline...)
	var buf bytes.Buffer
	cmd.Stdout = &buf

	if err := cmd.Start(); err != nil {
		return "", errors.Wrapf(err, "failed to start command %v", cmdline)
	}
	defer cmd.Wait(testexec.DumpLogOnError)

	size, err := PollWindowSize(ctx, tconn, windowName, timeout)
	if err != nil {
		return "", errors.Wrapf(err, "failed to find window %q while running %v", windowName, cmdline)
	}
	testing.ContextLogf(ctx, "Window %q is visible with size %v", windowName, size)
	testing.ContextLog(ctx, "Sending keypress to ", windowName)
	if err := keyboard.Type(ctx, " "); err != nil {
		return "", errors.Wrapf(err, "failed to send keypress to window while running %v", cmdline)
	}

	if condition != nil {
		if err := condition(ctx); err != nil {
			return "", errors.Wrapf(err, "failed to check condition closure while running %v", cmdline)
		}
	}

	if closeWindow {
		// TODO(crbug.com/996609) Change this to only close the window that just got opened.
		testing.ContextLog(ctx, "Closing all windows")
		if err := ash.CloseAllWindows(ctx, tconn); err != nil {
			return "", errors.Wrapf(err, "failed to close all windows while running %v", cmdline)
		}
	}

	if err := cmd.Wait(testexec.DumpLogOnError); err != nil {
		return "", errors.Wrapf(err, "command %v failed to terminate properly", cmdline)
	}

	return buf.String(), nil
}
