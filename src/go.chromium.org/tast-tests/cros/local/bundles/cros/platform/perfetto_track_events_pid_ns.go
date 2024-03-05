// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package platform

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/tracing"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

const (
	// trackEventsPidNSConfigFile is the data path of the trace config in text proto format.
	// The config enables several ftrace events and all track events (produced using the SDk).
	trackEventsPidNSConfigFile = "perfetto/track_events_pid_ns.pbtxt"

	// trackEventsPidNSQueryFile is the data path of the SQL query to post-process and verify the collected trace data.
	trackEventsPidNSQueryFile = "perfetto/track_events_pid_ns.sql"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: PerfettoTrackEventsPidNS,
		Desc: "Tests Perfetto's support of tracing PID-namespaced processes",
		Contacts: []string{
			"baseos-perf@google.com",
			"chinglinyu@chromium.org",
		},
		BugComponent: "b:1069482", // ChromeOS > Platform > System > Performance > CrOSetto (Tracing)
		Data:         []string{trackEventsPidNSConfigFile, trackEventsPidNSQueryFile},
		Attr:         []string{"group:mainline", "informational"},
	})
}

func verifyTrackEventPid(ctx context.Context, s *testing.State, sess *tracing.Session) {
	if err := sess.Stop(ctx); err != nil {
		s.Fatal("Failed to stop tracing: ", err)
	}

	// Process the trace data with the SQL query and get [][]string as the result.
	// See the content of trackEventsPidNSQueryFile for details.
	// Example result:
	// {
	//   { "name", "tid" }
	//   { "Trial1", "6838" }
	// }
	res, err := sess.RunQuery(ctx, s.DataPath(trackEventsPidNSQueryFile))
	if err != nil {
		s.Fatal("Failed to process the trace data: ", err)
	}

	if len(res) != 2 {
		s.Fatal("Failed to verify PID of track events: the query returns empty results")
	}

	if pid, err := strconv.Atoi(res[1][1]); err != nil || pid <= 0 {
		s.Fatalf("Failed to verify PID of track events: malformed query result: %q", res)
	}

	// On test success, remove the trace data file.
	if err := os.Remove(sess.TraceDataPath()); err != nil {
		s.Fatal("Failed to remove the trace data: ", err)
	}
}

// PerfettoTrackEventsPidNS tests tracing PID-namespaced processes.
// The test runs the perfetto_simple_producer binary within a PID namespace (starting with minijail -p)
// and checks whether the track events are associated with the root-level PID.
func PerfettoTrackEventsPidNS(ctx context.Context, s *testing.State) {
	ctxForCleanup := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 3*time.Second)
	defer cancel()

	// Start a trace session using the perfetto command line tool.
	traceConfigPath := s.DataPath(trackEventsPidNSConfigFile)
	sess, err := tracing.StartSession(ctx, traceConfigPath,
		tracing.WithTraceDataPath(filepath.Join(s.OutDir(), "perfetto-trace.pb")),
		tracing.WithCompression())
	if err != nil {
		s.Fatal("Failed to start tracing: ", err)
	}
	defer sess.Finalize(ctxForCleanup)

	// Wait until tracing is done and verify the collected trace data using the trace processor.
	// This doesn't perform cleanup so it runs with ctx.
	defer verifyTrackEventPid(ctx, s, sess)

	// GoBigSleepLint: sleep to ensure that the minijailed process sends trace data to a running tracing session.
	if err := testing.Sleep(ctx, 2*time.Second); err != nil {
		s.Fatal("Failed to sleep to wait for the tracing session: ", err)
	}

	// Run the perfetto_simple_producer process with a new PID namespace.
	cmd := testexec.CommandContext(ctx, "minijail0", "-p", "/usr/local/bin/perfetto_simple_producer")
	if err := cmd.Run(testexec.DumpLogOnError); err != nil {
		s.Fatal("Failed to run the producer process: ", err)
	}

	// Run deferred function verifyTrackEventPid to assert the PID of track events.
}
