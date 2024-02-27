// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package lifecycle

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/testing"
)

func runCapture(ctx context.Context, s *testing.State, t *tester, startSec, endSec int, loopbackArgs []string) {
	t.logAction(ctx, startSec, "start capture", true)
	captureCtx, cancel := context.WithDeadline(
		ctx,
		t.t0.Add(time.Duration(endSec)*time.Second+extraTimeout),
	)
	defer cancel()

	cmd := testexec.CommandContext(
		captureCtx,
		"cras_test_client",
		fmt.Sprintf("--duration_seconds=%d", endSec-startSec),
	)
	cmd.Args = append(cmd.Args, loopbackArgs...)
	err := cmd.Run()
	t.logAction(ctx, endSec, "end capture", false)
	if err != nil {
		s.Fatal("Capture failed: ", err)
	}
}

type captureAction struct {
	isCaptureAction
	schedule
}

var _ CaptureAction = captureAction{}

func (a captureAction) Do(ctx context.Context, s *testing.State, t *tester) {
	t.logAction(ctx, a.startSec, "start capture", true)
	captureCtx, cancel := context.WithDeadline(
		ctx,
		t.t0.Add(time.Duration(a.endSec)*time.Second+extraTimeout),
	)
	defer cancel()

	cmd := testexec.CommandContext(
		captureCtx,
		"cras_tests",
		"capture",
		fmt.Sprintf("--duration=%d", a.endSec-a.startSec),
		t.captureRaw,
	)
	err := cmd.Run()
	t.logAction(ctx, a.endSec, "end capture", false)
	if err != nil {
		s.Fatal("Capture failed: ", err)
	}
}

func (a captureAction) maybeLogSchedule(ctx context.Context, t *tester) {
	t.logScheduleRow(ctx, "capture", 'c', a.schedule)
}

// Capture returns a CaptureAction which capture audio from system selected
// node in the given interval.
func Capture(startSec, endSec int) CaptureAction {
	return &captureAction{
		schedule: schedule{startSec, endSec},
	}
}
