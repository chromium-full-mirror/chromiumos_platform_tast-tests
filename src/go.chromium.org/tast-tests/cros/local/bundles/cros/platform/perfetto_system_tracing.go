// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package platform

import (
	"context"
	"os"
	"time"

	"go.chromium.org/tast-tests/cros/local/tracing"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: PerfettoSystemTracing,
		Desc: "Verifies functions of Perfetto traced and traced_probes",
		Contacts: []string{
			"baseos-perf@google.com",
			"chinglinyu@chromium.org",
		},
		BugComponent: "b:1069482", // ChromeOS > Platform > System > Performance > CrOSetto (Tracing)
		Data:         []string{tracing.TraceConfigFile},
		Attr:         []string{"group:mainline"},
	})
}

// collectTraceData collect a system-wide trace using the perfetto command line
// too.
func collectTraceData(ctx context.Context, traceConfigPath string) error {
	ctxForCleanup := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Second)
	defer cancel()

	sess, err := tracing.StartSessionAndWaitUntilDone(ctx, traceConfigPath)
	if err != nil {
		return err
	}
	defer sess.Finalize(ctxForCleanup)

	// Validate the trace data.
	stat, err := os.Stat(sess.TraceDataPath())
	if err != nil {
		return errors.Wrapf(err, "unexpected error stating %s", sess.TraceDataPath())
	}

	testing.ContextLogf(ctx, "Collected %d bytes of trace data", stat.Size())
	return nil
}

// PerfettoSystemTracing tests perfetto system-wide trace collection.
func PerfettoSystemTracing(ctx context.Context, s *testing.State) {
	// The tracing service daemons are started by default. Check their status.
	// Remember the PID of both jobs to verify that the jobs didn't have seccomp crash during trace collection.
	tracedPID, tracedProbesPID, err := tracing.CheckTracingServices(ctx)
	if err != nil {
		s.Fatal("Tracing services not running: ", err)
	}

	traceConfigPath := s.DataPath(tracing.TraceConfigFile)
	if err := collectTraceData(ctx, traceConfigPath); err != nil {
		s.Fatal("Failed to collect trace data: ", err)
	}

	tracedPID2, tracedProbesPID2, err := tracing.CheckTracingServices(ctx)
	if err != nil {
		s.Fatal("Tracing services not running after trace collection: ", err)
	}

	// Check that PID stays the same as a heuristic that the jobs didn't crash during the test.
	if tracedPID != tracedPID2 {
		s.Errorf("Unexpected respawn of job %s (PID changed from %d to %d)", tracing.TracedJobName, tracedPID, tracedPID2)
	}
	if tracedProbesPID != tracedProbesPID2 {
		s.Errorf("Unexpected respawn of job %s (PID changed from %d to %d)", tracing.TracedProbesJobName, tracedProbesPID, tracedProbesPID2)
	}
}
