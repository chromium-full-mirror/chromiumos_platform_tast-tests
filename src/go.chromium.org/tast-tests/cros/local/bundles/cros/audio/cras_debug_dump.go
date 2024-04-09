// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

import (
	"context"
	"os/exec"
	"sync"
	"time"

	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/audio/debug"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CrasDebugDump,
		Desc:         "Check debug.Dump",
		Contacts:     []string{"chromeos-audio-bugs@google.com", "aaronyu@google.com"},
		BugComponent: "b:776546",
		Attr: []string{
			"group:mainline",
			"informational",
		},
		Timeout:      3 * time.Minute,
		LacrosStatus: testing.LacrosVariantUnneeded,
	})
}

func CrasDebugDump(ctx context.Context, s *testing.State) {
	_, err := audio.RestartCras(ctx)
	if err != nil {
		s.Fatal("Failed to restart CRAS: ", err)
	}

	var wg sync.WaitGroup
	defer wg.Wait()

	captureCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	wg.Add(1)
	go func(ctx context.Context) {
		defer wg.Done()
		cmd := exec.CommandContext(ctx, "cras_tests", "capture", "/dev/null", "--effects=0x301")
		if err := cmd.Run(); err != nil && ctx.Err() == nil {
			s.Fatal("Cannot run cras_tests capture: ", err)
		}
	}(captureCtx)

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		debugInfo, err := debug.Dump(ctx)
		if err != nil {
			return errors.Wrap(err, "debug.Dump")
		}
		if got, want := len(debugInfo.Streams), 1; got != want {
			return errors.Errorf("stream count: got %d; want %d", got, want)
		}
		if got, want := debugInfo.Streams[0].Effects, uint(0x301); got != want {
			return errors.Errorf("stream effect incorrect: got 0x%x; want 0x%x", got, want)
		}
		return nil
	}, &testing.PollOptions{
		Interval: 200 * time.Millisecond,
		Timeout:  1 * time.Second,
	}); err != nil {
		s.Fatal("Debug dump did not reach the desired condition: ", err)
	}
}
