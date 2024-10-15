// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputsimulations

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/input"
)

// ScrollDownFor two-finger swipes on the trackpad to scroll down repeatedly until the
// scrollDuration has passed, with a scrollDelay between each swipe.
func ScrollDownFor(ctx context.Context, tpw *input.TrackpadEventWriter, tw *input.TouchEventWriter, scrollDelay, scrollDuration time.Duration) error {
	fingerHorizontalSpacing := tpw.Width() / 4
	fingerVerticalSpacing := input.TouchCoord(0)
	fingerNum := 2

	var startX, startY, endX, endY input.TouchCoord
	startX, startY, endX, endY = tpw.Width()/2, 1, tpw.Width()/2, tpw.Height()-1

	for endTime := time.Now().Add(scrollDuration); time.Now().Before(endTime); {
		// Double swipe from the middle button to the middle top of the touchpad.
		if err := tw.Swipe(ctx, startX, startY, endX, endY, fingerHorizontalSpacing,
			fingerVerticalSpacing, fingerNum, scrollDelay); err != nil {
			return err
		}
	}
	return tw.End()
}

// RepeatScrollDownFor two-finger swipes on the trackpad to scroll down repeatedly for
// a specified number of times, with a scrollDelay between each swipe.
func RepeatScrollDownFor(ctx context.Context, tpw *input.TrackpadEventWriter, tw *input.TouchEventWriter, scrollDelay time.Duration, scrollTimes int) error {
	return repeatScrollFor(ctx, tpw, tw, scrollDelay, scrollTimes, false /* scrollUp */)
}

// RepeatScrollUpFor two-finger swipes on the trackpad to scroll up repeatedly for
// a specified number of times, with a scrollDelay between each swipe.
func RepeatScrollUpFor(ctx context.Context, tpw *input.TrackpadEventWriter, tw *input.TouchEventWriter, scrollDelay time.Duration, scrollTimes int) error {
	return repeatScrollFor(ctx, tpw, tw, scrollDelay, scrollTimes, true /* scrollUp */)
}

func repeatScrollFor(ctx context.Context, tpw *input.TrackpadEventWriter, tw *input.TouchEventWriter, scrollDelay time.Duration, scrollTimes int, scrollUp bool) error {
	fingerHorizontalSpacing := tpw.Width() / 4
	fingerVerticalSpacing := input.TouchCoord(0)
	xCoord := tpw.Width() / 2
	fingerNum := 2

	var startY, endY input.TouchCoord
	startY, endY = 1, tpw.Height()-1
	if scrollUp {
		startY, endY = tpw.Height()-1, 1
	}

	for i := 0; i < scrollTimes; i++ {
		// Double swipe from the middle top to the middle bottom of the touchpad.
		if err := tw.Swipe(ctx, xCoord, startY, xCoord, endY, fingerHorizontalSpacing,
			fingerVerticalSpacing, fingerNum, scrollDelay); err != nil {
			return err
		}
	}
	return tw.End()
}
