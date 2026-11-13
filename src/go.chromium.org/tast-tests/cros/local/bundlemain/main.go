// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package bundlemain provides a main function implementation for a bundle
// to share it from various local bundle executables.
// The most of the frame implementation is in go.chromium.org/tast/core/bundle package,
// but some utilities, which lives in support libraries for maintenance,
// need to be injected.
package bundlemain

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"time"

	"go.chromium.org/tast/core/bundle"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"

	"go.chromium.org/tast-tests/cros/common/bundleutil"
	"go.chromium.org/tast-tests/cros/common/hwsec"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/crash"
	"go.chromium.org/tast-tests/cros/local/disk"
	"go.chromium.org/tast-tests/cros/local/faillog"
	hwseclocal "go.chromium.org/tast-tests/cros/local/hwsec"
	"go.chromium.org/tast-tests/cros/local/network/ehide"
	"go.chromium.org/tast-tests/cros/local/ready"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast-tests/cros/local/syslog"
	"go.chromium.org/tast-tests/cros/local/upstart"
)

const (
	varLogMessages    = "/var/log/messages"
	statefulPartition = "/mnt/stateful_partition"

	mib                 = 1024 * 1024
	spaceUsageThreshold = 10 * mib
)

// ensureDiskSpace deletes core dumps and, if free space is still below the low
// space threshold, calls purgeable and deletes the returned files until free
// space reaches at least the threshold. It returns the resulting free space.
func ensureDiskSpace(ctx context.Context, purgeable func() []string) (uint64, error) {
	return ensureDiskSpaceWith(
		ctx,
		crash.DeleteCoreDumps,
		func() (uint64, error) { return disk.FreeSpace(statefulPartition) },
		bundleutil.LowSpaceThreshold(),
		purgeable,
	)
}

func ensureDiskSpaceWith(ctx context.Context, deleteCoreDumps func(context.Context) error, freeSpace func() (uint64, error), threshold uint64, purgeable func() []string) (uint64, error) {
	// Unconditionally delete core dumps.
	if err := deleteCoreDumps(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to delete core dumps: ", err)
	}

	free, err := freeSpace()
	if err != nil {
		return 0, err
	}
	// Listing purgeable files stats every external data file, which takes
	// hundreds of milliseconds on low-end DUTs, so do it only when needed.
	if free >= threshold {
		return free, nil
	}

	// Delete purgeable files until free space reaches at least threshold.
	for _, path := range purgeable() {
		if err := os.Remove(path); err != nil {
			testing.ContextLog(ctx, "Failed to remove a purgeable file: ", err)
			continue
		}
		testing.ContextLog(ctx, "Deleted ", path)
		free, err = freeSpace()
		if err != nil {
			return 0, err
		}
		if free >= threshold {
			return free, nil
		}
	}
	return free, nil
}

// newTPMManager returns a client of tpm_managerd. It uses D-Bus because test
// hooks call it up to four times per test, and running tpm_manager_client
// takes ~50 ms each time on low-end DUTs.
func newTPMManager() (*hwseclocal.TPMManagerDBus, error) {
	// testHookLocal ignores TPM errors on devices without tpm_manager_client
	// by matching exec.ErrNotFound, so check PATH before opening D-Bus.
	if _, err := exec.LookPath("tpm_manager_client"); err != nil {
		return nil, err
	}
	return hwseclocal.NewTPMManagerDBus()
}

func hwsecGetDACounter(ctx context.Context) (uint, error) {
	tpmManager, err := newTPMManager()
	if err != nil {
		return 0, err
	}

	// Get the TPM dictionary attack info
	daInfo, err := tpmManager.GetDAInfo(ctx)
	if err != nil {
		return 0, errors.Wrap(err, "failed to get the TPM dictionary attack info")
	}
	return daInfo.Counter, nil
}

func hwsecGetTPMStatus(ctx context.Context) (*hwsec.NonsensitiveStatusInfo, error) {
	tpmManager, err := newTPMManager()
	if err != nil {
		return nil, err
	}

	// Get the TPM nonsensitive status info
	status, err := tpmManager.GetNonsensitiveStatusIgnoreCache(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get the TPM nonsensitive status info")
	}
	return status, nil
}

type mainfwTypeCache struct {
	mu     sync.Mutex
	cached bool
	value  string
}

// get returns the output of "crossystem mainfw_type".
func (c *mainfwTypeCache) get(ctx context.Context, runner hwsec.CmdRunner) (string, error) {
	// The firmware type does not change until the DUT reboots, which this
	// process does not survive, so cache the first successful result.
	// Failed calls are not cached so subsequent calls can retry after
	// transient errors (such as a canceled context).
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.cached {
		out, err := runner.Run(ctx, "crossystem", "mainfw_type")
		if err != nil {
			return "", err
		}
		c.value = string(out)
		c.cached = true
	}
	return c.value, nil
}

var cachedMainfwType mainfwTypeCache

// mainfwType returns the output of "crossystem mainfw_type".
func mainfwType(ctx context.Context) (string, error) {
	return cachedMainfwType.get(ctx, hwseclocal.NewLoglessCmdRunner())
}

