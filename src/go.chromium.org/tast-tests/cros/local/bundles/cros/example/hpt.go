// Copyright 2023 The ChromiumOS Authors
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
		Func:         HPT,
		Desc:         "Demonstrates how to use perf.go file for HPT",
		Contacts:     []string{"hpt-project-team@google.com", "acwans@google.com"},
		BugComponent: "b:1094001", // ChromeOS > EngProd > Platform > Base OS
		Attr:         []string{"group:mainline", "informational"},
	})
}

func HPT(ctx context.Context, s *testing.State) {

	profs := []profiler.Profiler{
		// Equivalent to perf record -g -a -e cycles
		profiler.Perf(profiler.PerfRecordOpts("cycles",
			&profiler.PerfRecordSamplingRate{RateType: profiler.PerfRecordFrequency, Value: 8000},
			profiler.PerfRecordCallgraph)),
	}

	p, err := profiler.Start(ctx, "/var/tmp", profs...)
	if err != nil {
		s.Fatal("Failure in starting the profiler: ", err)
	}

	defer func() {
		if err := p.End(ctx); err != nil {
			s.Error("Trace may not be saved due to failure in ending the profiler: ", err)
		} else {
			s.Log("Trace saved")
		}
	}()

	// Wait for 5 seconds to gather perf_record.data.
	if err := testing.Sleep(ctx, 5*time.Second); err != nil {
		s.Fatal("Failure in sleeping: ", err)
	}
}
