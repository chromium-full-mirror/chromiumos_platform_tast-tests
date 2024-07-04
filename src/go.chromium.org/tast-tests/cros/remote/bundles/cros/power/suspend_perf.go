// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/chrome/histogram"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/remote/cros/metrics"
	"go.chromium.org/tast-tests/cros/remote/memory/mempressure"
	"go.chromium.org/tast-tests/cros/remote/tracing"
	"go.chromium.org/tast-tests/cros/remote/tracing/linuxperf"
	powerpb "go.chromium.org/tast-tests/cros/services/cros/power"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
	"google.golang.org/protobuf/types/known/emptypb"
)

const (
	// forceTabsVarName is the name of the variable to specify the number of tabs opened forcibly.
	forceTabsVarName = "power.SuspendPerf.forceTabs"

	// traceCmdEventsVarName is the name of the variable to specify events for trace-cmd to collect.
	traceCmdEventsVarName = "power.SuspendPerf.traceCmdEvents"

	// enablePerfettoVarName is the name of the variable to enable Perfetto trace.
	enablePerfettoVarName = "power.SuspendPerf.enablePerfetto"

	// perfEventVarName is the name of the variable to specify events for trace-cmd to collect.
	perfEventVarName = "power.SuspendPerf.perfEvent"
)

var forceTabsVar = testing.RegisterVarString(
	forceTabsVarName,
	"0",
	"The number of tabs to open forcibly. This option is for '*_mem' test variants",
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

var perfEventVar = testing.RegisterVarString(
	perfEventVarName,
	"",
	"Event name to trace by perf record (e.g. 'cpu-cycles', 'sched:sched_switch')",
)

type testArgsForSuspendPerf struct {
	numSuspend        int
	enableLacros      bool
	enableArc         bool
	enableMempressure bool
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
		SoftwareDeps: []string{"chrome"},
		ServiceDeps: []string{
			"tast.cros.browser.ChromeService",
			"tast.cros.browser.LacrosService",
			"tast.cros.power.SuspendPerfService",
			"tast.cros.tracing.TraceCmdService",
			"tast.cros.tracing.PerfettoTraceService",
			"tast.cros.tracing.linuxperf.LinuxPerfService",
			"tast.cros.ui.ConnService",
			"tast.cros.ui.TconnService",
		},
		Params: []testing.Param{{
			Name: "",
			Val: testArgsForSuspendPerf{
				numSuspend: 5,
			},
			ExtraAttr: []string{"group:crosbolt", "crosbolt_perbuild"},
			// (40 sec for histograms + 10 + 60 sec suspend/resume) * 5 times + open tabs.
			Timeout: 20 * time.Minute,
		}, {
			Name: "arc",
			Val: testArgsForSuspendPerf{
				numSuspend: 5,
				enableArc:  true,
			},
			Timeout: 20 * time.Minute,
		}, {
			Name: "arc_lacros",
			Val: testArgsForSuspendPerf{
				numSuspend:   5,
				enableArc:    true,
				enableLacros: true,
			},
			Timeout: 20 * time.Minute,
		}, {
			Name: "mem",
			Val: testArgsForSuspendPerf{
				numSuspend:        5,
				enableMempressure: true,
			},
			// mempressure will take another 20minutes
			Timeout: 30 * time.Minute,
		}, {
			Name: "arc_mem",
			Val: testArgsForSuspendPerf{
				numSuspend:        5,
				enableArc:         true,
				enableMempressure: true,
			},
			// mempressure will take another 20minutes
			Timeout: 30 * time.Minute,
		}, {
			Name: "arc_lacros_mem",
			Val: testArgsForSuspendPerf{
				numSuspend:        5,
				enableArc:         true,
				enableLacros:      true,
				enableMempressure: true,
			},
			ExtraAttr: []string{"group:crosbolt", "crosbolt_perbuild"},
			Timeout:   30 * time.Minute,
		}},
	})
}

const (
	defaultSuspendSeconds       = 10
	defaultRedialTimeoutSeconds = 60

	defaultInstanceName = "suspend_perf"
	defaultBufferSize   = 10240

	defaultCycleTabs = 10
)

type histogramRequest struct {
	Name       string
	FromLacros bool
}

