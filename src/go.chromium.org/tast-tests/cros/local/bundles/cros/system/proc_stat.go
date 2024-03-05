// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package system

import (
	"context"
	"os"
	"time"

	"golang.org/x/sys/unix"

	"go.chromium.org/tast-tests/cros/local/profiler"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: ProcStat,
		Desc: "Check profiler.ProcStat",
		Contacts: []string{
			"chromeos-audio-bugs@google.com",
			"aaronyu@google.com", // test author
		},
		BugComponent: "b:776546",
		Attr: []string{
			"group:mainline",
			"group:hw_agnostic",
			"informational", // TODO(b/285592661): Promote.
		},
		Timeout: 10 * time.Second,
	})
}

func ProcStat(ctx context.Context, s *testing.State) {
	const (
		tol        = 100 * time.Millisecond
		stressTime = time.Second
	)

	pid := os.Getpid()

	var out profiler.ProcStatOutput

	instance, err := profiler.Start(ctx, s.OutDir(), profiler.ProcStat(&out, pid))
	if err != nil {
		s.Fatal("Failed to start profiler: ", err)
	}
	var rusageStart unix.Rusage
	if err := unix.Getrusage(unix.RUSAGE_SELF, &rusageStart); err != nil {
		s.Fatal("unix.Getrusage: ", err)
	}

	// stress CPU
	until := time.After(stressTime)
loop:
	for {
		select {
		case <-until:
			break loop
		default:
		}
	}

	if err := instance.End(ctx); err != nil {
		s.Fatal("Failed to stop profiler: ", err)
	}
	var rusageEnd unix.Rusage
	if err := unix.Getrusage(unix.RUSAGE_SELF, &rusageEnd); err != nil {
		s.Fatal("unix.Getrusage: ", err)
	}

	if out.WallTime < stressTime-tol || out.WallTime > stressTime+tol {
		s.Errorf("Expected WallTime to be %s, got %s", stressTime, out.WallTime)
	}
	expectedUserTime := time.Unix(rusageEnd.Utime.Unix()).Sub(time.Unix(rusageStart.Utime.Unix()))
	if out.UserTime < expectedUserTime-tol || out.UserTime > expectedUserTime+tol {
		s.Errorf("Expected UserTime to be %s, got %s", expectedUserTime, out.UserTime)
	}
	if out.SysTime > tol {
		s.Errorf("Expected SysTime to be 0, got %s", out.SysTime)
	}
}
