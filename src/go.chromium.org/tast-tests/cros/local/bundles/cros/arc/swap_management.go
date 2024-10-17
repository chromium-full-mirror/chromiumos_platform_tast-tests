// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/memory"
	"go.chromium.org/tast-tests/cros/local/memory/memoryuser"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type swapManagementTestParams struct {
	// Expected swap area name
	swapAreaName string
	// Whether virtual swap is enabled
	virtualSwapEnabled bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         SwapManagement,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that Android has swap management set up correctly",
		Contacts: []string{
			"hungmn@google.com",
			"raging@google.com",
			"arcvm-software@google.com",
		},
		// ChromeOS > Software > ARC++ > ARCVM
		BugComponent: "b:883059",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome", "android_vm"},
		Params: []testing.Param{{
			Name: "zram",
			Val: swapManagementTestParams{
				swapAreaName:       "/dev/block/zram0",
				virtualSwapEnabled: false,
			},
		}, {
			Name: "pmem",
			Val: swapManagementTestParams{
				swapAreaName:       "/dev/block/pmem0",
				virtualSwapEnabled: true,
			},
		}},
		Timeout: chrome.LoginTimeout + arc.BootTimeout + time.Minute,
	})
}

const (
	// The ratio for checking expected swap usage difference to avoid flakiness due to other swap activities.
	swapMemDiffThresholdRatio               = 0.9
	memoryToAllocate          int64         = 100 * memory.MiB
	guestSwapSize             int64         = 1 * memory.GiB
	swapInterval              time.Duration = time.Second
)

