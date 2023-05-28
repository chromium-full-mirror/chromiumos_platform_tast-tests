// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package multivm

import (
	"context"
	"net/http"
	"os"
	"path"
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
	"go.chromium.org/tast-tests/cros/local/memory/metrics"
	"go.chromium.org/tast-tests/cros/local/multivm"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type canaryHealthPerfParam struct {
	canary           memoryuser.CanaryType
	allocationTarget memoryuser.AllocationTarget
	browserType      browser.Type
}

const iterationsVar = "multivm.MemoryCanaryPerf.iterations"
const throttleVar = "multivm.MemoryCanaryPerf.throttle"

func init() {
	testing.AddTest(&testing.Test{
		Func:         MemoryCanaryPerf,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "How much memory can we allocate before the specified canary dies",
		Contacts: []string{
			"arcvm-memory@google.com",
			"kokiryu@chromium.org",
			"cwd@google.com",
		},
		BugComponent: "b:930563",
		Attr:         []string{"group:crosbolt", "crosbolt_nightly"},
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{{
			Name: "tab_host",
			Pre:  multivm.NoVMStarted(),
			Val:  &canaryHealthPerfParam{memoryuser.Tab, memoryuser.Host, browser.TypeAsh},
			ExtraData: []string{
				memoryuser.AllocPageFilename,
				memoryuser.JavascriptFilename,
			},
		}, {
			Name:              "app_host",
			Pre:               multivm.ArcStarted(),
			Val:               &canaryHealthPerfParam{memoryuser.App, memoryuser.Host, browser.TypeAsh},
			ExtraSoftwareDeps: []string{"android_vm"},
			ExtraData: []string{
				memoryuser.AllocPageFilename,
				memoryuser.JavascriptFilename,
			},
		}, {
			Name:              "app_arc",
			Pre:               multivm.ArcStarted(),
			Val:               &canaryHealthPerfParam{memoryuser.App, memoryuser.Arc, browser.TypeAsh},
			ExtraSoftwareDeps: []string{"android_vm", "lacros"},
		}, {
			Name: "tab_host_lacros",
			Pre:  multivm.NoVMLacrosStarted(),
			Val:  &canaryHealthPerfParam{memoryuser.Tab, memoryuser.Host, browser.TypeLacros},
			ExtraData: []string{
				memoryuser.AllocPageFilename,
				memoryuser.JavascriptFilename,
			},
		}, {
			Name:              "app_host_lacros",
			Pre:               multivm.ArcLacrosStarted(),
			Val:               &canaryHealthPerfParam{memoryuser.App, memoryuser.Host, browser.TypeLacros},
			ExtraSoftwareDeps: []string{"android_vm", "lacros"},
			ExtraData: []string{
				memoryuser.AllocPageFilename,
				memoryuser.JavascriptFilename,
			},
		}, {
			Name:              "app_arc_lacros",
			Pre:               multivm.ArcLacrosStarted(),
			Val:               &canaryHealthPerfParam{memoryuser.App, memoryuser.Arc, browser.TypeLacros},
			ExtraSoftwareDeps: []string{"android_vm", "lacros"},
		}},
		Vars: []string{
			iterationsVar,
			throttleVar,
		},
		Timeout: 30 * time.Minute,
	})
}

const canaryAllocatedMiB = 0
const canaryCompressionRatio = 0.67
const allocatorComplessionRatio = 0.67

func stressCanary(ctx context.Context, fs http.FileSystem, param *canaryHealthPerfParam, allocationMiB int64, allocationPeriod time.Duration, cr *chrome.Chrome, br *browser.Browser, a *arc.ARC) (int64, time.Duration, error) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Minute)
	defer cancel()

	var err error
	var canary memoryuser.Canary
	switch param.canary {
	case memoryuser.Tab:
		canary = memoryuser.NewTabCanary(ctx, canaryAllocatedMiB, canaryCompressionRatio, fs, br, false)
	case memoryuser.App:
		canary, err = memoryuser.NewAppCanary(ctx, canaryAllocatedMiB, canaryCompressionRatio, cr, a)
		if err != nil {
			return -1, -1, errors.Wrap(err, "failed to create the canary")
		}
	default:
		return -1, -1, errors.New("invalid canary type")
	}
	canary.Run(ctx)
	defer canary.Close(cleanupCtx)

	target := param.allocationTarget
	allocationManager := memoryuser.NewMemoryAllocationManager(ctx, target, allocationMiB, allocatorComplessionRatio, a)
	defer allocationManager.Cleanup(cleanupCtx)

	var allocationTime time.Duration = 0
	var allocatedMiB int64 = 0

	start := time.Now()
	allocationNum := 0
	// Keep allocating until the canary dies.
	for {
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
			// If we're more than 5 seconds behind schedule, then pause for a second to let the system catch up.
			if allocationDelay < -5*time.Second {
				testing.ContextLogf(ctx, "WARNING: %.2fs behind schedule after %d allocations", -allocationDelay.Seconds(), allocationNum)
				allocationDelay = time.Second
				// Reset start to pretend that we are on schedule after a 1s wait.
				start = time.Now().Add(time.Second - allocationPeriod*time.Duration(allocationNum))
			}
			if allocationDelay > 0 {
				if err := testing.Sleep(ctx, allocationDelay); err != nil {
					return -1, -1, errors.Wrap(err, "failed to sleep to throttle allocations")
				}
			}
		}

		// Check to see if the test is over.
		if !canary.StillAlive(ctx) {
			testing.ContextLogf(ctx, "%s died after %d MiB allocations", canary.String(), allocationManager.TotalAllocatedMiB())
			allocatedMiB += allocationManager.TotalAllocatedMiB()
			break
		}
		if err := allocationManager.AssertNoDeadAllocator(); err != nil {
			return -1, -1, errors.Wrap(err, "an allocator is killed before the canary")
		}

		// Track the time spent actually allocating as a performance metric.
		allocationStart := time.Now()
		if err := allocationManager.AddAllocator(ctx); err != nil {
			return -1, -1, errors.Wrap(err, "failed to add an allocator")
		}
		allocationTime += time.Since(allocationStart)
		allocationNum++
	}
	return allocatedMiB, allocationTime, nil
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
	allocationSizeMetric := perf.Metric{
		Name:      "allocated",
		Unit:      "MiB",
		Direction: perf.BiggerIsBetter,
		Multiple:  true,
	}
	allocationSpeedMetric := perf.Metric{
		Name:      "unthrottledSpeed",
		Unit:      "MiBps",
		Direction: perf.BiggerIsBetter,
		Multiple:  true,
	}
	memTotalMetric := perf.Metric{
		Name:      "MemTotal",
		Unit:      "MiB",
		Direction: perf.BiggerIsBetter,
	}
	p.Set(memTotalMetric, float64(info.Total)/float64(memory.MiB))

	basemem, err := metrics.NewBaseMemoryStats(ctx, preARC)
	if err != nil {
		s.Fatal("Failed to retrieve base memory stats: ", err)
	}

	for i := 0; i < iterations; i++ {
		mib, time, err := stressCanary(ctx, s.DataFileSystem(), param, allocationMiB, allocationPeriod, pre.Chrome, br, preARC)
		if err != nil {
			s.Fatal("Error in the canary stress test: ", err)
		}
		speed := float64(mib) / time.Seconds()
		s.Logf("Allocation speed: %.00f MiB / s", speed)
		p.Append(allocationSizeMetric, float64(mib))
		p.Append(allocationSpeedMetric, float64(speed))
	}

	memoryStats := perf.NewValues()
	if err := metrics.LogMemoryStats(ctx, basemem, preARC, memoryStats, s.OutDir(), ""); err != nil {
		s.Error("Failed to collect memory metrics: ", err)
	}

	nouploadPath := path.Join(s.OutDir(), "noupload")
	if err := os.Mkdir(nouploadPath, 0777); err != nil {
		s.Error("Failed to create a directory for memory metrics: ", err)
	}
	if err := memoryStats.Save(nouploadPath); err != nil {
		s.Error("Failed to save memory metrics: ", err)
	}

	if err := p.Save(s.OutDir()); err != nil {
		s.Error("Failed to save perf.Values: ", err)
	}
}
