// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package multivm

import (
	"context"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/crostini"
	"go.chromium.org/tast-tests/cros/local/memory"
	"go.chromium.org/tast-tests/cros/local/memory/kernelmeter"
	"go.chromium.org/tast-tests/cros/local/memory/memoryuser"
	"go.chromium.org/tast-tests/cros/local/memory/metrics"
	"go.chromium.org/tast-tests/cros/local/multivm"
	"go.chromium.org/tast/core/testing"
)

type lifecycleParam struct {
	inHost, inARC, inCrostini bool
	browserType               browser.Type
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         Lifecycle,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Create many Apps, Tabs, Processes across multiple VMs, and see how many can stay alive",
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
			Name: "host",
			Pre:  multivm.NoVMStarted(),
			Val:  &lifecycleParam{inHost: true, browserType: browser.TypeAsh},
			ExtraData: []string{
				memoryuser.AllocPageFilename,
				memoryuser.JavascriptFilename,
			},
		}, {
			Name: "host_lacros",
			Pre:  multivm.NoVMLacrosStarted(),
			Val:  &lifecycleParam{inHost: true, browserType: browser.TypeLacros},
			ExtraData: []string{
				memoryuser.AllocPageFilename,
				memoryuser.JavascriptFilename,
			},
			ExtraSoftwareDeps: []string{"lacros"},
		}, {
			Name:              "arc",
			Pre:               multivm.ArcStarted(),
			Val:               &lifecycleParam{inARC: true, browserType: browser.TypeAsh},
			ExtraSoftwareDeps: []string{"android_vm"},
		}, {
			Name:              "arc_lacros",
			Pre:               multivm.ArcLacrosStarted(),
			Val:               &lifecycleParam{inARC: true, browserType: browser.TypeLacros},
			ExtraSoftwareDeps: []string{"android_vm", "lacros"},
		}, {
			Name: "crostini",
			Pre:  multivm.CrostiniStarted(),
			Val:  &lifecycleParam{inCrostini: true, browserType: browser.TypeAsh},
			ExtraData: []string{
				crostini.GetContainerMetadataArtifact("bullseye", false),
				crostini.GetContainerRootfsArtifact("bullseye", false),
			},
			ExtraHardwareDeps: crostini.CrostiniStable,
			ExtraSoftwareDeps: []string{"vm_host"},
		}, {
			Name:              "arc_host",
			Pre:               multivm.ArcStarted(),
			Val:               &lifecycleParam{inARC: true, inHost: true, browserType: browser.TypeAsh},
			ExtraSoftwareDeps: []string{"android_vm"},
			ExtraData: []string{
				memoryuser.AllocPageFilename,
				memoryuser.JavascriptFilename,
			},
		}, {
			Name:              "host_with_bg_arc",
			Pre:               multivm.ArcStarted(),
			Val:               &lifecycleParam{inHost: true, browserType: browser.TypeAsh},
			ExtraSoftwareDeps: []string{"android_vm"},
			ExtraData: []string{
				memoryuser.AllocPageFilename,
				memoryuser.JavascriptFilename,
			},
		}},
	})
}

func Lifecycle(ctx context.Context, s *testing.State) {
	pre := s.PreValue().(*multivm.PreData)
	param := s.Param().(*lifecycleParam)
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

	var server *memoryuser.MemoryStressServer
	numTypes := 0
	if param.inHost {
		server = memoryuser.NewMemoryStressServer(s.DataFileSystem())
		defer server.Close()
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
	const numTasks = 100
	taskAllocMiB := (2 * int64(info.Total) / numTasks) / memory.MiB
	var tasks []memoryuser.MemoryTask
	var tabsAliveTasks []memoryuser.KillableTask
	var appsAliveTasks []memoryuser.KillableTask
	var procsAliveTasks []memoryuser.KillableTask
	for i := 0; i < numTasks/numTypes; i++ {
		if param.inHost {
			const tabOpenCooldown = 2 * time.Second
			task := server.NewMemoryStressTask(int(taskAllocMiB), compressRatio, tabOpenCooldown)
			tabsAliveTasks = append(tabsAliveTasks, task)
			tasks = append(tasks, task)
		}
		if param.inARC {
			task := memoryuser.NewArcLifecycleTask(len(appsAliveTasks), int64(taskAllocMiB)*memory.MiB, compressRatio, nil, minimizeArc)
			appsAliveTasks = append(appsAliveTasks, task)
			tasks = append(tasks, task)
		}
		if param.inCrostini {
			task := memoryuser.NewCrostiniLifecycleTask(preCrostini, len(procsAliveTasks), taskAllocMiB, compressRatio, nil)
			procsAliveTasks = append(procsAliveTasks, task)
			tasks = append(tasks, task)
		}
		if len(tasks) == 0 {
			s.Fatal("No MemoryTasks created")
		}
	}

	if param.inHost {
		task := memoryuser.NewStillAliveMetricTask(tabsAliveTasks, "tabs_alive")
		tasks = append(tasks, task)
	}
	if param.inARC {
		task := memoryuser.NewStillAliveMetricTask(appsAliveTasks, "apps_alive")
		tasks = append(tasks, task)
		if err := memoryuser.InstallArcLifecycleTestApps(ctx, preARC, len(appsAliveTasks)); err != nil {
			s.Fatal("Failed to install ArcLifecycleTestApps: ", err)
		}
	}
	if param.inCrostini {
		task := memoryuser.NewStillAliveMetricTask(procsAliveTasks, "procs_alive")
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
		s.Error("Failed to collect memory metrics: ", err)
	}

	killsAfter, err := arc.GetAppKills(ctx, tconn)
	if err != nil {
		s.Log("Failed to collect ARC app kill counts: ", err)
	}
	if killsBefore != nil && killsAfter != nil {
		killsAfter.Subtract(killsBefore).LogPerfMetrics(p, "")
	}
	if err := p.Save(s.OutDir()); err != nil {
		s.Error("Failed to save perf.Values: ", err)
	}
}
