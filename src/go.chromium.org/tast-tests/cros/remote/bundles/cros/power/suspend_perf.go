// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/common/chrome/histogram"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/remote/tracing"
	powerpb "go.chromium.org/tast-tests/cros/services/cros/power"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

const (
	// traceCmdEventsVarName is the name of the variable to specify events for trace-cmd to collect.
	traceCmdEventsVarName = "power.SuspendPerf.traceCmdEvents"

	// enablePerfettoVarName is the name of the variable to enable Perfetto trace.
	enablePerfettoVarName = "power.SuspendPerf.enablePerfetto"
)

var traceCmdEventsVar = testing.RegisterVarString(
	traceCmdEventsVarName,
	"",
	"Comma-separated events to enable trace-cmd and to ask it to record. (e.g. 'syscalls,sched:*')",
)

var enablePerfettoVar = testing.RegisterVarString(
	enablePerfettoVarName,
	"",
	"Boolean value to enable Perfetto to record. Use 'yes or 'no'",
)

type testArgsForSuspendPerf struct {
	numSuspend int
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         SuspendPerf,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests that performance of suspend/resume",
		Contacts: []string{
			"cros-suspend-resume@google.com",
			"mhiramat@google.com",
		},
		BugComponent: "b:256693104",
		Attr:         []string{"group:crosbolt", "crosbolt_perbuild"},
		SoftwareDeps: []string{"chrome"},
		ServiceDeps:  []string{"tast.cros.power.SuspendPerfService", "tast.cros.tracing.TraceCmdService", "tast.cros.tracing.PerfettoTraceService"},
		// (40 sec for histograms + 10 + 60 sec suspend/resume) * 5 times
		Timeout: 10 * time.Minute,
		Params: []testing.Param{{
			Name: "",
			Val: testArgsForSuspendPerf{
				numSuspend: 5,
			},
		}},
	})
}

const (
	defaultSuspendSeconds       = 10
	defaultRedialTimeoutSeconds = 60

	defaultInstanceName = "suspend_perf"
	defaultBufferSize   = 10240
)

// TODO make a new struct type with name and direction.
var defaultMetrics = []string{"Power.KernelSuspendTimeOnAC", "Power.KernelResumeTimeOnAC", "Power.DisplayAfterResumeDurationMsOnAC"}

// Delay and timeout for waitHistogramsUpdate().
var defaultWaitInterval = time.Duration(2) * time.Second
var defaultWaitTimeout = time.Duration(40) * time.Second

var remoteCommandTimeout = time.Duration(3) * time.Second

func SuspendPerf(ctx context.Context, s *testing.State) {
	args := s.Param().(testArgsForSuspendPerf)

	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer cl.Close(ctx)

	if err := initTracing(ctx, cl); err != nil {
		s.Log("Failed to initialize tracing, but this is ignorable: ", err)
	}
	defer cleanupTracing(ctx, s, cl)

	// Login and setup
	service := powerpb.NewSuspendPerfServiceClient(cl.Conn)
	if _, err := service.Prepare(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to login on DUT: ", err)
	}

	// Get old (before the suspend) histograms if exist. Usually this is empty.
	older, err := getHistograms(ctx, service, defaultMetrics)
	if err != nil {
		s.Fatal("Failed to get Histograms from DUT: ", err)
	}

	prev := older
	seconds := defaultSuspendSeconds

	for i := 0; i < args.numSuspend; i++ {
		tok := startTracing(ctx, s, cl)

		// Suspend and resume
		s.Logf("Suspending DUT for %d seconds", seconds)
		req := powerpb.SuspendRequest{Seconds: int32(seconds)}
		if res, err := service.Suspend(ctx, &req); err != nil {
			if res != nil && res.Failed {
				if res.Output != "" {
					s.Logf("Suspend command failed, the command output is: %s", res.Output)
				}
				s.Fatal("Failed to suspend DUT: ", err)
			}
			s.Log("Ignore suspend command error if connection is lost: ", err)

			// Reconnect because suspend can disconnect network.
			cl, err = redialRPC(ctx, s.DUT(), s.RPCHint(), defaultRedialTimeoutSeconds+seconds)
			if err != nil {
				s.Fatal("Failed to reconnect the RPC: ", err)
			}
			// defer cl.Close() is already set.
		}
		s.Log("Resumed")

		s.Log("Wait for suspend metrics update")
		service = powerpb.NewSuspendPerfServiceClient(cl.Conn)
		prev, err = waitHistogramsUpdate(ctx, service, prev)
		if err != nil {
			s.Fatal("Could not observe histogram update: ", err)
		}
		saveTraceData(ctx, s, cl, tok, i)
	}
	newer := prev

	// Make differences of Histograms.
	diff, err := histogram.DiffHistograms(older, newer)
	if err != nil {
		s.Fatal("Failed to make difference of histograms: ", err)
	}

	// Write perf metrics from the Diff Histogram and save it.
	pv := perf.NewValues()
	writeMetricsFromHistograms(diff, pv)
	err = pv.Save(s.OutDir())
	if err != nil {
		s.Fatal("Failed saving perf data: ", err)
	}
}

