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
	"go.chromium.org/tast-tests/cros/local/memory"
	"go.chromium.org/tast-tests/cros/local/memory/kernelmeter"
	"go.chromium.org/tast-tests/cros/local/memory/memoryuser"
	"go.chromium.org/tast-tests/cros/local/multivm"
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

const canaryAllocatedMiB = 256
const canaryCompressionRatio = 0.67
const allocatorComplessionRatio = 0.67

type allocationTimelineEntry struct {
	allocatedMiB int64
	time         time.Time
}

func appendAllocatedMetric(p *perf.Values, allocationTimeline []allocationTimelineEntry, label string, target time.Time) {
	var prevEntry *allocationTimelineEntry
	for i := range allocationTimeline {
		curEntry := &allocationTimeline[i]
		if prevEntry != nil {
			if prevEntry.time.Before(target) && target.Before(curEntry.time) {
				p.Append(perf.Metric{
					Name:      label,
					Unit:      "MiB",
					Direction: perf.BiggerIsBetter,
					Multiple:  true,
				}, float64(prevEntry.allocatedMiB))
				return
			}
		}
		prevEntry = curEntry
	}
}

func stressCanary(ctx context.Context, fs http.FileSystem, param *canaryHealthPerfParam, allocationMiB int64, allocationPeriod time.Duration, cr *chrome.Chrome, br *browser.Browser, a *arc.ARC, p *perf.Values) error {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Minute)
	defer cancel()

	allocationManager := memoryuser.NewMemoryAllocationManager(ctx, memoryuser.Host, allocationMiB, allocatorComplessionRatio, a)
	defer allocationManager.Cleanup(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create TestConn for opening canaries")
	}
	canaryCloser, err := memoryuser.OpenAppTabCanaries(ctx, canaryAllocatedMiB, canaryCompressionRatio, br, fs, tconn, a)
	if err != nil {
		return err
	}
	defer canaryCloser(cleanupCtx)

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

	var allocationTime time.Duration = 0
	var allocationTimeline []allocationTimelineEntry

	testing.ContextLog(ctx, "Starting allocation")
	start := time.Now()
	lastLogTime := start.Add(-11 * time.Second)
	allocationNum := 0
	// Keep allocating until the canary dies.
	for {
		if err := allocationManager.AssertNoDeadAllocator(); err != nil {
			return errors.Wrap(err, "an allocator is killed before the canary")
		}

		// Track the time spent actually allocating as a performance metric.
		allocationStart := time.Now()
		if err := allocationManager.AddAllocator(ctx); err != nil {
			return errors.Wrap(err, "failed to add an allocator")
		}
		allocationTime += time.Since(allocationStart)
		allocationNum++
		allocationTimeline = append(allocationTimeline, allocationTimelineEntry{
			allocationMiB * int64(allocationNum),
			time.Now(),
		})

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

		// Check to see if the test is over.
		if appKills.Error != nil {
			return errors.Wrap(appKills.Error, "failed while waiting for app kill")
		}
		if tabDiscards.Error != nil {
			return errors.Wrap(tabDiscards.Error, "failed while waiting for tab discard")
		}

		if appKills.Foreground != nil && appKills.Perceptible != nil && appKills.Cached != nil && tabDiscards.ProtectedBackground != nil && tabDiscards.Background != nil {
			break
		}
	}
	totalAllocated := allocationManager.TotalAllocatedMiB()
	testing.ContextLogf(ctx, "Canary died after %d MiB allocations", totalAllocated)

	p.Append(perf.Metric{
		Name:      "unthrottledSpeed",
		Unit:      "MiBps",
		Direction: perf.BiggerIsBetter,
		Multiple:  true,
	}, float64(totalAllocated)/allocationTime.Seconds())

	if appKills.Foreground != nil {
		appendAllocatedMetric(p, allocationTimeline, "arc_foreground", appKills.Foreground.Time)
	}
	if appKills.Perceptible != nil {
		appendAllocatedMetric(p, allocationTimeline, "arc_perceptible", appKills.Perceptible.Time)
	}
	if appKills.Cached != nil {
		appendAllocatedMetric(p, allocationTimeline, "arc_cached", appKills.Cached.Time)
	}
	if tabDiscards.ProtectedBackground != nil {
		appendAllocatedMetric(p, allocationTimeline, "tab_protected", tabDiscards.ProtectedBackground.Time)
	}
	if tabDiscards.Background != nil {
		appendAllocatedMetric(p, allocationTimeline, "tab_background", tabDiscards.Background.Time)
	}

	// Check that app kills and tab discards happened in the right order.
	// NB: We do this here after observing all tab discards and app killed because
	// we don't want any races between the log parsing in the observers.
	if appKills.Perceptible.Time.Before(tabDiscards.Background.Time) {
		return errors.New("perceptible app kill before background tab discard")
	}
	if appKills.Perceptible.Time.Before(appKills.Cached.Time) {
		return errors.New("perceptible app kill before cached app kill")
	}
	if tabDiscards.ProtectedBackground.Time.Before(tabDiscards.Background.Time) {
		return errors.New("protected tab discard before background tab discard")
	}
	if appKills.Foreground.Time.Before(tabDiscards.ProtectedBackground.Time) {
		return errors.New("foreground app kill before protected background tab discard")
	}
	// These two aren't necessary if we assume there is no priority inversion
	// within a component, but probably it's best to check.
	if appKills.Foreground.Time.Before(appKills.Perceptible.Time) {
		return errors.New("foreground app kill before perceptible app kill")
	}
	if tabDiscards.ProtectedBackground.Time.Before(appKills.Cached.Time) {
		return errors.New("protected tab discard before backgrond tab discard")
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

	// Default allocation rate is 1% per second
	allocationRate := 0.01
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

	s.Logf("Allocation size: %.3f RAM = %d MiB", allocationFraction, allocationMiB)
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
		if err := stressCanary(ctx, s.DataFileSystem(), param, allocationMiB, allocationPeriod, pre.Chrome, br, preARC, p); err != nil {
			s.Fatal("Error in the canary stress test: ", err)
		}

	}
}
