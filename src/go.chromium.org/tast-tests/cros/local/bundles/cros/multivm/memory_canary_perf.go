// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package multivm

import (
	"context"
	"net/http"
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
		BugComponent: "b:930563",
		Attr:         []string{"group:crosbolt", "crosbolt_nightly"},
		SoftwareDeps: []string{"chrome", "arc"},
		Params: []testing.Param{{
			Pre: multivm.ArcStartedVMMMSTabManagerDelegate(),
			Val: &canaryHealthPerfParam{browser.TypeAsh},
			ExtraData: []string{
				memoryuser.AllocPageFilename,
				memoryuser.JavascriptFilename,
			},
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

func stressCanary(ctx context.Context, fs http.FileSystem, param *canaryHealthPerfParam, allocationMiB int64, allocationPeriod time.Duration, cr *chrome.Chrome, br *browser.Browser, a *arc.ARC, p *perf.Values) error {
	allocationKiB := allocationMiB * 1024
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Minute)
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

	appKills, err := memoryuser.NewArcMemoryKillObserver(ctx, a)
	if err != nil {
		return errors.Wrap(err, "failed to observe ARC kills")
	}
	defer appKills.Close()

	tabDiscards, err := memoryuser.NewTabDiscardObserver(ctx, cr)
	if err != nil {
		return errors.Wrap(err, "failed to observe Chrome tab discards")
	}
	defer tabDiscards.Close()

	vmmmsKills, err := memoryuser.NewVmmmmsKillObserver(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to observer VMMMS kills")
	}
	defer vmmmsKills.Close()

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

	canaryCloser, err := memoryuser.OpenAppTabCanaries(ctx, canaryAllocationMiB, canaryCompressionRatio, br, fs, tconn, a)
	if err != nil {
		return err
	}
	defer canaryCloser(cleanupCtx)

	var allocationTime time.Duration = 0
	var allocationTimeline []allocationTimelineEntry

	testing.ContextLog(ctx, "Starting allocation")
	start := time.Now()
	lastLogTime := start.Add(-11 * time.Second)
	allocationNum := 0
	// Keep allocating until the highest priority app or tab has been seen by all
	// observers.
	for appKills.Foreground == nil || tabDiscards.ProtectedBackground == nil || vmmmsKills.FocusedApp == nil {
		if err := allocationManager.AssertNoDeadAllocator(); err != nil {
			return errors.Wrap(err, "an allocator is killed before the canary")
		}

		availableKB, err := rm.AvailableMemoryKB(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get available memory")
		}

		// Check to see if any observer failed.
		if appKills.Error != nil {
			return errors.Wrap(appKills.Error, "failed while waiting for app kill")
		}
		if tabDiscards.Error != nil {
			return errors.Wrap(tabDiscards.Error, "failed while waiting for tab discard")
		}
		if vmmmsKills.Error != nil {
			return errors.Wrap(vmmmsKills.Error, "failed while waiting for VMMMS kill")
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
			// Update start so that we throttle allocations as if this delay didn't
			// happen.
			start = time.Now().Add(-allocationPeriod * time.Duration(allocationNum))
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
			allocationDelay := time.Until(start.Add(allocationPeriod * time.Duration(allocationNum)))
			// If the target allocation is in the past, we are behind schedule.
			// If we're more than 5 seconds behind schedule, then pause for a second
			// to let the system catch up.
			if allocationDelay < -5*time.Second {
				testing.ContextLogf(ctx, "WARNING: %.2fs behind schedule after %d allocations", -allocationDelay.Seconds(), allocationNum)
				allocationDelay = time.Second
				// Reset start to pretend that we are on schedule after a 1s wait.
				start = time.Now().Add(time.Second - allocationPeriod*time.Duration(allocationNum))
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

	allocationTimeline = append(allocationTimeline, allocationTimelineEntry{
		totalAllocated,
		time.Now(),
	})

	p.Append(perf.Metric{
		Name:      "unthrottledSpeed",
		Unit:      "MiBps",
		Direction: perf.BiggerIsBetter,
		Multiple:  true,
	}, float64(totalAllocated)/allocationTime.Seconds())

	if !appKills.AllPrioritiesObserved() {
		return errors.New("not all app kill priorities observed")
	}

	if !tabDiscards.AllPrioritiesObserved() {
		return errors.New("not all tab discard priorities observed")
	}

	if !vmmmsKills.AllPrioritiesObserved() {
		return errors.New("not all VMMMS kill priorities observed")
	}

	appendAllocatedMetric(p, allocationTimeline, "arc_foreground", appKills.Foreground.Time)
	appendAllocatedMetric(p, allocationTimeline, "arc_perceptible", appKills.Perceptible.Time)
	appendAllocatedMetric(p, allocationTimeline, "arc_cached", appKills.Cached.Time)
	appendAllocatedMetric(p, allocationTimeline, "tab_protected", tabDiscards.ProtectedBackground.Time)
	appendAllocatedMetric(p, allocationTimeline, "tab_background", tabDiscards.Background.Time)

	// TODO(cwd): Figure out how to synchronize guest and host clocks so we don't
	// get negative latencies from Android.
	appendKillLatencyMetric(p, "arc_foreground", appKills.Foreground.Time.Sub(vmmmsKills.PerceptibleApp.Time))
	appendKillLatencyMetric(p, "arc_perceptible", appKills.Perceptible.Time.Sub(vmmmsKills.PerceptibleApp.Time))
	appendKillLatencyMetric(p, "arc_cached", appKills.Cached.Time.Sub(vmmmsKills.CachedApp.Time))
	appendKillLatencyMetric(p, "tab_protected", tabDiscards.ProtectedBackground.Time.Sub(vmmmsKills.PerceptibleTab.Time))
	appendKillLatencyMetric(p, "tab_background", tabDiscards.Background.Time.Sub(vmmmsKills.CachedTab.Time))

	if err := metrics.WriteHistogramMetrics(ctx, tconn, p, umaMetrics); err != nil {
		return err
	}

	// Check that app kills and tab discards happened in the right order.
	// NB: We do this here after observing all tab discards and app killed because
	// we don't want any races between the log parsing in the observers.
	if vmmmsKills.CachedTab.Time.Before(vmmmsKills.CachedApp.Time) {
		return errors.New("background tab discard before cached app kill")
	}
	if vmmmsKills.PerceptibleApp.Time.Before(vmmmsKills.CachedTab.Time) {
		return errors.New("perceptible app kill before background tab discard")
	}
	if vmmmsKills.PerceptibleTab.Time.Before(vmmmsKills.PerceptibleApp.Time) {
		return errors.New("protected background tab discard before perceptible app kill")
	}
	if vmmmsKills.FocusedApp.Time.Before(vmmmsKills.PerceptibleTab.Time) {
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
		if i > 0 {
			s.Log("Sleeping between iterations to allow VMMMS blockers to expire")
			// GoBigSleepLint: Sleep for 10s in between iterations so that the
			// highest priority blockers have a chance to expire.
			// TODO (kalutes): Figure out a good way to clear the blockers in tests
			// without waiting.
			if err := testing.Sleep(ctx, 10*time.Second); err != nil {
				s.Fatal("Failed to sleep between iterations: ", err)
			}

			const psiLowThreshold = 0.1
			s.Logf("Waiting for arc and host psi some avg10 to be below %.2f", psiLowThreshold)
			if err := testing.Poll(ctx, func(ctx context.Context) error {
				psi, err := memory.NewPSIStats(ctx, preARC)
				if err != nil {
					return err
				}
				if psi.Arc.Some.Avg10 > psiLowThreshold || psi.Host.Some.Avg10 > psiLowThreshold {
					s.Logf("psi some avg10 arc=%f host=%f", psi.Arc.Some.Avg10, psi.Host.Some.Avg10)
					return errors.Errorf("psi some avg10 arc=%.2f host=%.2f above threshold=%.2f", psi.Arc.Some.Avg10, psi.Host.Some.Avg10, psiLowThreshold)
				}
				return nil
			}, &testing.PollOptions{
				Interval: 5 * time.Second,
			}); err != nil {
				s.Fatal("Failed to wait for PSI to be low between tests: ", err)
			}
		}
		s.Logf("Starting iteration %d of %d", i+1, iterations)
		if err := stressCanary(ctx, s.DataFileSystem(), param, allocationMiB, allocationPeriod, pre.Chrome, br, preARC, p); err != nil {
			s.Fatal("Error in the canary stress test: ", err)
		}

	}
}
