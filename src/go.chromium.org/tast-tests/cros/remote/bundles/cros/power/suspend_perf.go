// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/tast-tests/cros/common/chrome/histogram"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/remote/cros/metrics"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast-tests/cros/remote/memory/mempressure"
	"go.chromium.org/tast-tests/cros/remote/tracing"
	"go.chromium.org/tast-tests/cros/remote/tracing/linuxperf"
	powerpb "go.chromium.org/tast-tests/cros/services/cros/power"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
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
	enableArc         bool
	enableMempressure bool
	enableDisplay     bool
	benchMarkEval     bool
}

const (
	perfettoConfigFile = "perfetto/perfetto_cfg.pbtxt"

	perfettoResumeConfigFile     = "perfetto/perfetto_resume_trace_cfg.pbtxt"
	perfettoDisplayResumeSQLFile = "perfetto/perfetto_display_after_resume.sql"
)

var waiverMaxSystemSuspendMs = testing.RegisterVarString(
	"power.waiverMaxSystemSuspendMs",
	"",
	"Override the maxSystemSuspendMs time",
)

var waiverMaxSystemResumeMs = testing.RegisterVarString(
	"power.waiverMaxSystemResumeMs",
	"",
	"Override the maxSystemResumeMs time",
)

func init() {
	testing.AddTest(&testing.Test{
		Func: SuspendPerf,
		Desc: "Tests that performance of suspend/resume",
		Contacts: []string{
			"cros-suspend-resume@google.com",
			"mhiramat@google.com",
		},
		BugComponent: "b:167279", // ChromeOS > Platform > baseOS > Performance
		Data:         []string{perfettoConfigFile, perfettoResumeConfigFile, perfettoDisplayResumeSQLFile},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(
			// Some meet devices don't support suspend. b/300024874
			hwdep.SkipOnModel("intrepid"),
			hwdep.SkipOnModel("genesis"),
		),
		ServiceDeps: []string{
			"tast.cros.browser.ChromeService",
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
				numSuspend:    5,
				enableDisplay: true,
			},
			ExtraAttr:         []string{"group:crosbolt", "crosbolt_perbuild"},
			ExtraHardwareDeps: hwdep.D(hwdep.Display()),
			// 10 min for setting up (login and opening tabs) +(3 min for each suspend/resume) * 5 times
			Timeout: 30 * time.Minute,
		}, {
			Name: "arc",
			Val: testArgsForSuspendPerf{
				numSuspend:    5,
				enableDisplay: true,
				enableArc:     true,
			},
			ExtraHardwareDeps: hwdep.D(hwdep.Display()),
			Timeout:           30 * time.Minute,
		}, {
			Name: "mem",
			Val: testArgsForSuspendPerf{
				numSuspend:        5,
				enableDisplay:     true,
				enableMempressure: true,
			},
			ExtraHardwareDeps: hwdep.D(hwdep.Display()),
			// mempressure will take another 20minutes
			Timeout: 45 * time.Minute,
		}, {
			Name: "arc_mem",
			Val: testArgsForSuspendPerf{
				numSuspend:        5,
				enableDisplay:     true,
				enableArc:         true,
				enableMempressure: true,
			},
			ExtraHardwareDeps: hwdep.D(hwdep.Display()),
			// mempressure will take another 20minutes
			Timeout: 45 * time.Minute,
		}, {
			Name: "no_display",
			Val: testArgsForSuspendPerf{
				numSuspend: 5,
			},
			// 10 min for setting up (login and opening tabs) +(3 min for each suspend/resume) * 5 times
			Timeout: 30 * time.Minute,
		}, {
			Name:             "fw_qual",
			ExtraTestBedDeps: tbdep.ServoPresentAndWorking,
			ExtraAttr:        []string{"group:firmware", "firmware_bios", "firmware_meets_kpi", "firmware_stressed", "firmware_bios_ro", "firmware_bios_rw", "firmware_ec", "firmware_ec_ro", "firmware_ec_rw"},
			Fixture:          fixture.NormalMode,
			Val: testArgsForSuspendPerf{
				numSuspend:    5,
				benchMarkEval: true,
			},
			Timeout: 30 * time.Minute,
		}},
	})
}

const (
	defaultSuspendSeconds       = 10
	defaultRedialTimeoutSeconds = 60

	defaultInstanceName = "suspend_perf"
	defaultBufferSize   = 10240

	defaultCycleTabs = 10

	// hwClockFile is the path where the timestamp, recorded by the RTC, will be saved to capture when the DUT wakes up from suspend.
	hwClockFile = "/run/power_manager/root/hwclock-on-resume"

	// displayAfterResumeName must specify the column name queried by perfettoDisplayResumeSQLFile.
	displayAfterResumeName = "display_after_resume_ms"
)