func redialRPC(ctx context.Context, dut *dut.DUT, hint *testing.RPCHint, timeoutSeconds int) (*rpc.Client, error) {
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		// Set a short timeout to the iteration in case of
		// any SSH operations blocking for a long time.
		ctx, cancel := context.WithTimeout(ctx, remoteCommandTimeout)
		defer cancel()

		if err := dut.WaitConnect(ctx); err != nil {
			return errors.Wrap(err, "failed to connect to DUT after suspend")
		}
		return nil
	}, &testing.PollOptions{
		Timeout:  time.Duration(timeoutSeconds) * time.Second,
		Interval: defaultWaitInterval,
	}); err != nil {
		return nil, err
	}

	return rpc.Dial(ctx, dut, hint)
}

func shouldRunTraceCmd() bool {
	return traceCmdEventsVar.Value() != ""
}

func shouldRunPerfetto() bool {
	return enablePerfettoVar.Value() == "yes"
}

// initTracing creates an trace-cmd instance in DUT.
func initTracing(ctx context.Context, cl *rpc.Client) error {
	if !shouldRunTraceCmd() {
		return nil
	}
	// Add a new tracing instance for this test
	// TODO: tune the parameters
	_, err := tracing.NewRemoteInstance(ctx, cl, defaultInstanceName,
		tracing.CPUBufferKiB(defaultBufferSize),
		tracing.InitialStop(),
		tracing.EnableEvents(strings.Split(traceCmdEventsVar.Value(), ",")...))
	return err
}

// startTraceCmd attaches to trace-cmd and start it.
func startTraceCmd(ctx context.Context, cl *rpc.Client) error {
	if !shouldRunTraceCmd() {
		return nil
	}
	if err := tracing.StartRemoteInstanceTrace(ctx, cl, defaultInstanceName); err != nil {
		return errors.Wrap(err, "failed to reconnect tracing")
	}
	return nil
}

// startPerfetto starts perfetto and detach from it.
func startPerfetto(ctx context.Context, cl *rpc.Client) (*tracing.RemoteSessionToken, error) {
	if !shouldRunPerfetto() {
		return nil, nil
	}
	sess, err := tracing.StartRemoteSession(ctx, cl, tracing.WithConfigTextData(perfettoConfigData), tracing.InBackground())
	if err != nil {
		return nil, err
	}
	tok := sess.Token()
	return tok, nil
}

// startTracing starts tracing in DUT.
func startTracing(ctx context.Context, s *testing.State, cl *rpc.Client) *tracing.RemoteSessionToken {
	if err := startTraceCmd(ctx, cl); err != nil {
		s.Log("Failed to start trace-cmd, but this is ignorable: ", err)
	}

	tok, err := startPerfetto(ctx, cl)
	if err != nil {
		s.Log("Failed to start perfetto, but this is ignorable: ", err)
	}
	return tok
}

