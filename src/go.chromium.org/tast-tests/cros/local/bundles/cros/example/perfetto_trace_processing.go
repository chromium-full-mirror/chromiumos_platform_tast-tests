// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package example

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"time"

	"go.chromium.org/tast-tests/cros/local/tracing"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

const (
	traceConfig = "perfetto_trace_cfg.pbtxt"
	traceQuery  = "perfetto_trace_query.sql"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: PerfettoTraceProcessing,
		Desc: "Exercises post-processing perfetto traces",
		Contacts: []string{
			"baseos-perf@google.com",
			"chinglinyu@chromium.org",
		},
		BugComponent: "b:1069482", // ChromeOS > Platform > System > Performance > CrOSetto (Tracing)
		Data:         []string{traceConfig, traceQuery},
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
	})
}

// PerfettoTraceProcessing exercises tracing.Session.RunQuery() and
// tracing.Session.RunQueryString() functions to show how to post-process a
// collected trace using a SQL query.
func PerfettoTraceProcessing(ctx context.Context, s *testing.State) {
	// This test requires compressing the trace data on finalizing the trace session.
	// Reserve some time for Finalize() to do it.
	ctxForCleanup := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// We don't need to run any test action during the tracing session.
	// Just use the blocking version of StartSession() for simplicity.
	traceDataPath := filepath.Join(s.OutDir(), "perfetto-trace.pb")
	sess, err := tracing.StartSessionAndWaitUntilDone(ctx, s.DataPath(traceConfig),
		tracing.WithTraceDataPath(traceDataPath), // Save trace data to traceDataPath.
		tracing.WithCompression())                // Also compress the trace data.

	if err != nil {
		s.Fatal("Failed to start tracing: ", err)
	}

	// Perform final actions of the trace session.
	defer sess.Finalize(ctxForCleanup)

	// Process the trace data using inline string query for simple queries.
	res1, err := sess.RunQueryString(ctx, "select cmdline from process where pid=1")
	if err != nil {
		s.Fatal("Failed to process the trace data: ", err)
	}
	if len(res1) != 2 || res1[1][0] != "/sbin/init" {
		s.Fatalf("Unexpected query result: %q", res1)
	}

	// Process the trace data using external SQL query file. This is preferable if the query is complex.
	res2, err := sess.RunQuery(ctx, s.DataPath(traceQuery))
	if err != nil {
		s.Fatal("Failed to process the trace data: ", err)
	}
	// We should get identical results using the same query.
	if !reflect.DeepEqual(res1, res2) {
		s.Fatalf("Unexpected query result: %q", res2)
	}

	// We don't need the trace data for debugging on test success.
	if err := os.Remove(traceDataPath); err != nil {
		s.Error("Failed to remove the trace data file: ", err)
	}
}