func hwsecCheckTPMState(ctx context.Context, origStatus *hwsec.NonsensitiveStatusInfo, origCounter uint) error {
	fwType, err := mainfwType(ctx)
	if err != nil {
		testing.ContextLog(ctx, "Failed to get the firmware type")
	}
	if fwType == "recovery" {
		// Skipping this check when the device is in recovery mode.
		return nil
	}

	status, err := hwsecGetTPMStatus(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get TPM status")
	}

	// We didn't expect the TPM is owned but doesn't have the permission to reset DA counter.
	if status.IsOwned && !status.HasResetLockPermissions {
		testing.ContextLog(ctx, "TPM is owned but doesn't have the permission to reset DA counter")
		// But don't failed the tast if it's not cause by this tast.
		if origStatus == nil || !origStatus.IsOwned || origStatus.HasResetLockPermissions {
			return errors.Errorf("unexpect TPM status: %#v -> %#v", origStatus, status)
		}
	}

	// Only Check the DA counter when the TPM is owned.
	if status.IsOwned {
		da, err := hwsecGetDACounter(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get DA counter")
		}
		if da > origCounter {
			return errors.Errorf("TPM dictionary counter is increased: %v -> %v", origCounter, da)
		}
	}

	return nil
}

type interTestGC struct {
	mu       sync.Mutex
	ranFirst bool
}

// beforeTest runs gc before the second and subsequent tests.
func (g *interTestGC) beforeTest(gc func()) {
	// Run GC between tests so finalizers release leaked os.Process pidfds
	// before open FDs accumulate across tests (b/401592311), without running
	// a redundant GC after the final or only test right before process exit.
	g.mu.Lock()
	shouldGC := g.ranFirst
	g.ranFirst = true
	g.mu.Unlock()
	if shouldGC {
		gc()
	}
}

var bundleGC interTestGC

func testHookLocal(ctx context.Context, s *testing.TestHookState) func(ctx context.Context, s *testing.TestHookState) {
	bundleGC.beforeTest(runtime.GC)

	endLogFn, err := syslog.CollectSyslog()
	if err != nil {
		s.Log("Saving log position: ", err)
	}

	// Ensure disk space and record the current free space.
	checkFreeSpace := false
	freeSpaceBefore, err := ensureDiskSpace(ctx, s.Purgeable)
	if err != nil {
		s.Log("Failed to ensure disk space: ", err)
	} else {
		checkFreeSpace = true
		if freeSpaceBefore < bundleutil.LowSpaceThreshold() {
			s.Logf("Low disk space before starting test: %d MiB available", freeSpaceBefore/mib)
		}
	}

	if err := crash.MarkTestInProgress(ctx, s.TestName()); err != nil {
		s.Log("Failed to mark crash test in progress: ", err)
	}

	// Wait for Internet connectivity, unless the ehide tool is on.
	ehideIsOn, err := ehide.IsOn(ctx)
	if err != nil {
		s.Log("Failed to check ehide state: ", err)
	}
	if !ehideIsOn {
		if err := shill.WaitForOnline(ctx); err != nil {
			s.Log("Failed to wait for Internet connectivity: ", err)
		}
	}

	// Store current DA value before running the tast.
	hwsecDACounter, err := hwsecGetDACounter(ctx)
	if err != nil {
		s.Log("Failed to get TPM DA counter: ", err)
		// Assume the counter value is zero when we failed to get the DA counter.
		hwsecDACounter = uint(0)
	}

	// Store current TPM status before running the tast.
	hwsecTpmStatus, err := hwsecGetTPMStatus(ctx)
	if err != nil {
		s.Log("Failed to get TPM status: ", err)
		hwsecTpmStatus = nil
	}

	return func(ctx context.Context, s *testing.TestHookState) {
		// Ensure the TPM is in the expect state after tast finish.
		if err := hwsecCheckTPMState(ctx, hwsecTpmStatus, hwsecDACounter); err != nil {
			if errors.Is(err, exec.ErrNotFound) {
				s.Log("Ignoring TPM error: ", err)
			} else {
				s.Error("Failed to check TPM state: ", err)
			}
		}

		if s.HasError() {
			faillog.Save(ctx)
		}

		if endLogFn != nil {
			if err := endLogFn(ctx, s.OutDir()); err != nil {
				s.Log("Failed to copy logs: ", err)
			}
		}

		if err := crash.MarkTestDone(ctx, s.TestName()); err != nil {
			s.Log("Failed to unmark crash test in progress file: ", err)
		}

		if checkFreeSpace {
			freeSpaceAfter, err := disk.FreeSpace(statefulPartition)
			if err != nil {
				s.Log("Failed to read the amount of free disk space: ", err)
				freeSpaceAfter = freeSpaceBefore
			}

			var spaceUsage uint64
			if freeSpaceBefore < freeSpaceAfter {
				spaceUsage = 0
			} else {
				spaceUsage = freeSpaceBefore - freeSpaceAfter
			}

			if spaceUsage > spaceUsageThreshold {
				s.Logf("Stateful partition usage: %d MiB (%d MiB free -> %d MiB free)", spaceUsage/mib, freeSpaceBefore/mib, freeSpaceAfter/mib)
			}
		}
	}
}

func runHookLocal(ctx context.Context) (func(context.Context) error, error) {
	if err := testexec.CommandContext(ctx, "which", "initctl").Run(); err != nil {
		return nil, nil
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Set the upstart log priority to info, which enables upstart job state
	// transition logs.
	if err := upstart.SetLogPriority(ctx, upstart.LogPriorityInfo); err != nil {
		return nil, err
	}
	return nil, nil
}

// beforeDownload is called before download external data files.
func beforeDownload(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	// Ensure networking is up.
	if err := shill.WaitForOnline(ctx); err != nil {
		testing.ContextLog(ctx, "Before downloading external data files: failed to wait for online: ", err)
	}
}

// RunLocal is an entry point function for local bundles.
func RunLocal() {
	os.Exit(bundle.LocalDefault(bundle.Delegate{
		Ready:          ready.Wait,
		TestHook:       testHookLocal,
		RunHook:        runHookLocal,
		BeforeDownload: beforeDownload,
	}))
}