// saveTraceCmd fetches the trace data from DUT and save it in s.OutDir().
func saveTraceCmd(ctx context.Context, s *testing.State, cl *rpc.Client, i int) error {
	if !shouldRunTraceCmd() {
		return nil
	}
	dest := fmt.Sprintf("%s/trace-%d.dat", s.OutDir(), i)
	if err := tracing.SaveRemoteInstanceTraceData(ctx, cl, defaultInstanceName,
		func(src string) error {
			return s.DUT().GetFile(ctx, src, dest)
		}); err != nil {
		return errors.Wrap(err, "failed to copy the data file from DUT")
	}
	s.Logf("Save trace data into %q", dest)
	return nil
}

func savePerfetto(ctx context.Context, s *testing.State, cl *rpc.Client, tok *tracing.RemoteSessionToken, i int) error {
	if tok == nil {
		return nil
	}
	dest := fmt.Sprintf("%s/perfetto-%d.trace", s.OutDir(), i)
	if err := tracing.SaveRemoteSessionTraceData(ctx, cl, tok,
		func(src string) error {
			return s.DUT().GetFile(ctx, src, dest)
		}); err != nil {
		return errors.Wrap(err, "failed to copy the perfetto data file from DUT")
	}
	s.Logf("Save perfetto trace data into %q", dest)
	return nil
}

// saveTraceData fetches the trace data from DUT and save it in s.OutDir().
func saveTraceData(ctx context.Context, s *testing.State, cl *rpc.Client, tok *tracing.RemoteSessionToken, i int) {
	if err := saveTraceCmd(ctx, s, cl, i); err != nil {
		s.Log("Ignorable: Failed to save trace-cmd data: ", err)
	}

	if err := savePerfetto(ctx, s, cl, tok, i); err != nil {
		s.Log("Ignorable: Failed to save perfetto data: ", err)
	}
}

func cleanupTracing(ctx context.Context, s *testing.State, cl *rpc.Client) error {
	if !shouldRunTraceCmd() {
		return nil
	}
	s.Logf("Cleaning up a trace instance: %s", defaultInstanceName)
	return tracing.CleanupRemoteInstance(ctx, cl, defaultInstanceName)
}

func waitHistogramsUpdate(ctx context.Context, service powerpb.SuspendPerfServiceClient, prev []*histogram.Histogram) ([]*histogram.Histogram, error) {

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		curr, err := getHistograms(ctx, service, defaultMetrics)
		if err != nil {
			return err
		}

		diff, err := histogram.DiffHistograms(prev, curr)
		if err != nil {
			return err
		}

		for _, h := range diff {
			if h.TotalCount() == 0 {
				return errors.Errorf("histogram %s is still empty", h.Name)
			}
		}
		return nil
	}, &testing.PollOptions{
		Timeout:  defaultWaitTimeout,
		Interval: defaultWaitInterval,
	}); err != nil {
		return nil, err
	}

	return getHistograms(ctx, service, defaultMetrics)
}

func getHistograms(ctx context.Context, service powerpb.SuspendPerfServiceClient, names []string) ([]*histogram.Histogram, error) {
	var hists []*histogram.Histogram
	for _, n := range names {
		hist, err := getHistogram(ctx, service, n)
		if err != nil {
			return nil, err
		}
		hists = append(hists, hist)
	}
	return hists, nil
}

func getHistogram(ctx context.Context, service powerpb.SuspendPerfServiceClient, name string) (*histogram.Histogram, error) {

	req := powerpb.HistogramRequest{Name: name}

	res, err := service.GetHistogram(ctx, &req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get a histogram of "+name)
	}

	return histogram.NewHistogramFromProto(res), nil
}

func writeMetricsFromHistograms(hs []*histogram.Histogram, pv *perf.Values) {
	for _, h := range hs {
		writeMetricsFromHistogram(h, pv)
	}
}

