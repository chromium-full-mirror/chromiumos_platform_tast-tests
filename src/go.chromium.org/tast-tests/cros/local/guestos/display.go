// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package guestos

import (
	"bytes"
	"context"
	_ "embed" // Embed x11_focus_wait.py.
	"fmt"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
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
func windowSize(ctx context.Context, tconn *chrome.TestConn, name string) (coords.Size, error) {
	ui := uiauto.New(tconn)
	appWindow := nodewith.Name(name).First()
	if err := ui.WithTimeout(30 * time.Second).WaitUntilExists(appWindow)(ctx); err != nil {
		return coords.Size{}, errors.Wrap(err, "failed to locate the app window")
	}

	// Apps can open extra "degenerate" windows (such as 1x1 helper/leader windows
	// or splash windows). Check up to 4 candidate Widget nodes with the target
	// name (one Widget per window in Ash's automation tree) so an unrelated
	// window's ClientView is not matched, and wait for Ash focus on the client
	// surface before callers send keyboard input.
	const maxCandidateWidgets = 4
	var lastErr error
	for i := 0; i < maxCandidateWidgets; i++ {
		widget := nodewith.Name(name).ClassName("Widget").Nth(i)
		// Most guest apps only open a single window (Nth(0)). Check immediately
		// whether an (i+1)-th Widget exists so we do not burn 15s per iteration
		// polling non-existent windows and clobbering Nth(0)'s failure reason.
		if i > 0 {
			if found, err := ui.IsNodeFound(ctx, widget); err != nil || !found {
				break
			}
		}
		view := nodewith.ClassName("ClientView").Ancestor(widget).First()
		loc, err := ui.WithTimeout(15*time.Second).Location(ctx, view)
		if err != nil {
			lastErr = err
			continue
		}
		if loc.Empty() {
			lastErr = errors.New("client view location is empty")
			continue
		}
		focusedView := nodewith.Focused().Ancestor(view).First()
		if err := ui.WithTimeout(15 * time.Second).WaitUntilExists(focusedView)(ctx); err != nil {
			lastErr = errors.Wrap(err, "failed waiting for client view focus")
			continue
		}
		return loc.Size(), nil
	}

	return coords.Size{}, errors.Wrap(lastErr, "failed to find client view location node")
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

// saveStderrToFile writes stderr output to a file in the test output directory
// and returns the filename if successful, or an empty string otherwise.
func saveStderrToFile(ctx context.Context, windowName, stderrStr string) string {
	outDir, ok := testing.ContextOutDir(ctx)
	if !ok || outDir == "" {
		testing.ContextLog(ctx, "Failed to get output directory to save stderr")
		return ""
	}
	fileName := fmt.Sprintf("%s_stderr.txt", strings.ReplaceAll(windowName, "/", "_"))
	filePath := filepath.Join(outDir, fileName)
	if err := os.WriteFile(filePath, []byte(stderrStr), 0644); err != nil {
		testing.ContextLogf(ctx, "Failed to write stderr to %s: %v", filePath, err)
		return ""
	}
	testing.ContextLogf(ctx, "Saved %s stderr to %s", windowName, fileName)
	return fileName
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
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	if err := cmd.Start(); err != nil {
		return "", errors.Wrapf(err, "failed to start command %v", cmdline)
	}
	defer cmd.Wait(testexec.DumpLogOnError)

	size, err := PollWindowSize(ctx, tconn, windowName, timeout)
	if err != nil {
		cancel()
		if waitErr := cmd.Wait(testexec.DumpLogOnError); waitErr != nil {
			testing.ContextLogf(ctx, "Command %v exited after window poll failure: %v", cmdline, waitErr)
		}
		if stderrStr := strings.TrimSpace(stderrBuf.String()); stderrStr != "" {
			if fileName := saveStderrToFile(ctx, windowName, stderrStr); fileName != "" {
				return "", errors.Wrapf(err, "failed to find window %q while running %v (stderr saved to %s)", windowName, cmdline, fileName)
			}
		}
		return "", errors.Wrapf(err, "failed to find window %q while running %v", windowName, cmdline)
	}
	testing.ContextLogf(ctx, "Window %q is visible with size %v", windowName, size)
	// Under X11/Sommelier, Ash marks the ExoShellSurface focused before Sommelier
	// processes xdg_toplevel.configure(ACTIVATED) / wl_keyboard.enter and sends
	// xcb_set_input_focus to Xwayland; wait until Xwayland's X11 input focus is on
	// windowName so the single keypress is not dropped.
	if isX11Command(cmdline) {
		if err := waitForX11WindowFocus(ctx, guest, windowName); err != nil {
			cancel()
			if waitErr := cmd.Wait(testexec.DumpLogOnError); waitErr != nil {
				testing.ContextLogf(ctx, "Command %v exited after X11 focus failure: %v", cmdline, waitErr)
			}
			if stderrStr := strings.TrimSpace(stderrBuf.String()); stderrStr != "" {
				if fileName := saveStderrToFile(ctx, windowName, stderrStr); fileName != "" {
					return "", errors.Wrapf(err, "failed waiting for X11 focus on %q while running %v (stderr saved to %s)", windowName, cmdline, fileName)
				}
			}
			return "", errors.Wrapf(err, "failed waiting for X11 focus on %q while running %v", windowName, cmdline)
		}
	}
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
		if stderrStr := strings.TrimSpace(stderrBuf.String()); stderrStr != "" {
			if fileName := saveStderrToFile(ctx, windowName, stderrStr); fileName != "" {
				return "", errors.Wrapf(err, "command %v failed to terminate properly (stderr saved to %s)", cmdline, fileName)
			}
		}
		return "", errors.Wrapf(err, "command %v failed to terminate properly", cmdline)
	}

	return stdoutBuf.String(), nil
}

const x11FocusTimeout = 10 * time.Second

// x11FocusWaitScript polls libX11 inside the guest until a viewable X11 window
// with WM_NAME matching sys.argv[1] holds X11 input focus.
//
//go:embed x11_focus_wait.py
var x11FocusWaitScript string

// isX11Command reports whether cmdline launches an X11 guest application.
func isX11Command(cmdline []string) bool {
	for _, arg := range cmdline {
		if arg == "GDK_BACKEND=x11" || filepath.Base(arg) == "x11_demo" {
			return true
		}
	}
	return false
}

// waitForX11WindowFocus waits until windowName is viewable and holds X11 input
// focus inside guest.
func waitForX11WindowFocus(ctx context.Context, guest vm.Guest, windowName string) error {
	// Reserve a small grace buffer before the parent context deadline so the
	// guest Python script can exit cleanly and flush stderr diagnostics before
	// vsh is terminated.
	const graceBuffer = 2 * time.Second
	waitTimeout := x11FocusTimeout
	if deadline, ok := ctx.Deadline(); ok {
		if remaining := time.Until(deadline); remaining > 0 {
			if remaining > graceBuffer && remaining-graceBuffer < waitTimeout {
				waitTimeout = remaining - graceBuffer
			} else if remaining <= graceBuffer {
				waitTimeout = remaining / 2
			}
		}
	}
	focusCtx, cancel := context.WithTimeout(ctx, waitTimeout+graceBuffer)
	defer cancel()

	// Pass x11FocusWaitScript via stdin rather than `python3 -c` because vsh
	// serializes argv into a single SetupConnectionRequest protobuf capped at
	// kMaxMessageSize (4096 bytes), whereas stdin is streamed in DataMessage
	// chunks.
	timeoutArg := fmt.Sprintf("%.2f", waitTimeout.Seconds())
	cmd := guest.Command(focusCtx, "python3", "-", windowName, timeoutArg)
	cmd.Stdin = strings.NewReader(x11FocusWaitScript)
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf
	if err := cmd.Run(testexec.DumpLogOnError); err != nil {
		if msg := strings.TrimSpace(stderrBuf.String()); msg != "" {
			return errors.Wrapf(err, "X11 focus check failed (%s)", msg)
		}
		return err
	}
	return nil
}
