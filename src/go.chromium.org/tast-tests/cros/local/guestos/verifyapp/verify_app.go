// Copyright 2018 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package verifyapp

import (
	"context"
	"fmt"
	"image/color"
	"path/filepath"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/guestos"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/vm"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// RunTest executes a test application directly from the command
// line in the terminal and verifies that it renders the majority of pixels on
// the screen in the specified color.
func RunTest(ctx context.Context, outDir string, cr *chrome.Chrome, guest vm.Guest, conf guestos.DemoConfig) error {
	keyboard, err := input.Keyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to find keyboard device")
	}
	defer keyboard.Close(ctx)

	// Launch the test app which will maximize itself and then use the
	// argument as a solid color to fill as its background.
	nrgba := color.NRGBAModel.Convert(conf.DominantColor).(color.NRGBA)
	commandColor := fmt.Sprintf("--bgcolor=0x%02x%02x%02x", nrgba.R, nrgba.G, nrgba.B)
	commandTitle := fmt.Sprintf("--title=%s_terminal", conf.Name)
	cmd := guest.Command(ctx, conf.AppPath, commandColor, commandTitle)
	if err := cmd.Start(); err != nil {
		return errors.Wrapf(err, "failed launching %v", conf.AppPath)
	}
	defer cmd.Wait(testexec.DumpLogOnError)
	defer cmd.Kill()

	if err := guestos.MatchScreenshotDominantColor(ctx, cr, conf.DominantColor, filepath.Join(outDir, conf.Name+"_screenshot.png")); err != nil {
		return errors.Wrapf(err, "failed to see screenshot %q", conf.Name)
	}

	// Terminate the app now so that if there's a failure in the
	// screenshot then we can get its output which may give us useful information
	// about display errors.
	testing.ContextLogf(ctx, "Closing %v with keypress", conf.Name)
	if err := keyboard.Accel(ctx, "Enter"); err != nil {
		testing.ContextLog(ctx, "Failed to type Enter key: ", err)
	}

	return nil
}
