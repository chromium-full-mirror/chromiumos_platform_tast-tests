// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"google.golang.org/protobuf/proto"
)

const (
	// chromettoStackProfilingTracingTime gives the time we sleep before
	// stopping the trace. See comments below.
	chromettoStackProfilingTracingTime = 15 * time.Second
	// chromettoStackProfilingCleanupTime is how long we reserve for cleanup.
	chromettoStackProfilingCleanupTime = 15 * time.Second
	// chromettoStackProfilingTimeout is the total timeout for the test.
	chromettoStackProfilingTimeout = time.Minute + chromettoStackProfilingTracingTime + chromettoStackProfilingCleanupTime
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ChromettoStackProfiling,
		Desc:         "Checks that Chrometto can gather stack data",
		Contacts:     []string{"chrometto-bugs@google.com"},
		BugComponent: "b:582280", // Android > Android OS & Apps > Web on Android Performance > Chrometto
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome", "stack_sampled_metrics"},
		Timeout:      chromettoStackProfilingTimeout,
		Fixture:      fixture.ChromeLoggedIn,
	})
}

// ChromettoStackProfiling tests that chrometto can capture stack data.
func ChromettoStackProfiling(ctx context.Context, s *testing.State) {
	// Reserve a few seconds for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, chromettoStackProfilingCleanupTime)
	defer cancel()

	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	if err := cr.StartTracing(ctx, []string{"disabled-by-default-cpu_profiler"}); err != nil {
		s.Fatal("Failed to start tracing: ", err)
	}

	// GoBigSleepLint: Unfortunately, we can't do a proper testing.Poll here. The
	// only way to see the Chrometto data is to do StopTracing, so we can't poll
	// to see if we have already gotten the CPU profiling data we expect. Instead,
	// we sleep for a while and hope the profiler has done its work by the time
	// we wake up.
	if err := testing.Sleep(ctx, chromettoStackProfilingTracingTime); err != nil {
		cr.StopTracing(cleanupCtx)
		s.Fatal("Failed to wait for profiling: ", err)
	}

	data, err := cr.StopTracing(ctx)
	if err != nil {
		s.Fatal("Failed to stop tracing: ", err)
	}

	// Find CPU profiles in the Trace.
	var foundCPUProfile bool
	for _, trace := range data.Packet {
		if streamingProfile := trace.GetStreamingProfilePacket(); streamingProfile != nil {
			foundCPUProfile = true
			break
		}
	}

	if !foundCPUProfile {
		s.Error("No CPU profiles found in Trace data")

		// Save the trace file so we can see what went wrong.
		serialized, err := proto.Marshal(data)
		if err != nil {
			s.Fatal("Could not serialize trace data: ", err)
		}
		traceFilePath := filepath.Join(s.OutDir(), "chrometto-trace")
		if err := os.WriteFile(traceFilePath, serialized, 0644); err != nil {
			s.Fatal("Could not save trace data: ", err)
		}

		s.Log("Trace data saved to ", traceFilePath)
	}
}