// TODO make a new struct type with name and direction.
var defaultMetrics = []*histogramRequest{
	{Name: "Power.KernelSuspendTimeOnAC"},
	{Name: "Power.KernelResumeTimeOnAC"},
	{Name: "Power.DisplayAfterResumeDurationMsOnAC"},
	{Name: "Browser.Tabs.TotalSwitchDuration3", FromLacros: true},
}

// Delay and timeout for waitHistogramsUpdate().
var defaultWaitInterval = time.Duration(2) * time.Second
var defaultWaitTimeout = time.Duration(80) * time.Second

var remoteCommandTimeout = time.Duration(3) * time.Second

func SuspendPerf(ctx context.Context, s *testing.State) {
	args := s.Param().(testArgsForSuspendPerf)
	forceTabs, err := strconv.Atoi(forceTabsVar.Value())
	if err != nil {
		s.Fatal("Failed to convert ", forceTabsVarName, err)
	}
	pv := perf.NewValues()

	var useMetrics []*histogramRequest
	for _, m := range defaultMetrics {
		m.FromLacros = m.FromLacros && args.enableLacros
		useMetrics = append(useMetrics, m)
	}

	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer cl.Close(ctx)

	// Login test user for memory pressure and suspend/resume.
	if err := mempressure.NewTestEnv(ctx, cl.Conn, args.enableArc, args.enableLacros); err != nil {
		s.Fatal("Failed to initalize test environment: ", err)
	}

	// Launch tabs for tab switching time.
	mp, err := mempressure.NewRemoteMemoryPressure(ctx, cl.Conn, args.enableLacros)
	if err != nil {
		s.Fatal("Failed to make a RemoteMemoryPressure: ", err)
	}

	if args.enableMempressure {
		// Open tabs until at least one tab is discarded.
		if err := mp.Run(ctx, forceTabs, forceTabs, 0); err != nil {
			s.Fatal("Failed to run RemoteMemoryPressure: ", err)
		}
		s.Logf("Memory pressure: Opened tabs: %d, discarded tabs: %d", mp.OpenedTabs(), mp.DiscardedTabs(ctx))
	}

	tracer := &compoundTracers{}

	if err := tracer.init(ctx, cl); err != nil {
		s.Log("Failed to initialize tracing, but this is ignorable: ", err)
	}
	defer tracer.cleanUp(ctx, s, cl)

	service := powerpb.NewSuspendPerfServiceClient(cl.Conn)
	tconn := ui.NewTconnServiceClient(cl.Conn)

	// First, turn the display on.
	if _, err := service.TurnOnDisplay(ctx, &emptypb.Empty{}); err != nil {
		s.Fatal("Failed to turn on display: ", err)
	}

	// Prefetch tabs for the same condition.
	if err := mp.OpenCycleTabs(ctx); err != nil {
		s.Fatal("Failed to open tabs for measure performance: ", err)
	}

	// Trace the base metrics.
	tracer.start(ctx, s, cl, false)
	s.Log("Take a metric before suspend as a base metric")
	if err := measureBaseTabSwitching(ctx, tconn, mp, args.enableLacros, pv); err != nil {
		s.Fatal("Failed to measure base tab switching performance: ", err)
	}
	tracer.save(ctx, s, cl, "_base")

	// Get old (before the suspend) histograms if exist. Usually this is empty.
	older, err := getHistograms(ctx, tconn, useMetrics)
	if err != nil {
		s.Fatal("Failed to get Histograms from DUT: ", err)
	}

	prev := older
	seconds := defaultSuspendSeconds

	for i := 0; i < args.numSuspend; i++ {
		tracer.start(ctx, s, cl, true)
		// Suspend and resume
		s.Logf("Suspending DUT for %d seconds", seconds)
		mp.Disconnect(ctx)
		req := powerpb.SuspendRequest{Seconds: int32(seconds)}
		if res, err := service.Suspend(ctx, &req); err != nil {
			if res != nil && res.Failed {
				if res.Output != "" {
					s.Logf("Suspend command failed, the command output is: %s", res.Output)
				}
				s.Fatal("Failed to suspend DUT: ", err)
			}
			s.Log("Ignore suspend command error if connection is lost: ", err)
		}
		s.Log("Resumed")

		// Reconnect because suspend can disconnect network.
		cl, err = redialRPC(ctx, s.DUT(), s.RPCHint(), defaultRedialTimeoutSeconds+seconds)
		if err != nil {
			s.Fatal("Failed to reconnect the RPC: ", err)
		}
		// defer cl.Close() is already set.
		if err := mempressure.ConnectTestEnv(ctx, cl.Conn, args.enableArc, args.enableLacros); err != nil {
			s.Fatal("Failed to re-initalize test environment: ", err)
		}
		tracer.save(ctx, s, cl, fmt.Sprintf("_resumed-%d", i))

		// Reconnect to browser via memory pressure service.
		if err := mp.Reconnect(ctx, cl.Conn); err != nil {
			s.Fatal("Could not recover memory pressure session: ", err)
		}
		// This tab switching metrics are corrected by waitForHistogramsUpdate()
		s.Log("Tab switching after resume")
		tracer.start(ctx, s, cl, false)
		mp.CycleTabs(ctx, defaultCycleTabs)
		tracer.save(ctx, s, cl, fmt.Sprintf("_tabs-%d", i))

		s.Log("Wait for suspend metrics update")
		service = powerpb.NewSuspendPerfServiceClient(cl.Conn)
		tconn = ui.NewTconnServiceClient(cl.Conn)
		prev, err = waitForHistogramsUpdate(ctx, tconn, useMetrics, prev)
		if err != nil {
			s.Fatal("Could not observe histogram update: ", err)
		}
	}
	newer := prev

	// Make differences of Histograms.
	diff, err := histogram.DiffHistograms(older, newer)
	if err != nil {
		s.Fatal("Failed to make difference of histograms: ", err)
	}

	// Write perf metrics from the Diff Histogram and save it.
	writeMetricsFromHistograms(diff, pv)
	if err := pv.Save(s.OutDir()); err != nil {
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

func measureBaseTabSwitching(ctx context.Context, tconn ui.TconnServiceClient, mp *mempressure.RemoteMemoryPressure, lacros bool, pv *perf.Values) error {

	prev, err := metrics.GetHistogram(ctx, tconn, "Browser.Tabs.TotalSwitchDuration3", lacros)
	if err != nil {
		return errors.Wrap(err, "failed to get histogram for cyclic tabs(prev)")
	}
	if err := mp.CycleTabs(ctx, defaultCycleTabs); err != nil {
		return errors.Wrap(err, "failed to do cycle tabs")
	}
	testing.ContextLog(ctx, "Cycke tab switching done")

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		post, err := metrics.GetHistogram(ctx, tconn, "Browser.Tabs.TotalSwitchDuration3", lacros)
		if err != nil {
			return errors.Wrap(err, "failed to get histogram for cyclic tabs(post)")
		}
		diff, err := post.Diff(prev)
		if err != nil {
			return errors.Wrap(err, "failed to make a diff histogram")
		}
		count := diff.TotalCount()
		if count < defaultCycleTabs {
			return errors.New("Some metrics are not counted yet")
		}
		testing.ContextLog(ctx, "Counted Browser.Tabs.TotalSwitchDuration3 is ", count)
		return nil
	}, &testing.PollOptions{
		Timeout:  defaultWaitTimeout,
		Interval: defaultWaitInterval,
	}); err != nil {
		return err
	}

	post, err := metrics.GetHistogram(ctx, tconn, "Browser.Tabs.TotalSwitchDuration3", lacros)
	if err != nil {
		return errors.Wrap(err, "failed to get histogram for cyclic tabs(post)")
	}
	diff, err := post.Diff(prev)
	if err != nil {
		return errors.Wrap(err, "failed to make a diff histogram")
	}
	writeMetricsFromHistogram(diff, "_base", pv)
	return nil
}