type histogramRequest struct {
	// Name specifies the histogram name.
	Name string

	// If Optional is true, waitForHistogramsUpdate() tries to update it but does not wait for update.
	Optional bool
}

// TODO make a new struct type with name and direction.
var defaultMetrics = []*histogramRequest{
	{Name: "Power.KernelSuspendTimeOnAC"},
	{Name: "Power.KernelResumeTimeOnAC"},
	{Name: "Power.DisplayAfterResumeDurationMsOnAC", Optional: true},
	{Name: "Browser.Tabs.TotalSwitchDuration3"},
}

var kernelSuspendMetrics = []*histogramRequest{
	{Name: "Power.KernelSuspendTimeOnAC"},
	{Name: "Power.KernelResumeTimeOnAC"},
}

// Delay and timeout for waitHistogramsUpdate().
var defaultWaitInterval = time.Duration(2) * time.Second
var defaultWaitTimeout = time.Duration(80) * time.Second

var remoteCommandTimeout = time.Duration(3) * time.Second

func SuspendPerf(ctx context.Context, s *testing.State) {
	args := s.Param().(testArgsForSuspendPerf)
	metrics := defaultMetrics
	if !args.enableDisplay {
		// If there is no display, only kernel metrics are available.
		metrics = kernelSuspendMetrics
	}
	forceTabs, err := strconv.Atoi(forceTabsVar.Value())
	if err != nil {
		s.Fatal("Failed to convert ", forceTabsVarName, err)
	}
	pv := perf.NewValues()
	displayAfterResume, err := tracing.NewPerfettoQueryValue(displayAfterResumeName, s.DataPath(perfettoResumeConfigFile), s.DataPath(perfettoDisplayResumeSQLFile))
	if err != nil {
		s.Fatal("Failed to setup "+displayAfterResumeName+" query: ", err)
	}

	var h *firmware.Helper
	if args.benchMarkEval {
		h = s.FixtValue().(*fixture.Value).Helper
		if err := h.RequireServo(ctx); err != nil {
			s.Fatal("Failed to connect to servo: ", err)
		}

		testing.ContextLog(ctx, "Removing USB")
		if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxOff); err != nil {
			s.Fatal("Failed to remove the USB: ", err)
		}

		if err := h.DUT.Conn().CommandContext(ctx, "sudo", "touch", hwClockFile).Run(); err != nil {
			s.Fatal("Failed to create hwClock file: ", err)
		}
		defer func() {
			if err := h.DUT.Conn().CommandContext(ctx, "rm", hwClockFile).Run(); err != nil {
				s.Error("Failed to remove hwClock file: ", err)
			}
		}()

		tlsdatedStatus, err := h.DUT.Conn().CommandContext(ctx, "status", "tlsdated").Output()
		if err != nil {
			s.Fatal("Failed to get the tlsdated status: ", err)
		}
		if strings.Contains(string(tlsdatedStatus), "start") {
			// Stop the tlsdated to use the rtc for the clock.
			if err := h.DUT.Conn().CommandContext(ctx, "stop", "tlsdated").Run(); err != nil {
				s.Fatal("Failed to stop the tlsdated: ", err)
			}
			defer func() {
				if err := h.DUT.Conn().CommandContext(ctx, "start", "tlsdated").Run(); err != nil {
					s.Error("Failed to start the tlsdated: ", err)
				}
			}()
		}

		defer func() {
			if err := h.EnsureDUTBooted(ctx); err != nil {
				s.Fatal("Failed to connect to the DUT: ", err)
			}
		}()
	}

	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer cl.Close(ctx)

	// Login test user for memory pressure and suspend/resume.
	if err := mempressure.NewTestEnv(ctx, cl.Conn, args.enableArc); err != nil {
		s.Fatal("Failed to initalize test environment: ", err)
	}

	// Launch tabs for tab switching time.
	mp, err := mempressure.NewRemoteMemoryPressure(ctx, cl.Conn)
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

	// First, turn the display on if exists.
	if args.enableDisplay {
		if _, err := service.TurnOnDisplay(ctx, &emptypb.Empty{}); err != nil {
			s.Fatal("Failed to turn on display: ", err)
		}
	}

	// Prefetch tabs for the same condition.
	if err := mp.OpenCycleTabs(ctx); err != nil {
		s.Fatal("Failed to open tabs for measure performance: ", err)
	}

	// Trace the base metrics if display exists.
	if args.enableDisplay {
		tracer.start(ctx, s, cl, false)
		s.Log("Take a metric before suspend as a base metric")
		if err := measureBaseTabSwitching(ctx, tconn, mp, pv); err != nil {
			s.Fatal("Failed to measure base tab switching performance: ", err)
		}
		tracer.save(ctx, s, cl, "_base")
	}

	// Get old (before the suspend) histograms if exist. Usually this is empty.
	older, err := getHistograms(ctx, tconn, metrics)
	if err != nil {
		s.Fatal("Failed to get Histograms from DUT: ", err)
	}

	prev := older
	seconds := defaultSuspendSeconds
	expectedSuspendStates := []string{"S0ix", "S3"}
	for i := 0; i < args.numSuspend; i++ {
		tracer.start(ctx, s, cl, true)
		if err := displayAfterResume.StartAndDetach(ctx, cl); err != nil {
			s.Fatal("Failed to start trace for "+displayAfterResumeName+": ", err)
		}
		// Suspend and resume
		s.Logf("Suspending DUT for %d seconds", seconds)
		mp.Disconnect(ctx)

		powerStateCh := make(chan string, 1)
		errCh := make(chan error, 1)
		if args.benchMarkEval {
			verifyPowerStateCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			go verifyPowerState(verifyPowerStateCtx, h, powerStateCh, errCh, expectedSuspendStates)
		}

		req := powerpb.SuspendRequest{Seconds: int32(seconds)}
		res, err := service.Suspend(ctx, &req)
		if err != nil {
			if res != nil && res.Failed {
				if res.Output != "" {
					s.Logf("Suspend command failed, the command output is: %s", res.Output)
				}
				s.Fatal("Failed to suspend DUT: ", err)
			}
			s.Log("Ignore suspend command error if connection is lost: ", err)
		}

		if args.benchMarkEval {
			if err := <-errCh; err != nil {
				s.Fatalf("Failed to get one of %v power state: %s", expectedSuspendStates, err)
			}
			_ = <-powerStateCh

			if res != nil {
				if err := evalSystemResume(ctx, h, res.Output, pv); err != nil {
					s.Errorf("Iteration %d: %v", i+1, err)
				}
				if err := evalSystemSuspend(ctx, s.DUT(), pv); err != nil {
					s.Errorf("Iteration %d: %v", i+1, err)
				}
				displayAfterResume.StopAndQuery(ctx, s.DUT(), cl)
				continue
			}
		}
		s.Log("Resumed")

		// Reconnect because suspend can disconnect network.
		cl, err = redialRPC(ctx, s.DUT(), s.RPCHint(), defaultRedialTimeoutSeconds+seconds)
		if err != nil {
			s.Fatal("Failed to reconnect the RPC: ", err)
		}
		// defer cl.Close() is already set.
		if err := mempressure.ConnectTestEnv(ctx, cl.Conn, args.enableArc); err != nil {
			s.Fatal("Failed to re-initalize test environment: ", err)
		}
		if err := displayAfterResume.StopAndQuery(ctx, s.DUT(), cl); err != nil {
			s.Fatal("Failed to query "+displayAfterResumeName+": ", err)
		}
		tracer.save(ctx, s, cl, fmt.Sprintf("_resumed-%d", i))

		// Reconnect to browser via memory pressure service.
		if err := mp.Reconnect(ctx, cl.Conn); err != nil {
			s.Fatal("Could not recover memory pressure session: ", err)
		}
		// This tab switching metrics are corrected by waitForHistogramsUpdate()
		if args.enableDisplay {
			s.Log("Tab switching after resume")
			tracer.start(ctx, s, cl, false)
			mp.CycleTabs(ctx, defaultCycleTabs)
			tracer.save(ctx, s, cl, fmt.Sprintf("_tabs-%d", i))
		}
		s.Log("Wait for suspend metrics update")
		service = powerpb.NewSuspendPerfServiceClient(cl.Conn)
		tconn = ui.NewTconnServiceClient(cl.Conn)
		prev, err = waitForHistogramsUpdate(ctx, tconn, metrics, prev)
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
	writeMetricsFromQuery(displayAfterResume, pv)
	if err := pv.Save(s.OutDir()); err != nil {
		s.Fatal("Failed saving perf data: ", err)
	}
}

