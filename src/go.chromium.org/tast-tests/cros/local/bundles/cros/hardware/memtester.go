// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hardware

import (
	"context"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v3/mem"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/hardware/memtester"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/testing"
)

type memtesterMode int

const (
	single memtesterMode = iota
	dual
	iterationDuration = 25 * time.Minute
)

type memtesterParams struct {
	mode        memtesterMode
	memoryUsage float64
	iteration   int
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         Memtester,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "This test starts Single / Dual processes of memtester and runs for given iterations",
		Contacts:     []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "chromeLoggedIn",
		BugComponent: "b:157291", // ChromeOS > External > Intel
		Params: []testing.Param{{
			Name:    "single_quick",
			Val:     memtesterParams{mode: single, memoryUsage: 0.50, iteration: 1},
			Timeout: 1 * iterationDuration,
		}, {
			Name:      "single_bronze",
			Val:       memtesterParams{mode: single, memoryUsage: 0.50, iteration: 25},
			Timeout:   25 * iterationDuration,
			ExtraAttr: []string{"group:intel-stress"},
		}, {
			Name:    "single_silver",
			Val:     memtesterParams{mode: single, memoryUsage: 0.50, iteration: 50},
			Timeout: 50 * iterationDuration,
		}, {
			Name:    "single_gold",
			Val:     memtesterParams{mode: single, memoryUsage: 0.50, iteration: 75},
			Timeout: 75 * iterationDuration,
		}, {
			Name:    "dual_quick",
			Val:     memtesterParams{mode: dual, memoryUsage: 0.40, iteration: 1},
			Timeout: 1 * iterationDuration,
		}, {
			Name:      "dual_bronze",
			Val:       memtesterParams{mode: dual, memoryUsage: 0.40, iteration: 25},
			Timeout:   25 * iterationDuration,
			ExtraAttr: []string{"group:intel-stress"},
		}, {
			Name:    "dual_silver",
			Val:     memtesterParams{mode: dual, memoryUsage: 0.40, iteration: 50},
			Timeout: 50 * iterationDuration,
		}, {
			Name:    "dual_gold",
			Val:     memtesterParams{mode: dual, memoryUsage: 0.40, iteration: 75},
			Timeout: 75 * iterationDuration,
		}},
	})
}

func Memtester(ctx context.Context, s *testing.State) {
	memoryUsage := s.Param().(memtesterParams).memoryUsage
	iteration := s.Param().(memtesterParams).iteration
	mode := s.Param().(memtesterParams).mode

	// Stop powerd before running memtester because this test case will run for
	// more than 48 hrs so dut should not go to sleep.
	if err := upstart.StopJob(ctx, "powerd"); err != nil {
		s.Fatal("Failed to stop powerd: ", err)
	}
	defer upstart.RestartJob(ctx, "powerd")

	vmstat, err := mem.VirtualMemory()
	if err != nil {
		s.Fatal("Failed to get memory stats: ", err)
	}

	const mb = 1024 * 1024
	testBytes := int64(float64(vmstat.Total) * memoryUsage)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.Logf("Testing %.1f MB of %.1f MB", float64(testBytes)/mb, float64(vmstat.Total)/mb)
		if err := memtester.Run(ctx, testBytes, iteration); err != nil {
			s.Fatal("Failed to run memtester: ", err)
		}
	}()
	if mode == dual {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.Logf("Testing %.1f MB of %.1f MB", float64(testBytes)/mb, float64(vmstat.Total)/mb)
			if err := memtester.Run(ctx, testBytes, iteration); err != nil {
				s.Fatal("Failed to run dual memtester: ", err)
			}
		}()
	}
	wg.Wait()

}
