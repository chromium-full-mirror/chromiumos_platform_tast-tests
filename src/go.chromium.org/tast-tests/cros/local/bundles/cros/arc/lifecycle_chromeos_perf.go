// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/memory"
	"go.chromium.org/tast-tests/cros/local/memory/kernelmeter"
	"go.chromium.org/tast-tests/cros/local/memory/memoryuser"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         LifecycleChromeOSPerf,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Launch many memory hogging tabs, and count how many are killed",
		Contacts: []string{
			"arcvm-eng@google.com",
			"cwd@chromium.org",
		},
		// ChromeOS > Platform > Virtualization > VM Technology
		BugComponent: "b:930563",
		Attr:         []string{"group:crosbolt", "crosbolt_nightly"},
		SoftwareDeps: []string{"chrome"},
		Data: []string{
			memoryuser.AllocPageFilename,
			memoryuser.JavascriptFilename,
		},
		Params: []testing.Param{{
			Name:              "noarc",
			ExtraSoftwareDeps: []string{"arc"}, // to prevent this from running on non-ARC boards
			Fixture:           "chromeLoggedIn",
		}, {
			Name:              "vm",
			ExtraSoftwareDeps: []string{"android_vm"},
			Fixture:           "arcBooted",
		}},
		Timeout: 10 * time.Minute,
	})
}

func LifecycleChromeOSPerf(ctx context.Context, s *testing.State) {
	// Define the list of tabs to load.
	const numTabs = 100
	info, err := kernelmeter.MemInfo()
	if err != nil {
		s.Fatal("Failed to get /proc/meminfo: ", err)
	}
	// Tabs are sized so that if all tabs were alive at once, they would use 2x
	// the memory of the system.
	tabAllocMiB := (int)((2 * info.Total / numTabs) / memory.MiB)
	var tasks []memoryuser.MemoryTask
	var tabsAliveTasks []memoryuser.KillableTask
	server := memoryuser.NewMemoryStressServer(s.DataFileSystem())
	defer server.Close()
	for i := 0; i < numTabs; i++ {
		const tabOpenCooldown = 2 * time.Second
		task := server.NewMemoryStressTask(tabAllocMiB, 0.67, tabOpenCooldown)
		tasks = append(tasks, task)
		tabsAliveTasks = append(tabsAliveTasks, task)
	}
	s.Logf("Created tasks to open %d tabs of %d MiB", numTabs, tabAllocMiB)

	// Add a task to collect metrics on which tabs are still alive.
	tasks = append(tasks, memoryuser.NewStillAliveMetricTask(tabsAliveTasks, "tabs_alive"))

	var a *arc.ARC
	var cr *chrome.Chrome
	if hasChrome, ok := s.FixtValue().(chrome.HasChrome); !ok {
		pre := s.FixtValue().(*arc.PreData)
		cr = pre.Chrome
		a = pre.ARC
	} else {
		cr = hasChrome.Chrome()
	}

	rp := &memoryuser.RunParameters{
		ExistingChrome: cr,
		ExistingARC:    a,
	}
	if err := memoryuser.RunTest(ctx, s.OutDir(), tasks, rp); err != nil {
		s.Fatal("RunTest failed: ", err)
	}
}