func verifyPowerState(ctx context.Context, h *firmware.Helper, powerStateCh chan string, errCh chan error, expectedSuspendStates []string) {
	var err error
	var currPowerState string
	defer func() {
		errCh <- err
		powerStateCh <- currPowerState
		close(errCh)
		close(powerStateCh)
	}()

	// Try reading the power state from the EC and verify if it includes the expected value.
	err = testing.Poll(ctx, func(c context.Context) error {
		state, err := h.Servo.GetECSystemPowerState(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to check power state")
		}
		if hasPowerState := func(currState string, expectedStates ...string) bool {
			for _, state := range expectedStates {
				if currState == state {
					return true
				}
			}
			return false
		}(state, expectedSuspendStates...); !hasPowerState {
			return errors.Errorf("Power state = %s", state)
		}
		currPowerState = state
		return nil
	}, &testing.PollOptions{Timeout: 15 * time.Second, Interval: 500 * time.Millisecond})
}

func evalSystemSuspend(ctx context.Context, dut *dut.DUT, pv *perf.Values) error {
	const timingsFile = "/run/power_manager/root/last_resume_timings"

	// Wait for last_resume_timings to be created.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		return dut.Conn().CommandContext(ctx, "test", "-f", timingsFile).Run()
	}, &testing.PollOptions{Timeout: 10 * time.Second}); err != nil {
		return errors.Wrapf(err, "failed to wait for %s to be created", timingsFile)
	}

	out, err := dut.Conn().CommandContext(ctx, "cat", timingsFile).Output()
	if err != nil {
		// It's possible the file doesn't exist if suspend failed early.
		return errors.Wrapf(err, "failed to read %s", timingsFile)
	}

	timings := make(map[string]float64)
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return errors.Errorf("invalid line in timings file: %q", line)
		}
		key := parts[0]
		val, err := strconv.ParseFloat(parts[1], 64)
		if err != nil {
			return errors.Wrapf(err, "failed to parse value for %q", key)
		}
		timings[key] = val
	}
	if err := scanner.Err(); err != nil {
		return errors.Wrap(err, "error scanning timings file")
	}

	var systemSuspendTimeMs int64
	if time, ok := timings["suspend_time"]; ok {
		systemSuspendTimeMs = int64(time * 1000)
	} else if start, ok := timings["start_suspend_time"]; ok {
		if end, ok := timings["end_suspend_time"]; ok {
			systemSuspendTimeMs = int64((end - start) * 1000)
		} else {
			return errors.New("found start_suspend_time but not end_suspend_time")
		}
	} else {
		return errors.New("failed to find suspend_time or start/end_suspend_time")
	}

	pv.Append(perf.Metric{
		Name:      "system_suspend",
		Unit:      "ms",
		Direction: perf.SmallerIsBetter,
		Multiple:  true,
	}, float64(systemSuspendTimeMs))
	var maxSystemSuspendMs int64
	maxSystemSuspendMs = 500

	waiver := waiverMaxSystemSuspendMs.Value()
	if waiver != "" {
		val, err := strconv.Atoi(waiver)
		if err != nil {
			return errors.Wrapf(err, "bad flag %s=%q", waiverMaxSystemSuspendMs.Name(), waiver)
		}
		testing.ContextLogf(ctx, "Using user provided value %d for maxSystemSuspendMs", val)
		maxSystemSuspendMs = int64(val)
	}

	if systemSuspendTimeMs > maxSystemSuspendMs {
		return errors.Errorf("failed to suspend device in %d milliseconds, got %d. Use --var=%s=??? to override if you have an approved waiver", maxSystemSuspendMs, systemSuspendTimeMs, waiverMaxSystemSuspendMs.Name())
	}
	testing.ContextLogf(ctx, "Suspend time %d ms within limit of %d ms", systemSuspendTimeMs, maxSystemSuspendMs)

	return nil
}