func SwapManagement(ctx context.Context, s *testing.State) {
	// Adding ArcVM dev config to allow adb root, for invoking per process reclaim.
	arc.AppendToArcvmDevConf(ctx, "--params=androidboot.verifiedbootstate=orange")
	defer arc.RestoreArcvmDevConf(ctx)

	testParams := s.Param().(swapManagementTestParams)
	guestSwapFeature := fmt.Sprintf("ArcGuestZram:size/%d", guestSwapSize)
	if testParams.virtualSwapEnabled {
		guestSwapFeature += fmt.Sprintf("/virtual_swap_enabled/true/virtual_swap_interval_ms/%d", swapInterval/time.Millisecond)
	}

	cr, err := chrome.New(ctx, chrome.ARCEnabled(), chrome.UnRestrictARCCPU(), chrome.EnableFeatures(guestSwapFeature))
	if err != nil {
		s.Fatal("Failed to connect to Chrome: ", err)
	}
	defer func() {
		if err := cr.Close(ctx); err != nil {
			s.Fatal("Failed to close Chrome while booting ARC: ", err)
		}
	}()
	a, err := arc.New(ctx, s.OutDir(), cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to start ARC: ", err)
	}
	defer a.Close(ctx)

	testing.ContextLog(ctx, "Restarting adbd as root")
	if err := a.Root(ctx); err != nil {
		s.Fatal("Failed to start adb root: ", err)
	}

	// Collect data on boot.
	swapInfoOnBoot, err := getSwapInfo(ctx, a)
	if err != nil {
		s.Fatal("Failed to get swap info: ", err)
	}
	if swapInfoOnBoot == nil {
		s.Fatal("ARC swap area is missing on boot")
	}
	s.Logf("Swap info on boot: %s", swapInfoOnBoot)
	//lint:ignore SA5011 swapInfoOnBoot is actually checked right above
	if swapInfoOnBoot.name != testParams.swapAreaName {
		s.Fatalf("Failed to verify swap area name. Expected %s, but found %s", testParams.swapAreaName, swapInfoOnBoot.name)
	}

	// Allocate memory.
	if err := memoryuser.InstallArcLifecycleTestApps(ctx, a, 1); err != nil {
		s.Fatal("Failed to install ArcLifecycleTestApps: ", err)
	}
	allocateMemoryTask := memoryuser.NewArcLifecycleUnit(0, memoryToAllocate, 1.0, nil, false)
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}
	if err := allocateMemoryTask.Run(ctx, a, tconn); err != nil {
		s.Fatal("Failed to run ArcLifecycleUnit: ", err)
	}
	memoryTaskClosed := false
	defer func() {
		if !memoryTaskClosed {
			allocateMemoryTask.Close(ctx, a)
		}
	}()

	memoryTaskPackageName := allocateMemoryTask.PackageName()
	// Invoke per process reclaim for the memory allocator.
	pidOut, err := a.Command(ctx, "pidof", memoryTaskPackageName).Output(testexec.DumpLogOnError)
	if err != nil {
		s.Fatalf("Failed to get PID of %q: %v", memoryTaskPackageName, err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(pidOut)))
	if err != nil {
		s.Fatalf("Failed to parse PID of %q: %v", memoryTaskPackageName, err)
	}
	_, err = a.Command(ctx, "/system/bin/sh", "-c", fmt.Sprintf("echo all > /proc/%d/reclaim", pid)).Output(testexec.DumpLogOnError)
	if err != nil {
		s.Fatalf("Failed to reclaim all memory for %d: %v", pid, err)
	}

	// Collect swap info after memory reclaim.
	swapInfoAfterReclaim, err := getSwapInfo(ctx, a)
	if err != nil {
		s.Fatal("Failed to get swap info: ", err)
	}
	if swapInfoAfterReclaim == nil {
		s.Fatal("ARC swap area is missing after memory reclaim")
	}
	s.Logf("Swap info after memory reclaim: %s", swapInfoAfterReclaim)

	swapMemDiffThreshold := int64(float64(memoryToAllocate) * swapMemDiffThresholdRatio)
	//lint:ignore SA5011 swapInfoAfterReclaim is actually checked right above
	if swapInfoAfterReclaim.used-swapInfoOnBoot.used < swapMemDiffThreshold {
		s.Fatalf("Swap usage did not increase as expected after per process memory reclaim."+
			" Swap used before: %d, after: %d, allocated memory to reclaim: %d",
			swapInfoOnBoot.used, swapInfoAfterReclaim.used, memoryToAllocate)
	}

	if testParams.virtualSwapEnabled {
		// GoBigSleepLint: Wait for crosvm to swap out the VMA.
		testing.Sleep(ctx, swapInterval+time.Second)

		hostSwapInfoBeforeAppKill, err := getHostSwapInfo(ctx)
		if err != nil {
			s.Fatal("Failed to get host swap info: ", err)
		}
		s.Logf("Host swap info before app kill: %s", hostSwapInfoBeforeAppKill)

		// Also verifies the swap usage from host is reduced after the memory task is closed.
		allocateMemoryTask.Close(ctx, a)
		memoryTaskClosed = true

		hostSwapInfoAfterAppKill, err := getHostSwapInfo(ctx)
		if err != nil {
			s.Fatal("Failed to get host swap info: ", err)
		}
		s.Logf("Host swap info after app kill: %s", hostSwapInfoAfterAppKill)

		if hostSwapInfoBeforeAppKill.used-hostSwapInfoAfterAppKill.used < swapMemDiffThreshold {
			s.Fatalf("Host swap usage did not decrease as expected after app kill."+
				" Swap used before: %d, after: %d, allocated memory from Android app: %d",
				hostSwapInfoBeforeAppKill.used, hostSwapInfoAfterAppKill.used, memoryToAllocate)
		}
	}
}

type swapInfo struct {
	name string
	size int64
	used int64
}

func (si *swapInfo) String() string {
	return fmt.Sprintf("name: %s, size %d, used %d,", si.name, si.size, si.used)
}

func getSwapInfo(ctx context.Context, a *arc.ARC) (*swapInfo, error) {
	output, err := a.Command(ctx, "cat", "/proc/swaps").Output(testexec.DumpLogOnError)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read /proc/swaps file")
	}

	return parseSwapInfo(output)
}

func getHostSwapInfo(ctx context.Context) (*swapInfo, error) {
	procSwap, err := os.ReadFile("/proc/swaps")
	if err != nil {
		return nil, errors.Wrap(err, "failed to read /proc/swaps file")
	}

	return parseSwapInfo(procSwap)
}

// parseSwapInfo parses swap info from the content of /proc/swaps
func parseSwapInfo(procSwapsContent []byte) (*swapInfo, error) {
	lines := strings.Split(strings.TrimSpace(string(procSwapsContent)), "\n")
	if len(lines) < 2 {
		return nil, nil
	}
	swapLineParts := strings.Fields(lines[1])
	sizeKiB, err := strconv.ParseInt(swapLineParts[2], 10, 64)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse swap size")
	}

	usedKiB, err := strconv.ParseInt(swapLineParts[3], 10, 64)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse swap used")
	}

	return &swapInfo{
		name: swapLineParts[0],
		size: sizeKiB * memory.KiB,
		used: usedKiB * memory.KiB,
	}, nil
}
