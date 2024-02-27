// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package lifecycle

import (
	"context"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type stressSystemAction struct {
	schedule
}

var _ Action = stressSystemAction{}

func (a stressSystemAction) Do(ctx context.Context, s *testing.State, t *tester) {
	t.logAction(ctx, a.startSec, "start stressing", true)
	stressCtx, cancel := context.WithDeadline(
		ctx,
		t.t0.Add(time.Duration(a.endSec)*time.Second+extraTimeout),
	)
	defer cancel()

	cmd := testexec.CommandContext(
		stressCtx, "stressapptest",
		"-s", strconv.Itoa(a.endSec-a.startSec),
		"-W",
	)
	err := cmd.Run()
	t.logAction(ctx, a.endSec, "end stressing", false)
	if err != nil && !errors.Is(err, context.DeadlineExceeded) {
		s.Fatal("Stress failed: ", err)
	}
}

func (a stressSystemAction) maybeLogSchedule(ctx context.Context, t *tester) {
	t.logScheduleRow(ctx, "stress system", '@', a.schedule)
}

// StressSystem returns a Action that will add stressapptest load in the given
// period of time. When stressapptest is still running at endSec, it will
// be killed on context level.
func StressSystem(startSec, endSec int) Action {
	return &stressSystemAction{
		schedule: schedule{startSec, endSec},
	}
}
