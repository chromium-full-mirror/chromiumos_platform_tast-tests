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

type playbackAction struct {
	isPlaybackAction
	schedule
	// Block size for the playback. If 0 uses cras_test_client's default.
	blockSize uint
}

var _ PlaybackAction = playbackAction{}

func (a playbackAction) Do(ctx context.Context, s *testing.State, t *tester) {
	t.logAction(ctx, a.startSec, "start playback", true)
	playbackCtx, cancel := context.WithDeadline(
		ctx,
		t.t0.Add(time.Duration(a.endSec)*time.Second+extraTimeout),
	)
	defer cancel()
	cmd := testexec.CommandContext(
		playbackCtx,
		"cras_test_client",
		fmt.Sprintf("--playback_file=%s", t.playbackRaw),
	)
	if a.blockSize != 0 {
		cmd.Args = append(cmd.Args, fmt.Sprintf("--block_size=%d", a.blockSize))
	}
	err := cmd.Run()
	t.logAction(ctx, a.endSec, "end playback", false)
	if err != nil {
		s.Fatal("Playback failed: ", err)
	}
}

func (a playbackAction) maybeLogSchedule(ctx context.Context, t *tester) {
	t.logScheduleRow(ctx, "playback", 'p', a.schedule)
}

// Playback returns a Playback which plays synthesized audio in the given interval
func Playback(startSec, endSec int) PlaybackAction {
	return &playbackAction{
		schedule: schedule{startSec, endSec},
	}
}

// PlaybackWithBlockSize is like Playback but plays with the specified block size.
func PlaybackWithBlockSize(startSec, endSec int, blockSize uint) PlaybackAction {
	return &playbackAction{
		schedule:  schedule{startSec, endSec},
		blockSize: blockSize,
	}
}
