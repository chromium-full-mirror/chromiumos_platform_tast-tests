// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package example

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/profiler"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: Profiler,
		Desc: "Demonstrates how to use profiler package",
		Contacts: []string{
			"baseos-perf@google.com",
			"chinglinyu@chromium.org",
		},
		BugComponent: "b:1034522", // ChromeOS > Test > Harness > Tast > Examples
		Attr:         []string{"group:mainline", "informational"},
	})
}

func Profiler(ctx context.Context, s *testing.State) {
	var perfStatCyclesPerSecondOutput profiler.PerfStatCyclesPerSecondOutput
	var perfStatInstuctionsAtIntervalsOutput profiler.PerfStatInstructionsAtIntervalsOutput

	profs := []profiler.Profiler{
		profiler.Top(&profiler.TopOpts{
			Interval: 2 * time.Second,
		}),
		profiler.VMStat(nil),
		profiler.Perf(profiler.PerfStatRecordOpts()),
		// Get CPU cycle count for all processes.
		profiler.Perf(profiler.PerfStatCyclesPerSecondOpts(&perfStatCyclesPerSecondOutput, profiler.PerfAllProcs)),
		// Get instructions at 500ms intervals.
		profiler.Perf(profiler.PerfStatInstructionsAtIntervalsOpts(&perfStatInstuctionsAtIntervalsOutput, 500)),
	}

	p, err := profiler.Start(ctx, s.OutDir(), profs...)
	if err != nil {
		s.Fatal("Failure in starting the profiler: ", err)
	}

	defer func() {
		if err := p.End(ctx); err != nil {
			s.Error("Failure in ending the profiler: ", err)
		}
		s.Log("All CPU cycle count per second: ", perfStatCyclesPerSecondOutput.CyclesPerSecond)
		s.Log("Instructions at intervals:")
		for _, inst := range perfStatInstuctionsAtIntervalsOutput.InstructionsAtIntervals {
			s.Log("  t=", inst.Timestamp, ",  inst=", inst.Value)
		}
	}()

	// GoBigSleepLint: Wait for 2 seconds to gather perf.data
	if err := testing.Sleep(ctx, 2*time.Second); err != nil {
		s.Fatal("Failure in sleeping: ", err)
	}

}
