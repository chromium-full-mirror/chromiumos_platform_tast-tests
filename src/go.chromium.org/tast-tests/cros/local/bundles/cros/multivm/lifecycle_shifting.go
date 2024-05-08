// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package multivm

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	chromeMetrics "go.chromium.org/tast-tests/cros/local/chrome/metrics"
	"go.chromium.org/tast-tests/cros/local/crostini"
	"go.chromium.org/tast-tests/cros/local/memory"
	"go.chromium.org/tast-tests/cros/local/memory/kernelmeter"
	"go.chromium.org/tast-tests/cros/local/memory/memoryuser"
	"go.chromium.org/tast-tests/cros/local/memory/metrics"
	"go.chromium.org/tast-tests/cros/local/multivm"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type lifecycleShiftingParam struct {
	inHost, inCrostini, inARC bool
	browserType               browser.Type
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         LifecycleShifting,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Create many Apps, Tabs, Processes in turn across multiple VMs, and see how many can stay alive",
		Contacts: []string{
			"arcvm-memory@google.com",
			"cwd@google.com",
		},
		BugComponent: "b:930563",
		Attr:         []string{"group:crosbolt", "crosbolt_nightly"},
		Timeout:      30 * time.Minute,
		SoftwareDeps: []string{"chrome"},
		Vars:         []string{"multivm.minimize_arc"},
		Params: []testing.Param{{
			Name:              "arc_host",
			Pre:               multivm.ArcStarted(),
			Val:               &lifecycleShiftingParam{inARC: true, inHost: true, browserType: browser.TypeAsh},
			ExtraSoftwareDeps: []string{"android_vm"},
			// Arc lifecycle unit allocates on the Java heap which is limited to 512MiB.
			// 2% * 24GiB = 492MiB, which gives room for the rest of the app.
			ExtraHardwareDeps: hwdep.D(hwdep.MaxMemory(24 * 1024)),
		}, {
			Name:              "arc_host_lacros",
			Pre:               multivm.ArcLacrosStarted(),
			Val:               &lifecycleShiftingParam{inARC: true, inHost: true, browserType: browser.TypeLacros},
			ExtraSoftwareDeps: []string{"android_vm", "lacros"},
			ExtraHardwareDeps: hwdep.D(hwdep.MaxMemory(24 * 1024)),
		}, {
			Name:              "arc_host_vmmms",
			Pre:               multivm.ArcStartedVMMMS(),
			Val:               &lifecycleShiftingParam{inARC: true, inHost: true, browserType: browser.TypeAsh},
			ExtraSoftwareDeps: []string{"android_vm"},
			ExtraHardwareDeps: hwdep.D(hwdep.MaxMemory(24 * 1024)),
		}, {
			Name:              "arc_host_lacros_vmmms",
			Pre:               multivm.ArcLacrosStartedVMMMS(),
			Val:               &lifecycleShiftingParam{inARC: true, inHost: true, browserType: browser.TypeLacros},
			ExtraSoftwareDeps: []string{"android_vm", "lacros"},
			ExtraHardwareDeps: hwdep.D(hwdep.MaxMemory(24 * 1024)),
		}, {
			Name:              "crostini_host",
			Pre:               multivm.CrostiniStarted(),
			Val:               &lifecycleShiftingParam{inCrostini: true, inHost: true, browserType: browser.TypeAsh},
			ExtraSoftwareDeps: []string{"android_vm"},
			ExtraData: []string{
				crostini.GetContainerMetadataArtifact("bullseye", false),
				crostini.GetContainerRootfsArtifact("bullseye", false),
			},
			ExtraHardwareDeps: crostini.CrostiniStable,
		}},
	})
}

