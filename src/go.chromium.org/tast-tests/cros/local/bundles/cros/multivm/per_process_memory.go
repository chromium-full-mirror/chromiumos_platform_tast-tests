// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package multivm

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/memory"
	"go.chromium.org/tast-tests/cros/local/multivm"
	"go.chromium.org/tast-tests/cros/local/session"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PerProcessMemory,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Starts session and records per-process memory usage",
		Contacts: []string{
			"arcvm-eng-team@google.com",
			"yixie@google.com",
		},
		BugComponent: "b:168382",
		Attr:         []string{"group:crosbolt", "crosbolt_nightly"},
		Timeout:      20 * time.Minute,
		SoftwareDeps: []string{"chrome", "android_vm"},
		Params: []testing.Param{{
			Name: "arc",
			Val: &stateManagerOptions{
				chromeOptions: multivm.DefaultChromeOptions,
				vmOptions:     []multivm.VMOptions{multivm.DefaultARCOptions},
			},
		}, {
			Name:              "arc_lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Val: &stateManagerOptions{
				chromeOptions: multivm.LacrosChromeOptions,
				vmOptions:     []multivm.VMOptions{multivm.DefaultARCOptions},
			},
		}},
	})
}

// stateManagerOptions stores options to create multivm.StateManager instance.
type stateManagerOptions struct {
	chromeOptions multivm.ChromeOptions
	vmOptions     []multivm.VMOptions
}

const (
	// Times to repeat the test and measure memory usage, in order to observe
	// the fluctuation between multiple test runs.
	totalRuns = 3
	// Time to wait before measuring memory usage after login.
	waitTimeBeforeMeasure = 60 * time.Second
)

func PerProcessMemory(ctx context.Context, s *testing.State) {
	opts := s.Param().(*stateManagerOptions)

	// Fetching smaps_rollup inside ARCVM requires adb root.
	if err := arc.AppendToArcvmDevConf(ctx, "--params=androidboot.verifiedbootstate=orange"); err != nil {
		s.Fatal("Failed to enable adb root: ", err)
	}
	defer arc.RestoreArcvmDevConf(ctx)

	m := multivm.NewStateManager(opts.chromeOptions, opts.vmOptions...).SetForceActivate(true)
	defer m.Deactivate(ctx)

	lastSessionManagerPid := 0

	var resultFiles []string
	for run := 1; run <= totalRuns; run++ {
		s.Logf("Starting run %d/%d", run, totalRuns)

		if err := m.Activate(ctx, s); err != nil {
			s.Fatal("Could not activate state: ", err)
		}

		if err := m.Chrome().Responded(ctx); err != nil {
			s.Fatal("Chrome did not respond: ", err)
		}
		a := arcFromStateManager(m)

		// Ensure the session was restarted by checking session manager PID.
		pid, err := session.GetSessionManagerPID()
		if err != nil {
			s.Fatal("Failed to get session manager PID: ", err)
		}
		if pid == lastSessionManagerPid {
			s.Fatal("Session manager was not restarted")
		}
		lastSessionManagerPid = pid

		if a != nil {
			// Ensure package manager service is running by checking the
			// existence of the "android" package.
			pkgs, err := a.InstalledPackages(ctx)
			if err != nil {
				s.Fatal("Getting installed packages failed: ", err)
			}

			if _, ok := pkgs["android"]; !ok {
				s.Fatal("Android package not found: ", pkgs)
			}
		}

		s.Log("Wait before collecting metrics")
		// GoBigSleepLint: Let the system quiesce for a while and measure its
		// memory consumption.
		if err := testing.Sleep(ctx, waitTimeBeforeMeasure); err != nil {
			s.Fatal("Failed to wait before collecting metrics")
		}
		s.Log("Will now collect idle perf values")

		smapsRollup, err := memory.PerProcessSmapsRollup(ctx, a != nil)
		if err != nil {
			s.Fatal("Failed to get full smaps_rollup: ", err)
		}

		buf, err := json.Marshal(smapsRollup)
		if err != nil {
			s.Fatal("Failed to marshal json: ", err)
		}

		resultFile := filepath.Join(s.OutDir(), fmt.Sprintf("smaps_full_%d.json", run))
		if err := os.WriteFile(resultFile, buf, 0644); err != nil {
			s.Fatal("Failed to save json: ", err)
		}
		resultFiles = append(resultFiles, resultFile)

		if err := m.Deactivate(ctx); err != nil {
			s.Fatal("Failed to deactivate state: ", err)
		}
	}
}

// arcFromStateManager finds the arc.ARC instance from multivm.StateManager.
func arcFromStateManager(m *multivm.StateManager) *arc.ARC {
	if vm, ok := m.VMs()[multivm.ARCName]; ok {
		return vm.(*arc.ARC)
	}
	return nil
}
