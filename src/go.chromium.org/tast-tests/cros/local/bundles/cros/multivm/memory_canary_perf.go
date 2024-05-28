// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package multivm

import (
	"context"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/metrics"
	"go.chromium.org/tast-tests/cros/local/memory"
	"go.chromium.org/tast-tests/cros/local/memory/kernelmeter"
	"go.chromium.org/tast-tests/cros/local/memory/memoryuser"
	"go.chromium.org/tast-tests/cros/local/multivm"
	"go.chromium.org/tast-tests/cros/local/resourced"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type canaryHealthPerfParam struct {
	browserType browser.Type
}

const iterationsVar = "multivm.MemoryCanaryPerf.iterations"
const throttleVar = "multivm.MemoryCanaryPerf.throttle"

func init() {
	testing.AddTest(&testing.Test{
		Func:         MemoryCanaryPerf,
		LacrosStatus: testing.LacrosVariantNeeded,
		Desc:         "How much memory can we allocate before the specified canary dies",
		Contacts: []string{
			"arcvm-memory@google.com",
			"cwd@google.com",
		},
		// ChromeOS > Platform > baseOS > Virtualization > ARC++ & ARCVM
		BugComponent: "b:882467",
		Attr:         []string{"group:crosbolt", "crosbolt_nightly"},
		SoftwareDeps: []string{"chrome", "android_vm"},
		Params: []testing.Param{{
			Pre: multivm.ArcStartedVMMMSTabManagerDelegate(),
			Val: &canaryHealthPerfParam{browser.TypeAsh},
		}},
		Vars: []string{
			iterationsVar,
			throttleVar,
		},
		Timeout: 30 * time.Minute,
	})
}

const canaryAllocationMiB = 256
const canaryAllocationKiB = canaryAllocationMiB * 1024
const canaryCompressionRatio = 0.67
const allocatorComplessionRatio = 0.67

// allocationTimelineEntry tracks the amount of memory allocated before a given
// time.
type allocationTimelineEntry struct {
	allocatedMiB int64
	time         time.Time
}

func allocatedMiBAtTime(allocationTimeline []allocationTimelineEntry, t time.Time) int64 {
	if len(allocationTimeline) == 0 {
		return 0.0
	}

	for _, a := range allocationTimeline {
		if !t.After(a.time) {
			return a.allocatedMiB
		}
	}

	return allocationTimeline[len(allocationTimeline)-1].allocatedMiB
}

func appendAllocatedMetric(p *perf.Values, allocationTimeline []allocationTimelineEntry, label string, target time.Time) {
	p.Append(perf.Metric{
		Name:      label,
		Unit:      "MiB",
		Direction: perf.BiggerIsBetter,
		Multiple:  true,
	}, float64(allocatedMiBAtTime(allocationTimeline, target)))
}

func appendKillLatencyMetric(p *perf.Values, label string, latency time.Duration) {
	p.Append(perf.Metric{
		Name:      label + "_latency",
		Unit:      "s",
		Direction: perf.SmallerIsBetter,
		Multiple:  true,
	}, latency.Seconds())
}

func stressCanary(ctx context.Context, param *canaryHealthPerfParam, allocationMiB int64, allocationPeriod time.Duration, cr *chrome.Chrome, br *browser.Browser, a *arc.ARC, p *perf.Values) error {
	allocationKiB := allocationMiB * 1024
	// Context used by cleanup actions that are deferred.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Minute)
	defer cancel()
	// Context used by measurement code after the main test completes, but before
	// we return. Separate so we can get more detailed logs even if the test times
	// out.
	measureCtx := ctx
	ctx, cancel = ctxutil.Shorten(ctx, time.Minute)
	defer cancel()

	rm, err := resourced.NewClient(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to connect to Resource Manager")
	}
	margins, err := rm.MemoryMarginsKB(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get memory margins")
	}

	allocationManager := memoryuser.NewMemoryAllocationManager(ctx, memoryuser.Host, allocationMiB, allocatorComplessionRatio, a)
	defer allocationManager.Cleanup(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create TestConn for opening canaries")
	}

	umaMetrics := []*metrics.HistogramMetrics{
		metrics.NewHistogramMetrics(
			"Memory.LowMemoryKiller.FirstKillLatency",
			metrics.AppendMeanHistogramMetricWriter("", "ms", perf.SmallerIsBetter),
		),
		metrics.NewHistogramMetrics(
			"Discarding.ReclaimTargetAge",
			metrics.AppendMeanHistogramMetricWriter("", "ms", perf.SmallerIsBetter),
		),
		metrics.NewHistogramMetrics(
			"Discarding.UnnecessaryDiscards",
			metrics.AppendMeanHistogramMetricWriter("", "tabs", perf.SmallerIsBetter),
		),
	}
	if metrics.StartHistogramMetrics(ctx, tconn, umaMetrics); err != nil {
		return errors.Wrap(err, "failed to Start UMA metrics")
	}

	lmkdKillsParser, err := memoryuser.NewLmkdKillsParser(ctx, a)
	if err != nil {
		return errors.Wrap(err, "failed to create LmkdKillsParser")
	}

	canaryCloser, canaryStillAlive, err := memoryuser.OpenAppTabCanaries(ctx, canaryAllocationMiB, canaryCompressionRatio, br, tconn, a)
	if err != nil {
		return err
	}
	defer canaryCloser(cleanupCtx)

	var allocationTime time.Duration = 0
	var allocationTimeline []allocationTimelineEntry

	testing.ContextLog(ctx, "Starting allocation")
	start := time.Now()
	if err != nil {
		return errors.Wrap(err, "failed to get logcat timestamp for start of run")
	}
	throttleStart := start
	lastLogTime := start.Add(-11 * time.Second)
	allocationNum := 0
	// Keep allocating until the highest priority app or tab has been seen by all
	// observers.
	for canaryStillAlive(ctx) {
		if err := allocationManager.AssertNoDeadAllocator(); err != nil {
			return errors.Wrap(err, "an allocator is killed before the canary")
		}

		availableKB, err := rm.AvailableMemoryKB(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get available memory")
		}

		// Don't allocate unless after this allocation we would still be less than
		// one half of a canary size below the ChromeOS critical margin. We don't
		// want Chrome discarding two canaries at once.
		aboveCriticalKiB := int64(availableKB) - int64(margins.CriticalKB)
		if aboveCriticalKiB-allocationKiB < -canaryAllocationKiB/2 {
			testing.ContextLogf(ctx, "ChromeOS critical margin breached by %d kiB, sleeping", -aboveCriticalKiB)
			// GoBigSleepLint: Sleep until we are not below the critical margin.
			if err := testing.Sleep(ctx, time.Second); err != nil {
				return errors.Wrap(err, "failed to sleep to throttle allocations")
			}
			// Update throttleStart so that we throttle allocations as if this delay
			// didn't happen.
			throttleStart = time.Now().Add(-allocationPeriod * time.Duration(allocationNum))
			continue
		}

		// Track the time spent actually allocating as a performance metric.
		allocationStart := time.Now()
		if err := allocationManager.AddAllocator(ctx); err != nil {
			return errors.Wrap(err, "failed to add an allocator")
		}
		allocationTime += time.Since(allocationStart)
		// The previous allocation timeline entry ended when this allocation
		// started.
		allocationTimeline = append(allocationTimeline, allocationTimelineEntry{
			allocationMiB * int64(allocationNum),
			allocationStart,
		})
		allocationNum++

		if time.Since(lastLogTime) > 10*time.Second {
			testing.ContextLogf(ctx, "Allocated %d MiB", allocationManager.TotalAllocatedMiB())
			lastLogTime = time.Now()
		}

		// Throttle allocations if we have a target time period between
		// allocations.
		// NB: We compute allocationDelay based on the start time because it allows
		// us to catch up to the target if we get behind for a bit. The
		// canary.StillAlive check sometimes takes a few seconds, so it's best to
		// not delay the test unless we are consistently behind the target. Hence
		// the 5s threshold before we add a pause.
		if allocationPeriod > 0 {
			// Compute when the next allocation should happen.
			allocationDelay := time.Until(throttleStart.Add(allocationPeriod * time.Duration(allocationNum)))
			// If the target allocation is in the past, we are behind schedule.
			// If we're more than 5 seconds behind schedule, then pause for a second
			// to let the system catch up.
			if allocationDelay < -5*time.Second {
				testing.ContextLogf(ctx, "WARNING: %.2fs behind schedule after %d allocations", -allocationDelay.Seconds(), allocationNum)
				allocationDelay = time.Second
				// Reset start to pretend that we are on schedule after a 1s wait.
				throttleStart = time.Now().Add(time.Second - allocationPeriod*time.Duration(allocationNum))
			}
			if allocationDelay > 0 {
				// GoBigSleepLint: Sleep until the next allocation is ready
				if err := testing.Sleep(ctx, allocationDelay); err != nil {
					return errors.Wrap(err, "failed to sleep to throttle allocations")
				}
			}
		}
	}
	totalAllocated := allocationManager.TotalAllocatedMiB()
	testing.ContextLogf(ctx, "Canary died after %d MiB allocations", totalAllocated)

	stop := time.Now()
	allocationTimeline = append(allocationTimeline, allocationTimelineEntry{
		totalAllocated,
		stop,
	})

	if err := allocationManager.RemoveAllAllocators(); err != nil {
		return errors.Wrap(err, "failed to free all memory after test")
	}

	testing.ContextLog(ctx, "Sleeping after test to allow system to settle")
	// GoBigSleepLint: Sleep for 10s in between iterations so that the
	// highest priority blockers have a chance to expire.
	// TODO (kalutes): Figure out a good way to clear the blockers in tests
	// without waiting.
	if err := testing.Sleep(ctx, 10*time.Second); err != nil {
		return errors.Wrap(err, "failed to sleep after test")
	}

	// We wait for host and guest PSI to settle before scanning logs, so that
	// memory pressure is low when compute metrics. This makes things a lot less
	// flaky.
	const psiLowThreshold = 0.1
	testing.ContextLogf(ctx, "Waiting for arc and host psi some avg10 to be below %.2f", psiLowThreshold)
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		psi, err := memory.NewPSIStats(ctx, a)
		if err != nil {
			return err
		}
		if psi.Host == nil {
			return errors.New("no PSI results from host")
		}
		if psi.Arc == nil {
			return errors.New("no PSI results from ARC")
		}
		if psi.Arc.Some.Avg10 > psiLowThreshold || psi.Host.Some.Avg10 > psiLowThreshold {
			testing.ContextLogf(ctx, "psi some avg10 arc=%f host=%f", psi.Arc.Some.Avg10, psi.Host.Some.Avg10)
			return errors.Errorf("psi some avg10 arc=%.2f host=%.2f above threshold=%.2f", psi.Arc.Some.Avg10, psi.Host.Some.Avg10, psiLowThreshold)
		}
		return nil
	}, &testing.PollOptions{
		Interval: 5 * time.Second,
	}); err != nil {
		return errors.Wrap(err, "failed to wait for PSI to be low between tests")
	}

	vmmmsLog, err := memoryuser.ParseVmmmsKills(measureCtx, start, stop)
	if err != nil {
		return errors.Wrap(err, "failed to parse VMMMS logs")
	}

	lmkdLog, err := lmkdKillsParser.Parse(ctx, a)
	if err != nil {
		return errors.Wrap(err, "failed to collect LMKD logs")
	}

	discardLog, err := memoryuser.ParseTabDiscards(measureCtx, cr, start, stop)
	if err != nil {
		return errors.Wrap(err, "failed to parse Chrome tab discard logs")
	}

	p.Append(perf.Metric{
		Name:      "unthrottledSpeed",
		Unit:      "MiBps",
		Direction: perf.BiggerIsBetter,
		Multiple:  true,
	}, float64(totalAllocated)/allocationTime.Seconds())

	if !memoryuser.AllVmmmsPrioritiesLogged(vmmmsLog) {
		memoryuser.DumpVmmmsKillLog(measureCtx, vmmmsLog)
		return errors.New("not all VMMMS kill priorities observed, see log above for full list of observed priorities")
	}

	// AllVmmmsPrioritiesLogged above ensures these are non-nil
	vmmmsFocusedApp := memoryuser.FirstVmmmsKillOfPriority(vmmmsLog, memoryuser.VmmmsFocusedAppPriority)
	vmmmsPerceptibleApp := memoryuser.FirstVmmmsKillOfPriority(vmmmsLog, memoryuser.VmmmsPerceptibleAppPriority)
	vmmmsCachedApp := memoryuser.FirstVmmmsKillOfPriority(vmmmsLog, memoryuser.VmmmsCachedAppPriority)
	vmmmsPerceptibleTab := memoryuser.FirstVmmmsKillOfPriority(vmmmsLog, memoryuser.VmmmsPerceptibleTabPriority)
	vmmmsCachedTab := memoryuser.FirstVmmmsKillOfPriority(vmmmsLog, memoryuser.VmmmsCachedTabPriority)

	appendAllocatedMetric(p, allocationTimeline, "arc_foreground", vmmmsFocusedApp.Time)
	appendAllocatedMetric(p, allocationTimeline, "arc_perceptible", vmmmsPerceptibleApp.Time)
	appendAllocatedMetric(p, allocationTimeline, "arc_cached", vmmmsCachedApp.Time)
	appendAllocatedMetric(p, allocationTimeline, "tab_protected", vmmmsPerceptibleTab.Time)
	appendAllocatedMetric(p, allocationTimeline, "tab_background", vmmmsCachedTab.Time)

	lmkdFocusedApp := memoryuser.FirstLmkdKillOfPriority(lmkdLog, memoryuser.VmmmsFocusedAppPriority)
	lmkdPerceptibleApp := memoryuser.FirstLmkdKillOfPriority(lmkdLog, memoryuser.VmmmsPerceptibleAppPriority)
	lmkdCachedApp := memoryuser.FirstLmkdKillOfPriority(lmkdLog, memoryuser.VmmmsCachedAppPriority)
	discardPerceptibleTab := memoryuser.FirstTabDiscardOfPriority(discardLog, memoryuser.VmmmsPerceptibleTabPriority)
	discardCachedTab := memoryuser.FirstTabDiscardOfPriority(discardLog, memoryuser.VmmmsCachedTabPriority)

	// // TODO(cwd): Figure out how to synchronize guest and host clocks so we don't
	// // get negative latencies from Android.
	if lmkdFocusedApp != nil {
		appendKillLatencyMetric(p, "arc_foreground", lmkdFocusedApp.Time.Sub(vmmmsFocusedApp.Time))
	} else {
		return errors.New("no LMKD focused app kill in logcat")
	}
	if lmkdPerceptibleApp != nil {
		appendKillLatencyMetric(p, "arc_perceptible", lmkdPerceptibleApp.Time.Sub(vmmmsPerceptibleApp.Time))
	} else {
		return errors.New("no LMKD perceptible app kill in logcat")
	}
	if lmkdCachedApp != nil {
		appendKillLatencyMetric(p, "arc_cached", lmkdCachedApp.Time.Sub(vmmmsCachedApp.Time))
	} else {
		return errors.New("no LMKD cached app kill in logcat")
	}
	if discardPerceptibleTab != nil {
		appendKillLatencyMetric(p, "tab_protected", discardPerceptibleTab.Time.Sub(vmmmsPerceptibleTab.Time))
	} else {
		return errors.New("no protected background tab discard in Chrome logs")
	}
	if discardCachedTab != nil {
		appendKillLatencyMetric(p, "tab_background", discardCachedTab.Time.Sub(vmmmsCachedTab.Time))
	} else {
		return errors.New("no background tab discard in Chrome logs")
	}

	if err := metrics.WriteHistogramMetrics(measureCtx, tconn, p, umaMetrics); err != nil {
		return err
	}

	// Check that app kills and tab discards happened in the right order.
	if vmmmsCachedTab.Time.Before(vmmmsCachedApp.Time) {
		memoryuser.DumpVmmmsKillLog(measureCtx, vmmmsLog)
		return errors.New("background tab discard before cached app kill")
	}
	if vmmmsPerceptibleApp.Time.Before(vmmmsCachedTab.Time) {
		memoryuser.DumpVmmmsKillLog(measureCtx, vmmmsLog)
		return errors.New("perceptible app kill before background tab discard")
	}
	if vmmmsPerceptibleTab.Time.Before(vmmmsPerceptibleApp.Time) {
		memoryuser.DumpVmmmsKillLog(measureCtx, vmmmsLog)
		return errors.New("protected background tab discard before perceptible app kill")
	}
	if vmmmsFocusedApp.Time.Before(vmmmsPerceptibleTab.Time) {
		memoryuser.DumpVmmmsKillLog(measureCtx, vmmmsLog)
		return errors.New("focused app kill before protected background tab discard")
	}

	return nil
}

