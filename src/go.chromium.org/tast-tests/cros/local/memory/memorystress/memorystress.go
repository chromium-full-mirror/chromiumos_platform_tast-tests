// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package memorystress opens synthetic pages to create memory pressure.
package memorystress

import (
	"context"
	"math/rand"
	"time"

	"go.chromium.org/tast-tests/cros/common/chrome/histogram"
	"go.chromium.org/tast-tests/cros/common/memory"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/metrics"
	"go.chromium.org/tast-tests/cros/local/memory/kernelmeter"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// TestCaseResult is the result of a stress test case.
type TestCaseResult struct {
	// jankyCount is the average janky count in 30 seconds histogram.
	jankyCount *histogram.Histogram
	// discardLatency is the discard latency histogram.
	discardLatency *histogram.Histogram
	// reloadCount is the tab reload count.
	reloadCount uint64
	// oomCount is the oom kill count.
	oomCount uint64
}

// activeTabURL returns the URL of the active tab.
func activeTabURL(ctx context.Context, br *browser.Browser) (string, error) {
	tconn, err := br.TestAPIConn(ctx)
	if err != nil {
		return "", errors.Wrap(err, "cannot create test connection")
	}

	var tabURL string
	if err := tconn.Call(ctx, &tabURL, `async () => {
                let tabs = await tast.promisify(chrome.tabs.query)({active: true});
                return tabs[0].url;
        }`); err != nil {
		return "", errors.Wrap(err, "active tab URL not found")
	}
	return tabURL, nil
}

// isTargetAvailable checks if there is any matched target.
func isTargetAvailable(ctx context.Context, br *browser.Browser, tm chrome.TargetMatcher) (bool, error) {
	targets, err := br.FindTargets(ctx, tm)
	if err != nil {
		return false, errors.Wrap(err, "failed to get targets")
	}
	return len(targets) != 0, nil
}

// waitAllocation waits for completion of JavaScript memory allocation.
func waitAllocation(ctx context.Context, conn *chrome.Conn) error {
	const timeout = 60 * time.Second
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Waits for completion of JavaScript allocation.
	// Checks completion only for the allocation page memory_stress.html.
	// memory_stress.html saves the allocation result to document.out.
	const expr = "!window.location.href.includes('memory_stress') || document.hasOwnProperty('out') == true"
	if err := conn.WaitForExprFailOnErr(waitCtx, expr); err != nil {
		if waitCtx.Err() == context.DeadlineExceeded {
			// Quiesce timeout is common under memory stress, do not interrupt the test in this case.
			testing.ContextLogf(ctx, "Ignoring tab quiesce timeout (%v)", timeout)
			return nil
		}
		return errors.Wrap(err, "unexpected error waiting for tab quiesce")
	}
	return nil
}

// waitAllocationForURL waits for completion of JavaScript memory allocation on the tab with specified URL.
func waitAllocationForURL(ctx context.Context, br *browser.Browser, url string) error {
	conn, err := br.NewConnForTarget(ctx, chrome.MatchTargetURL(url))
	if err != nil {
		return errors.Wrap(err, "NewConnForTarget failed")
	}
	defer conn.Close()

	return waitAllocation(ctx, conn)
}

// openAllocationPage opens a page to allocate many JavaScript objects.
func openAllocationPage(ctx context.Context, url string, br *browser.Browser) error {
	conn, err := br.NewConn(ctx, url)
	if err != nil {
		return errors.Wrap(err, "cannot create new renderer")
	}
	defer conn.Close()

	return waitAllocation(ctx, conn)
}

// openTabCount returns the count of tabs to open.
func openTabCount(mbPerTab int) (int, error) {
	memPerTab := kernelmeter.NewMemSizeMiB(mbPerTab)
	memInfo, err := kernelmeter.MemInfo()
	if err != nil {
		return 0, errors.Wrap(err, "cannot obtain memory info")
	}
	// Allocates more than total memory and swap space size to trigger low memory.
	return int((memInfo.Total+memInfo.SwapTotal)/memPerTab) + 1, nil
}

// openTabs opens tabs to create memory pressure.
func openTabs(ctx context.Context, br *browser.Browser, createTabCount, mbPerTab int, compressRatio float32) error {
	for i := 0; i < createTabCount; i++ {
		url := memory.CompileMemoryStressDataURL(mbPerTab, compressRatio)
		if err := openAllocationPage(ctx, url, br); err != nil {
			return errors.Wrap(err, "cannot create tab")
		}
	}
	return nil
}

// createWaitTimeList creates the wait time list between switching tabs according to the random generator.
func createWaitTimeList(switchCount int, localRand *rand.Rand) []time.Duration {
	waitTimeList := make([]time.Duration, switchCount)
	waitTime := 3 * time.Second
	for i := 0; i < switchCount; i++ {
		// Waits between tab switches.
		// +/- within 1000ms from the previous wait time to cluster long and short wait times.
		// On some devices, it's easier to trigger OOM with clustered short wait times.
		// On other devices, it's easier with clustered long wait times.
		// It's necessary to make wait time depending on previous wait time.
		// If each wait time is independent, there will be less clusters of long or short wait times.
		// Wait time is in [1, 5] seconds range.
		waitTime += time.Duration(localRand.Intn(2001)-1000) * time.Millisecond
		if waitTime < time.Second {
			waitTime = time.Second
		} else if waitTime > 5*time.Second {
			waitTime = 5 * time.Second
		}
		waitTimeList[i] = waitTime
	}
	return waitTimeList
}

// ReportTestCaseResult writes the test case result to perfValues and prints the test case result.
func ReportTestCaseResult(ctx context.Context, perfValues *perf.Values, result TestCaseResult, label string) error {
	testing.ContextLog(ctx, "===== "+label+" test results =====")

	jankMean, err := result.jankyCount.Mean()
	if err == nil {
		jankyMetric := perf.Metric{
			Name:      "tast_janky_count_" + label,
			Unit:      "count",
			Direction: perf.SmallerIsBetter,
		}
		perfValues.Set(jankyMetric, jankMean)
		testing.ContextLog(ctx, "Average janky count in 30s: ", jankMean)
	} else {
		testing.ContextLog(ctx, "Failed to get mean for tast_janky_count")
	}

	killLatency, err := result.discardLatency.Mean()
	if err == nil {
		killLatencyMetric := perf.Metric{
			Name:      "tast_discard_latency_" + label,
			Unit:      "ms",
			Direction: perf.SmallerIsBetter,
		}
		perfValues.Set(killLatencyMetric, killLatency)
		testing.ContextLog(ctx, "Average discard latency(ms): ", killLatency)
	} else {
		testing.ContextLog(ctx, "Failed to get mean for tast_discard_latency")
	}

	reloadTabMetric := perf.Metric{
		Name:      "tast_reload_tab_count_" + label,
		Unit:      "count",
		Direction: perf.SmallerIsBetter,
	}
	perfValues.Set(reloadTabMetric, float64(result.reloadCount))
	testing.ContextLog(ctx, "Reload tab count: ", result.reloadCount)

	oomKillerMetric := perf.Metric{
		Name:      "tast_oom_killer_count_" + label,
		Unit:      "count",
		Direction: perf.SmallerIsBetter,
	}
	perfValues.Set(oomKillerMetric, float64(result.oomCount))
	testing.ContextLog(ctx, "OOM Kill count: ", result.oomCount)

	return nil
}

// TestCase opens synthetic pages to allocate JavaScript objects to create memory pressure.
func TestCase(ctx context.Context, br *browser.Browser, localRand *rand.Rand, mbPerTab, switchCount int, compressRatio float32) (TestCaseResult, error) {
	vmstatsStart, err := kernelmeter.VMStats()
	if err != nil {
		return TestCaseResult{}, errors.Wrap(err, "failed to get vmstat")
	}

	createTabCount, err := openTabCount(mbPerTab)
	if err != nil {
		return TestCaseResult{}, errors.Wrap(err, "failed to get open tab count")
	}
	testing.ContextLog(ctx, "Tab count to create: ", createTabCount)

	if err := openTabs(ctx, br, createTabCount, mbPerTab, compressRatio); err != nil {
		return TestCaseResult{}, errors.Wrap(err, "failed to open tabs")
	}

	tconn, err := br.TestAPIConn(ctx)
	if err != nil {
		return TestCaseResult{}, errors.Wrap(err, "failed to connect to test API")
	}

	histogramNames := []string{"Browser.Responsiveness.JankyIntervalsPerThirtySeconds", "Memory.LowMemoryKiller.FirstKillLatency"}
	startHistograms, err := metrics.GetHistograms(ctx, tconn, histogramNames)
	if err != nil {
		return TestCaseResult{}, errors.Wrap(err, "failed to get histograms")
	}

	waitTimeList := createWaitTimeList(switchCount, localRand)
	reloadCount, err := SwitchTabsList(ctx, br, waitTimeList)
	if err != nil {
		return TestCaseResult{}, errors.Wrap(err, "failed to switch tabs")
	}

	endHistograms, err := metrics.GetHistograms(ctx, tconn, histogramNames)
	if err != nil {
		return TestCaseResult{}, errors.Wrap(err, "failed to get histograms")
	}
	histograms, err := histogram.DiffHistograms(startHistograms, endHistograms)
	if err != nil {
		return TestCaseResult{}, errors.Wrap(err, "failed to diff histograms")
	}
	jankyCount := histograms[0]
	discardLatency := histograms[1]

	vmstatsEnd, err := kernelmeter.VMStats()
	if err != nil {
		return TestCaseResult{}, errors.Wrap(err, "failed to get vmstat")
	}
	oomCount := vmstatsEnd["oom_kill"] - vmstatsStart["oom_kill"]

	return TestCaseResult{
		reloadCount:    reloadCount,
		jankyCount:     jankyCount,
		discardLatency: discardLatency,
		oomCount:       oomCount,
	}, nil
}