type compoundTracers struct {
	instanceName  string
	perfetto      *tracing.RemoteSession
	perfettoToken *tracing.RemoteSessionToken
	perf          *linuxperf.RemoteLinuxPerf
	perfToken     *linuxperf.RemotePerfToken
}

func shouldRunTraceCmd() bool {
	return traceCmdEventsVar.Value() != ""
}

func shouldRunPerfetto() bool {
	return enablePerfettoVar.Value() == "yes"
}

func shouldRunPerf() bool {
	return perfEventVar.Value() != ""
}

func (c *compoundTracers) init(ctx context.Context, cl *rpc.Client) error {
	if !shouldRunTraceCmd() {
		return nil
	}
	// Add a new tracing instance for this test
	// TODO: tune the parameters
	_, err := tracing.NewRemoteInstance(ctx, cl, defaultInstanceName,
		tracing.CPUBufferKiB(defaultBufferSize),
		tracing.InitialStop(),
		tracing.EnableEvents(strings.Split(traceCmdEventsVar.Value(), ",")...))
	if err != nil {
		return errors.Wrap(err, "failed to initialize TraceCmd")
	}
	c.instanceName = defaultInstanceName
	return nil
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
func startPerfetto(ctx context.Context, cl *rpc.Client) (*tracing.RemoteSession, error) {
	if !shouldRunPerfetto() {
		return nil, nil
	}
	sess, err := tracing.StartRemoteSession(ctx, cl, tracing.WithConfigTextData(perfettoConfigData), tracing.InBackground())
	if err != nil {
		return nil, err
	}
	return sess, nil
}

func startLinuxPerf(ctx context.Context, cl *rpc.Client) (*linuxperf.RemoteLinuxPerf, error) {
	if !shouldRunPerf() {
		return nil, nil
	}
	return linuxperf.StartRemoteLinuxPerf(ctx, cl,
		linuxperf.AllCpus(),
		linuxperf.Stacks(),
		linuxperf.Event(perfEventVar.Value()),
		linuxperf.Timeout(100))
}

// start starts the tracers in remote. If `disconnect` is true, the started
// tracing sessions will be tokenized. This means if you run save() method,
// it will reconnect using these token instead of raw session.
// This `disconnect` must be true if you are sure that `cl` will be renewed
// by redial DUT, since the tracing sessions depends on the `cl` rpc.Client.
// (e.g. suspend/resume usually need to redial the DUT)
func (c *compoundTracers) start(ctx context.Context, s *testing.State, cl *rpc.Client, disconnect bool) {
	var err error

	if err := startTraceCmd(ctx, cl); err != nil {
		s.Log("Failed to start trace-cmd, but this is ignorable: ", err)
	}

	c.perfetto, err = startPerfetto(ctx, cl)
	if err != nil {
		s.Log("Failed to start perfetto, but this is ignorable: ", err)
	}
	c.perfettoToken = nil

	c.perf, err = startLinuxPerf(ctx, cl)
	if err != nil {
		s.Fatal("Failed to start perf, but this is ignorable: ", err)
	}
	c.perfToken = nil

	if disconnect {
		// Get tokens from tracers and discard the tracer instances.
		if c.perfetto != nil {
			c.perfettoToken = c.perfetto.Token()
			c.perfetto = nil
		}
		if c.perf != nil {
			c.perfToken = c.perf.Token()
			c.perf = nil
		}
	}
}

// saveTraceCmd fetches the trace data from DUT and save it in s.OutDir().
func saveTraceCmd(ctx context.Context, s *testing.State, cl *rpc.Client, suffix string) error {
	if !shouldRunTraceCmd() {
		return nil
	}
	dest := fmt.Sprintf("%s/trace%s.dat", s.OutDir(), suffix)
	if err := tracing.SaveRemoteInstanceTraceData(ctx, cl, defaultInstanceName,
		func(src string) error {
			return s.DUT().GetFile(ctx, src, dest)
		}); err != nil {
		return errors.Wrap(err, "failed to copy the data file from DUT")
	}
	s.Logf("Save trace data into %q", dest)
	return nil
}

func savePerfetto(ctx context.Context, s *testing.State, cl *rpc.Client, sess *tracing.RemoteSession, tok *tracing.RemoteSessionToken, suffix string) error {
	if !shouldRunPerfetto() {
		return nil
	}
	dest := fmt.Sprintf("%s/perfetto%s.trace", s.OutDir(), suffix)
	if tok == nil {
		defer sess.Finalize(ctx)
		sess.Stop(ctx)
		if err := s.DUT().GetFile(ctx, sess.TraceDataPath(), dest); err != nil {
			return errors.Wrap(err, "failed to copy the perfetto data file from DUT")
		}
	} else if err := tracing.SaveRemoteSessionTraceData(ctx, cl, tok,
		func(src string) error {
			return s.DUT().GetFile(ctx, src, dest)
		}); err != nil {
		return errors.Wrap(err, "failed to copy the perfetto data file from DUT")
	}
	s.Logf("Save perfetto trace data into %q", dest)
	return nil
}

// savePerfData fetches the perf.data from DUT and save it in s.OutDir().
func savePerfData(ctx context.Context, s *testing.State, cl *rpc.Client, perf *linuxperf.RemoteLinuxPerf, tok *linuxperf.RemotePerfToken, suffix string) error {
	if !shouldRunPerf() {
		return nil
	}
	dest := fmt.Sprintf("%s/perf%s.data", s.OutDir(), suffix)
	if perf == nil {
		var err error
		perf, err = linuxperf.ReconnectRemoteLinuxPerf(ctx, cl, tok)
		if err != nil {
			return errors.Wrap(err, "failed to reconnect to perf record")
		}
		tok = nil
	}
	defer func() {
		if err := perf.Finalize(ctx); err != nil {
			s.Log("Failed to run finilize: ", err)
		}
	}()
	if err := perf.Stop(ctx); err != nil {
		return errors.Wrap(err, "failed to stop perf tracing")
	}
	if err := s.DUT().GetFile(ctx, perf.TraceDataPath(), dest); err != nil {
		return errors.Wrap(err, "failed to copy the perfetto data file from DUT")
	}

	s.Logf("Save perf data into %q", dest)
	return nil
}

func (c *compoundTracers) save(ctx context.Context, s *testing.State, cl *rpc.Client, suffix string) {
	if err := saveTraceCmd(ctx, s, cl, suffix); err != nil {
		s.Log("Ignorable: Failed to save trace-cmd data: ", err)
	}
	if err := savePerfetto(ctx, s, cl, c.perfetto, c.perfettoToken, suffix); err != nil {
		s.Log("Ignorable: Failed to save perfetto data: ", err)
	}
	if err := savePerfData(ctx, s, cl, c.perf, c.perfToken, suffix); err != nil {
		s.Log("Ignorable: Failed to save perf data: ", err)
	}
}

func (c *compoundTracers) cleanUp(ctx context.Context, s *testing.State, cl *rpc.Client) error {
	if !shouldRunTraceCmd() {
		return nil
	}
	s.Logf("Cleaning up a trace instance: %s", c.instanceName)
	return tracing.CleanupRemoteInstance(ctx, cl, c.instanceName)
}

func waitForHistogramsUpdate(ctx context.Context, tconn ui.TconnServiceClient, req []*histogramRequest, prev []*histogram.Histogram) ([]*histogram.Histogram, error) {
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		curr, err := getHistograms(ctx, tconn, req)
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

	return getHistograms(ctx, tconn, req)
}

func getHistograms(ctx context.Context, tconn ui.TconnServiceClient, req []*histogramRequest) ([]*histogram.Histogram, error) {
	var hists []*histogram.Histogram
	for _, r := range req {
		hist, err := metrics.GetHistogram(ctx, tconn, r.Name, r.FromLacros)
		if err != nil {
			return nil, err
		}
		hists = append(hists, hist)
	}
	return hists, nil
}

func writeMetricsFromHistograms(hs []*histogram.Histogram, pv *perf.Values) {
	for _, h := range hs {
		writeMetricsFromHistogram(h, "", pv)
	}
}

// writeMetricsFromHistogram writes <histname>_mean, <histname>_p50 (median), <histname>_p100 (max) to @pv
func writeMetricsFromHistogram(hist *histogram.Histogram, suffix string, pv *perf.Values) {
	if hist.TotalCount() == 0 {
		return
	}
	mean, err := hist.Mean()
	if err == nil {
		pv.Set(perf.Metric{
			Name:      fmt.Sprintf("%s%s_mean", hist.Name, suffix),
			Unit:      "ms",
			Direction: perf.SmallerIsBetter,
		}, mean)
	}
	p50, err := hist.Percentile(50)
	if err == nil {
		pv.Set(perf.Metric{
			Name:      fmt.Sprintf("%s%s_p50", hist.Name, suffix),
			Unit:      "ms",
			Direction: perf.SmallerIsBetter,
		}, p50)
	}
	p100, err := hist.Percentile(100)
	if err == nil {
		pv.Set(perf.Metric{
			Name:      fmt.Sprintf("%s%s_p100", hist.Name, suffix),
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
