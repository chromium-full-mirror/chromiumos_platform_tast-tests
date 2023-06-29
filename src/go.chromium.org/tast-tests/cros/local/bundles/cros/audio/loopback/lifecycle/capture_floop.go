// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package lifecycle

import (
	"context"
	"fmt"

	"go.chromium.org/tast-tests/cros/local/audio/crastestclient"
	"go.chromium.org/tast/core/testing"
)

type requestFlexibleLoopbackAction struct {
	requestSec int
	floopMask  int
}

var _ Action = requestFlexibleLoopbackAction{}

func (a requestFlexibleLoopbackAction) Do(ctx context.Context, s *testing.State, t *tester) {
	t.logAction(ctx, a.requestSec, "request flexible loopback", true)
	floopDev, err := crastestclient.RequestFloopMask(ctx, a.floopMask)
	if err != nil {
		s.Fatal("Request floop failed: ", floopDev)
	}
}

func (a requestFlexibleLoopbackAction) maybeLogSchedule(ctx context.Context, t *tester) {
	t.logScheduleRow(ctx, "floop active", 'f', schedule{a.requestSec, t.testDurationSec})
}

func (a requestFlexibleLoopbackAction) getSchedule() schedule {
	return schedule{a.requestSec, a.requestSec}
}

func (a requestFlexibleLoopbackAction) durationSec() int {
	return 0
}

// RequestFloopOnly returns a Action which requests the flexible loopback at requestSec
func RequestFloopOnly(requestSec int) Action {
	return &requestFlexibleLoopbackAction{
		requestSec: requestSec,
		floopMask:  4,
	}
}

type captureFlexibleLoopbackAction struct {
	isCaptureAction
	schedule
	requestSec int
	floopMask  int
}

var _ CaptureAction = captureFlexibleLoopbackAction{}

func (a captureFlexibleLoopbackAction) Do(ctx context.Context, s *testing.State, t *tester) {
	t.logAction(ctx, a.requestSec, "request flexible loopback", true)
	floopDev, err := crastestclient.RequestFloopMask(ctx, a.floopMask)
	if err != nil {
		s.Fatal("Request floop failed: ", floopDev)
	}

	loopbackArgs := []string{
		fmt.Sprintf("--capture_file=%s", t.captureRaw),
		fmt.Sprintf("--pin_device=%d", floopDev),
	}

	runCapture(ctx, s, t, a.startSec, a.endSec, loopbackArgs)
}

func (a captureFlexibleLoopbackAction) maybeLogSchedule(ctx context.Context, t *tester) {
	t.logScheduleRow(ctx, "floop active", 'f', schedule{a.requestSec, t.testDurationSec})
	t.logScheduleRow(ctx, "capture", 'c', a.schedule)
}

// CaptureFloop returns a CaptureAction which:
// - requests the flexible loopback at requestSec
// - captures from the flexible loopback between startSec and endSec
func CaptureFloop(requestSec, startSec, endSec int) CaptureAction {
	return &captureFlexibleLoopbackAction{
		schedule:   schedule{startSec, endSec},
		requestSec: requestSec,
		floopMask:  4,
	}
}

// CaptureMismatchedFloop is like FloopCapture but the requested flexible loopback device
// is configured to not match the Playback CRAS client, so no audio should be captured
func CaptureMismatchedFloop(requestSec, startSec, endSec int) CaptureAction {
	return &captureFlexibleLoopbackAction{
		schedule:   schedule{startSec, endSec},
		requestSec: requestSec,
		floopMask:  0,
	}
}