func convertTimeStamp(timeStr string) (int64, error) {
	t, err := time.Parse(time.RFC3339Nano, timeStr)
	if err != nil {
		return -1, errors.Wrap(err, "failed to parse time")
	}

	return t.UnixMilli(), nil
}

func evalSystemResume(ctx context.Context, h *firmware.Helper, wakeAlarm string, pv *perf.Values) error {
	matchSubString := regexp.MustCompile(`rtc wakealarm: (\d+)`).FindStringSubmatch(wakeAlarm)
	if len(matchSubString) != 2 {
		return errors.Errorf("unexpected wakealarm format, got: %s", wakeAlarm)
	}
	expectedWakeupTime, err := strconv.ParseInt(matchSubString[1], 10, 64)
	if err != nil {
		return errors.Wrap(err, "failed to convert the matched substring to int64 format")
	}

	out, err := h.DUT.Conn().CommandContext(ctx, "cat", hwClockFile).Output()
	if err != nil {
		return errors.Wrap(err, "failed to read hwClock file")
	}
	timeString := strings.Replace(strings.TrimSpace(string(out)), " ", "T", 1)
	actualWakeupTime, err := convertTimeStamp(timeString)
	if err != nil {
		return errors.Wrap(err, "failed to convert time format")
	}

	systemResumeTimeMs := actualWakeupTime - expectedWakeupTime*1000
	pv.Append(perf.Metric{
		Name:      "system_resume",
		Unit:      "ms",
		Direction: perf.SmallerIsBetter,
		Multiple:  true,
	}, float64(systemResumeTimeMs))
	var maxSystemResumeMs int64
	maxSystemResumeMs = 500

	waiver := waiverMaxSystemResumeMs.Value()
	if waiver != "" {
		val, err := strconv.Atoi(waiver)
		if err != nil {
			return errors.Wrapf(err, "bad flag %s=%q", waiverMaxSystemResumeMs.Name(), waiver)
		}
		testing.ContextLogf(ctx, "Using user provided value %d for maxSystemResumeMs", val)
		maxSystemResumeMs = int64(val)
	}

	if systemResumeTimeMs > maxSystemResumeMs {
		return errors.Errorf("failed to resume from suspend state in %d milliseconds, got %d. Use --var=%s=??? to override if you have an approved waiver", maxSystemResumeMs, systemResumeTimeMs, waiverMaxSystemResumeMs.Name())
	}
	testing.ContextLogf(ctx, "Resume time %d ms within limit of %d ms", systemResumeTimeMs, maxSystemResumeMs)

	return nil
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

func measureBaseTabSwitching(ctx context.Context, tconn ui.TconnServiceClient, mp *mempressure.RemoteMemoryPressure, pv *perf.Values) error {

	prev, err := metrics.GetHistogram(ctx, tconn, "Browser.Tabs.TotalSwitchDuration3")
	if err != nil {
		return errors.Wrap(err, "failed to get histogram for cyclic tabs(prev)")
	}
	if err := mp.CycleTabs(ctx, defaultCycleTabs); err != nil {
		return errors.Wrap(err, "failed to do cycle tabs")
	}
	testing.ContextLog(ctx, "Cycke tab switching done")

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		post, err := metrics.GetHistogram(ctx, tconn, "Browser.Tabs.TotalSwitchDuration3")
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

	post, err := metrics.GetHistogram(ctx, tconn, "Browser.Tabs.TotalSwitchDuration3")
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
func startPerfetto(ctx context.Context, cl *rpc.Client, s *testing.State) (*tracing.RemoteSession, error) {
	if !shouldRunPerfetto() {
		return nil, nil
	}
	cfg, err := os.ReadFile(s.DataPath(perfettoConfigFile))
	if err != nil {
		return nil, err
	}
	sess, err := tracing.StartRemoteSession(ctx, cl, tracing.WithConfigTextData(string(cfg)), tracing.InBackground())
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

	c.perfetto, err = startPerfetto(ctx, cl, s)
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

func histogramIsOptional(name string, req []*histogramRequest) bool {
	for _, r := range req {
		if r.Name == name {
			return r.Optional
		}
	}
	return true
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
			if h.TotalCount() == 0 && !histogramIsOptional(h.Name, req) {
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
		hist, err := metrics.GetHistogram(ctx, tconn, r.Name)
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

func writeMetricsFromQuery(q *tracing.PerfettoQueryValue, pv *perf.Values) {
	if len(q.Values) == 0 {
		return
	}
	sort.Float64s(q.Values)

	mean := func(fa []float64) float64 {
		total := 0.0
		for _, f := range fa {
			total += f
		}
		return total / float64(len(fa))
	}(q.Values)
	pv.Set(perf.Metric{
		Name:      fmt.Sprintf("%s_mean", q.Name),
		Unit:      "ms",
		Direction: perf.SmallerIsBetter,
	}, mean)

	p50 := q.Values[int(len(q.Values)/2)]
	pv.Set(perf.Metric{
		Name:      fmt.Sprintf("%s_p50", q.Name),
		Unit:      "ms",
		Direction: perf.SmallerIsBetter,
	}, p50)

	p100 := q.Values[len(q.Values)-1]
	pv.Set(perf.Metric{
		Name:      fmt.Sprintf("%s_p100", q.Name),
		Unit:      "ms",
		Direction: perf.SmallerIsBetter,
	}, p100)
}
