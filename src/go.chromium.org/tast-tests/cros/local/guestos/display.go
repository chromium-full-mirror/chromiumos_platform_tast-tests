// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package guestos

import (
	"context"
	"image/color"
	"image/png"
	"os"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/colorcmp"
	"go.chromium.org/tast-tests/cros/local/screenshot"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// MatchScreenshotDominantColor takes a screenshot and attempts to verify if it
// mostly (>= 1/2) contains the expected color. Will retry for up to 10 seconds
// if it fails. For logging purposes, the screenshot will be saved at the given
// path.
func MatchScreenshotDominantColor(ctx context.Context, cr *chrome.Chrome, expectedColor color.Color, screenshotPath string) error {
	if !strings.HasSuffix(screenshotPath, ".png") {
		return errors.New("Screenshots must have the '.png' extension, got: " + screenshotPath)
	}
	// Largest differing color known to date, we will be changing this over time
	// based on testing results.
	const maxKnownColorDiff = 0x1

	// Allow up to 10 seconds for the target screen to render.
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
		if ratio >= 0.5 && colorcmp.ColorsMatch(color, expectedColor, maxKnownColorDiff) {
			return nil
		}
		return errors.Errorf("screenshot did not have matching dominant color, got %v at ratio %0.2f but expected %v",
			colorcmp.ColorStr(color), ratio, colorcmp.ColorStr(expectedColor))
	}, &testing.PollOptions{Timeout: 30 * time.Second}); err != nil {
		return err
	}
	return nil
}