func LifecycleShifting(ctx context.Context, s *testing.State) {
	pre := s.PreValue().(*multivm.PreData)
	param := s.Param().(*lifecycleShiftingParam)
	preARC := multivm.ARCFromPre(pre)
	preCrostini := multivm.CrostiniFromPre(pre)

	minimizeArc := false
	minimizeArcString, ok := s.Var("multivm.minimize_arc")
	if ok {
		if minimize, err := strconv.ParseBool(minimizeArcString); err == nil {
			minimizeArc = minimize
		}
	}

	tconn, err := pre.Chrome.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Could not get Chrome test API connection: ", err)
	}
	killsBefore, err := arc.GetAppKills(ctx, tconn)
	if err != nil {
		s.Log("Failed to collect ARC app kill counts: ", err)
	}

	info, err := kernelmeter.MemInfo()
	if err != nil {
		s.Fatal("Failed to get /proc/meminfo: ", err)
	}
	basemem, err := metrics.NewBaseMemoryStats(ctx, preARC)
	if err != nil {
		s.Fatal("Failed to retrieve base memory stats: ", err)
	}

	umaMetrics := []*chromeMetrics.HistogramMetrics{
		chromeMetrics.NewHistogramMetrics(
			"Memory.LowMemoryKiller.FirstKillLatency",
			chromeMetrics.SetMeanHistogramMetricWriter("", "ms", perf.SmallerIsBetter),
			chromeMetrics.SetPercentileHistogramMetricWriter(50, "_p50", "ms", perf.SmallerIsBetter),
			chromeMetrics.SetPercentileHistogramMetricWriter(90, "_p90", "ms", perf.SmallerIsBetter),
			chromeMetrics.SetCountHistogramMetricWriter("_count", perf.SmallerIsBetter),
		),
		chromeMetrics.NewHistogramMetrics(
			"ChromeOS.CWP.PSIMemPressure.Some",
			chromeMetrics.SetMeanHistogramMetricWriter("", "per10000", perf.SmallerIsBetter),
			chromeMetrics.SetPercentileHistogramMetricWriter(50, "_p50", "per10000", perf.SmallerIsBetter),
			chromeMetrics.SetPercentileHistogramMetricWriter(90, "_p90", "per10000", perf.SmallerIsBetter),
			chromeMetrics.SetPercentileHistogramMetricWriter(100, "_max", "per10000", perf.SmallerIsBetter),
			chromeMetrics.SetCountHistogramMetricWriter("_count", perf.SmallerIsBetter),
		),
		chromeMetrics.NewHistogramMetrics(
			"ChromeOS.CWP.PSIMemPressure.Full",
			chromeMetrics.SetMeanHistogramMetricWriter("", "per10000", perf.SmallerIsBetter),
			chromeMetrics.SetPercentileHistogramMetricWriter(50, "_p50", "per10000", perf.SmallerIsBetter),
			chromeMetrics.SetPercentileHistogramMetricWriter(90, "_p90", "per10000", perf.SmallerIsBetter),
			chromeMetrics.SetPercentileHistogramMetricWriter(100, "_max", "per10000", perf.SmallerIsBetter),
		),
	}

	if chromeMetrics.StartHistogramMetrics(ctx, tconn, umaMetrics); err != nil {
		s.Fatal("Failed to Start UMA metrics: ", err)
	}

	numTypes := 0
	if param.inHost {
		numTypes++
	}
	if param.inARC {
		numTypes++
	}
	if param.inCrostini {
		numTypes++
	}

	if numTypes == 0 {
		s.Fatal("No lifecycle unit types")
	}
	// Created tabs/apps/etc. should have memory that is a bit compressible.
	// We use the same value as the low compress ratio in
	// platform.MemoryStressBasic.
	const compressRatio = 0.67
	taskAllocMiB := (2 * int64(info.Total) / 100) / memory.MiB
	var tasks []memoryuser.MemoryTask
	var appsAliveTasks []memoryuser.KillableTask
	var tabsAliveTasks []memoryuser.KillableTask
	var procsAliveTasks []memoryuser.KillableTask
	var appsAliveMetrics []*memoryuser.StillAliveMetricTask
	var tabsAliveMetrics []*memoryuser.StillAliveMetricTask
	var procsAliveMetrics []*memoryuser.StillAliveMetricTask
	for i := 0; i < 3; i++ {
		const numTasks = 50
		if param.inHost {
			for j := 0; j < numTasks/numTypes; j++ {
				const tabOpenCooldown = 2 * time.Second
				task := memoryuser.NewMemoryStressTask(int(taskAllocMiB), compressRatio, tabOpenCooldown)
				tabsAliveTasks = append(tabsAliveTasks, task)
				tasks = append(tasks, task)
			}
			task := memoryuser.NewStillAliveMetricTask(
				tabsAliveTasks,
				fmt.Sprintf("tabs_alive_%d", i),
			)
			tabsAliveMetrics = append(tabsAliveMetrics, task)
			tasks = append(tasks, task)
		}
		if param.inARC {
			for j := 0; j < numTasks/numTypes; j++ {
				task := memoryuser.NewArcLifecycleTask(len(appsAliveTasks), taskAllocMiB*memory.MiB, compressRatio, nil, minimizeArc)
				appsAliveTasks = append(appsAliveTasks, task)
				tasks = append(tasks, task)
			}
			task := memoryuser.NewStillAliveMetricTask(
				appsAliveTasks,
				fmt.Sprintf("apps_alive_%d", i),
			)
			appsAliveMetrics = append(appsAliveMetrics, task)
			tasks = append(tasks, task)
		}
		if param.inCrostini {
			for j := 0; j < numTasks/numTypes; j++ {
				task := memoryuser.NewCrostiniLifecycleTask(preCrostini, len(procsAliveTasks), taskAllocMiB, compressRatio, nil)
				procsAliveTasks = append(procsAliveTasks, task)
				tasks = append(tasks, task)
			}
			task := memoryuser.NewStillAliveMetricTask(
				procsAliveTasks,
				fmt.Sprintf("procs_alive_%d", i),
			)
			procsAliveMetrics = append(procsAliveMetrics, task)
			tasks = append(tasks, task)
		}
	}

	if param.inHost {
		task := memoryuser.NewMinStillAliveMetricTask(tabsAliveMetrics, "tabs_alive_min")
		tasks = append(tasks, task)
	}
	if param.inARC {
		task := memoryuser.NewMinStillAliveMetricTask(appsAliveMetrics, "apps_alive_min")
		tasks = append(tasks, task)
		if err := memoryuser.InstallArcLifecycleTestApps(ctx, preARC, len(appsAliveTasks)); err != nil {
			s.Fatal("Failed to install ArcLifecycleTestApps: ", err)
		}
	}
	if param.inCrostini {
		task := memoryuser.NewMinStillAliveMetricTask(procsAliveMetrics, "procs_alive_min")
		tasks = append(tasks, task)
		if err := memoryuser.InstallCrostiniLifecycle(ctx, preCrostini); err != nil {
			s.Fatal("Failed to install Crostini lifecycle unit: ", err)
		}
		defer func() {
			if err := memoryuser.UninstallCrostiniLifecycle(ctx, preCrostini); err != nil {
				s.Error("Failed to uninstall Crostini lifecycle unit: ", err)
			}
		}()
	}

	p := perf.NewValues()

	// Run all the tasks.
	rp := &memoryuser.RunParameters{
		UseARC:             preARC != nil,
		ExistingChrome:     pre.Chrome,
		BrowserType:        param.browserType,
		ExistingARC:        preARC,
		ExistingPerfValues: p,
	}
	if err := memoryuser.RunTest(ctx, s.OutDir(), tasks, rp); err != nil {
		s.Fatal("RunTest failed: ", err)
	}

	if err := metrics.LogMemoryStats(ctx, basemem, preARC, p, s.OutDir(), ""); err != nil {
		s.Error("Failed to collect memory metrics")
	}
	killsAfter, err := arc.GetAppKills(ctx, tconn)
	if err != nil {
		s.Log("Failed to collect ARC app kill counts: ", err)
	}
	if killsBefore != nil && killsAfter != nil {
		killsAfter.Subtract(killsBefore).LogPerfMetrics(p, "")
	}
	if err := chromeMetrics.WriteHistogramMetrics(ctx, tconn, p, umaMetrics); err != nil {
		s.Error("Failed to write UMA metrics: ", err)
	}
	if err := p.Save(s.OutDir()); err != nil {
		s.Error("Failed to save perf.Values: ", err)
	}
}