// writeMetricsFromHistogram writes <histname>_mean, <histname>_p50 (median), <histname>_p100 (max) to @pv
func writeMetricsFromHistogram(hist *histogram.Histogram, pv *perf.Values) {
	if hist.TotalCount() == 0 {
		return
	}
	mean, err := hist.Mean()
	if err == nil {
		pv.Set(perf.Metric{
			Name:      fmt.Sprintf("%s_mean", hist.Name),
			Unit:      "ms",
			Direction: perf.SmallerIsBetter,
		}, mean)
	}
	p50, err := hist.Percentile(50)
	if err == nil {
		pv.Set(perf.Metric{
			Name:      fmt.Sprintf("%s_p50", hist.Name),
			Unit:      "ms",
			Direction: perf.SmallerIsBetter,
		}, p50)
	}
	p100, err := hist.Percentile(100)
	if err == nil {
		pv.Set(perf.Metric{
			Name:      fmt.Sprintf("%s_p100", hist.Name),
			Unit:      "ms",
			Direction: perf.SmallerIsBetter,
		}, p100)
	}
}

const (
	// Perfetto configuration data, generated by https://ui.perfetto.dev/#!/record
	perfettoConfigData = `
buffers: {
	size_kb: 63488
	fill_policy: DISCARD
}
buffers: {
	size_kb: 2048
	fill_policy: DISCARD
}
data_sources: {
	config {
		name: "android.packages_list"
		target_buffer: 1
	}
}
data_sources: {
	config {
		name: "linux.process_stats"
		target_buffer: 1
		process_stats_config {
			scan_all_processes_on_start: true
			proc_stats_poll_ms: 1000
		}
	}
}
data_sources: {
	config {
		name: "track_event"
		chrome_config {
			trace_config: "{\"record_mode\":\"record-continuously\",\"included_categories\":[\"power\",\"disabled-by-default-power\",\"log\",\"toplevel\",\"cc\",\"gpu\",\"viz\",\"ui\",\"views\"],\"excluded_categories\":[\"*\"],\"memory_dump_config\":{}}"
			privacy_filtering_enabled: false
			client_priority: USER_INITIATED
		}
		track_event_config {
			disabled_categories: "*"
			enabled_categories: "power"
			enabled_categories: "disabled-by-default-power"
			enabled_categories: "log"
			enabled_categories: "toplevel"
			enabled_categories: "cc"
			enabled_categories: "gpu"
			enabled_categories: "viz"
			enabled_categories: "ui"
			enabled_categories: "views"
			enabled_categories: "__metadata"
			timestamp_unit_multiplier: 1000
			filter_debug_annotations: false
			enable_thread_time_sampling: true
			filter_dynamic_event_names: false
		}
	}
}
data_sources: {
	config {
		name: "linux.sys_stats"
		sys_stats_config {
			psi_period_ms: 100
		}
	}
}
data_sources: {
	config {
		name: "linux.sys_stats"
		sys_stats_config {
			vmstat_period_ms: 1000
			stat_period_ms: 1000
			stat_counters: STAT_CPU_TIMES
			stat_counters: STAT_FORK_COUNT
		}
	}
}
data_sources: {
	config {
		name: "linux.ftrace"
		ftrace_config {
			ftrace_events: "sched/sched_switch"
			ftrace_events: "power/suspend_resume"
			ftrace_events: "sched/sched_wakeup"
			ftrace_events: "sched/sched_wakeup_new"
			ftrace_events: "sched/sched_waking"
			ftrace_events: "regulator/regulator_set_voltage"
			ftrace_events: "regulator/regulator_set_voltage_complete"
			ftrace_events: "power/clock_enable"
			ftrace_events: "power/clock_disable"
			ftrace_events: "power/clock_set_rate"
			ftrace_events: "sched/sched_process_exit"
			ftrace_events: "sched/sched_process_free"
			ftrace_events: "task/task_newtask"
			ftrace_events: "task/task_rename"
			ftrace_events: "irq/*"
			ftrace_events: "timer/*"
		}
	}
}
duration_ms: 60000
flush_period_ms: 30000
incremental_state_config {
	clear_period_ms: 5000
}`
)
