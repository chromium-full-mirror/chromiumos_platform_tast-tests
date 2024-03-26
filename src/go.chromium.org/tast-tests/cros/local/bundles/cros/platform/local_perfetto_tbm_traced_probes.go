// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package platform

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"android.googlesource.com/platform/external/perfetto/protos/perfetto/metrics/github.com/google/perfetto/perfetto_proto"
	"go.chromium.org/tast/core/ctxutil"

	"go.chromium.org/tast-tests/cros/local/tracing"

	"go.chromium.org/tast/core/testing"
)

const (
	traceMetricCPU = "android_cpu"
	traceMetricMEM = "android_mem"

	targetProcessName = "/usr/bin/traced_probes"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: LocalPerfettoTBMTracedProbes,
		Desc: "Verifies functions of Perfetto traced, traced_probes and trace_processor_shell commands",
		Contacts: []string{
			"baseos-perf@google.com",
			"chenghaoyang@chromium.org",
			"chinglinyu@chromium.org",
		},
		BugComponent: "b:1069482", // ChromeOS > Platform > System > Performance > CrOSetto (Tracing)
		Data:         []string{tracing.TBMTracedProbesConfigFile},
		Attr:         []string{"group:mainline"},
	})
}

// processCPUMetrics extracts information of the target process in the
// cpu metric.
func processCPUMetrics(cpuMetric *perfetto_proto.AndroidCpuMetric, s *testing.State) {
	for _, processInfo := range cpuMetric.GetProcessInfo() {
		if processInfo.GetName() == targetProcessName {
			metric := processInfo.GetMetrics()
			s.Log("megacycles: ", metric.GetMcycles())
			s.Log("runtime in nanosecond: ", metric.GetRuntimeNs())
			s.Log("min_freq in kHz: ", metric.GetMinFreqKhz())
			s.Log("max_freq in kHz: ", metric.GetMaxFreqKhz())
			s.Log("avg_freq in kHz: ", metric.GetAvgFreqKhz())

			return
		}
	}

	s.Fatal("Failed to find the CPU metrics of the target process")
}

// processMemMetrics extracts information of the target process in the
// mem metric.
func processMemMetrics(memMetric *perfetto_proto.AndroidMemoryMetric, s *testing.State) {
	for _, processMetric := range memMetric.GetProcessMetrics() {
		if processMetric.GetProcessName() == targetProcessName {
			counters := processMetric.GetTotalCounters()
			s.Log("anon_avg in rss: ", counters.GetAnonRss().GetAvg())
			s.Log("file_avg in rss: ", counters.GetFileRss().GetAvg())
			s.Log("swap_avg in rss: ", counters.GetSwap().GetAvg())

			return
		}
	}

	s.Fatal("Failed to find the memory metrics of the target process")
}

// LocalPerfettoTBMTracedProbes tests perfetto trace collection on
// traced_probes and process the trace result with trace_processor_shell.
func LocalPerfettoTBMTracedProbes(ctx context.Context, s *testing.State) {
	ctxForCleanup := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 3*time.Second)
	defer cancel()

	// Start a trace session using the perfetto command line tool.
	traceConfigPath := s.DataPath(tracing.TBMTracedProbesConfigFile)
	traceDataPath := filepath.Join(s.OutDir(), "perfetto-trace.pb")
	sess, err := tracing.StartSession(ctx, traceConfigPath,
		tracing.WithTraceDataPath(traceDataPath),
		tracing.WithCompression())

	if err != nil {
		s.Fatal("Failed to start tracing: ", err)
	}
	defer sess.Finalize(ctxForCleanup)

	// Developers can run other tests to trigger more trace data.
	const pauseDuration = time.Second * 10
	// GoBigSleepLint: sleep to let the tracing session have time collecting trace events.
	if err := testing.Sleep(ctx, pauseDuration); err != nil {
		s.Fatal("Failed to sleep while waiting for overview to trigger: ", err)
	}

	// The trace config uses a long duration. Terminate the session with sess.Stop() before the full trace duration elapses to avoid context timeout in trace processing.
	if err := sess.Stop(); err != nil {
		s.Fatal("Failed to stop the tracing session: ", err)
	}

	// Run the trace processor and get CPU and memory metrics from the raw trace data.
	metrics, err := sess.RunMetrics(ctx, []string{traceMetricCPU, traceMetricMEM})
	if err != nil {
		s.Fatal("Failed to RunMetrics: ", err)
	}

	// Check if the target process has CPU and memory metrics.
	processCPUMetrics(metrics.GetAndroidCpu(), s)
	processMemMetrics(metrics.GetAndroidMem(), s)

	// Skip removal of the trace file on test error.
	if s.HasError() {
		return
	}
	// We don't need the trace data for debugging on test success.
	if err := os.Remove(traceDataPath); err != nil {
		s.Error("Failed to remove the trace data file: ", err)
	}
}