func MemoryCanaryPerf(ctx context.Context, s *testing.State) {
	pre := s.PreValue().(*multivm.PreData)
	param := s.Param().(*canaryHealthPerfParam)
	preARC := multivm.ARCFromPre(pre)
	br, cleanupBr, err := browserfixt.SetUp(ctx, pre.Chrome.Chrome(), param.browserType)
	if err != nil {
		s.Fatal("Failed to get Browser: ", err)
	}
	defer cleanupBr(ctx)

	info, err := kernelmeter.MemInfo()
	if err != nil {
		s.Fatal("Failed to get meminfo for RAM size: ", err)
	}

	iterationsStr, ok := s.Var(iterationsVar)
	var iterations int
	if ok {
		iterationsConv, err := strconv.Atoi(iterationsStr)
		if err != nil {
			s.Fatalf("Could not convert var %s := %q to integer: %s", iterationsVar, iterationsStr, err)
		}
		iterations = iterationsConv
	} else {
		iterations = 5
	}

	// Each allocation is for 0.5% of RAM.
	const allocationFraction = 0.005
	allocationMiB := int64(allocationFraction * float64(info.Total) / float64(memory.MiB))

	if allocationMiB*3 > canaryAllocationMiB {
		// Limit the allocation size to 1/3 of the canary size so that we can't
		// breach the ChromeOS critical margin by too much with a single
		// allocation. We don't want Chrome to kill two canaries at once.
		allocationMiB = canaryAllocationMiB / 3
	}

	// Default allocation rate is 2% per second
	allocationRate := 0.02
	throttleStr, ok := s.Var(throttleVar)
	if ok {
		parsedAllocationRate, err := strconv.ParseFloat(throttleStr, 64)
		if err != nil {
			s.Fatalf("Could not convert var %s := %q to float: %s", throttleVar, throttleStr, err)
		}
		if parsedAllocationRate < 0 || parsedAllocationRate >= 0.1 {
			s.Fatalf("Var %s := %q must be in the range [0, 0.1]", throttleVar, throttleStr)
		}
		allocationRate = parsedAllocationRate
	}

	s.Logf("Allocation size: %d MiB", allocationMiB)
	allocationPeriod := time.Duration(0)
	if allocationRate > 0 {
		allocationPeriod = time.Duration(float64(time.Second) * allocationFraction / allocationRate)
		s.Logf("Allocation rate: %.3f RAM/s = %d MiB / %.3f s = %.f MiB/s", allocationRate, allocationMiB, allocationPeriod.Seconds(), float64(allocationMiB)/allocationPeriod.Seconds())
	} else {
		s.Log("Allocation rate not throttled")
	}

	p := perf.NewValues()
	// Save perf.Values even on failure.
	defer func() {
		if err := p.Save(s.OutDir()); err != nil {
			s.Error("Failed to save perf.Values: ", err)
		}
	}()

	p.Set(perf.Metric{
		Name:      "MemTotal",
		Unit:      "MiB",
		Direction: perf.BiggerIsBetter,
	}, float64(info.Total)/float64(memory.MiB))

	for i := 0; i < iterations; i++ {
		s.Logf("Starting iteration %d of %d", i+1, iterations)
		if err := stressCanary(ctx, param, allocationMiB, allocationPeriod, pre.Chrome, br, preARC, p); err != nil {
			s.Fatal("Error in the canary stress test: ", err)
		}
	}
}
