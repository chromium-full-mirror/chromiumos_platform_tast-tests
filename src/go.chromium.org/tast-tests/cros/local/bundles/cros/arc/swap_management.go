// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"fmt"
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

// The ratio for checking expected swap usage difference to avoid flakiness due to other swap activities.
const swapMemDiffThresholdRatio = 0.9

type swapManagementTestParams struct {
	// Extra Chrome command line options
	chromeArgs []string
	// Expected swap area name
	swapAreaName string
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
				// Enable ZRAM with the size of 1GB
				chromeArgs:   []string{"--enable-features=ArcGuestZram:size/1073741824"},
				swapAreaName: "/dev/block/zram0",
			},
		}},
		Timeout: chrome.LoginTimeout + arc.BootTimeout + time.Minute,
	})
}

const (
	memoryToAllocate int64 = 100 * memory.MiB
)

func SwapManagement(ctx context.Context, s *testing.State) {
	// Adding ArcVM dev config to allow adb root, for invoking per process reclaim.
	arc.AppendToArcvmDevConf(ctx, "--params=androidboot.verifiedbootstate=orange")
	defer arc.RestoreArcvmDevConf(ctx)

	testParams := s.Param().(swapManagementTestParams)
	cr, err := chrome.New(ctx, chrome.ARCEnabled(), chrome.UnRestrictARCCPU(), chrome.ExtraArgs(testParams.chromeArgs...))
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
	defer allocateMemoryTask.Close(ctx, a)

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
	if swapInfoAfterReclaim.used-swapInfoOnBoot.used < swapMemDiffThreshold {
		s.Fatalf("Swap usaged did not increase as expected after per process memory reclaim."+
			" Swap used before: %d, after: %d, allocated memory to reclaim: %d",
			swapInfoOnBoot.used, swapInfoAfterReclaim.used, memoryToAllocate)
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

	lines := strings.Split(string(output), "\n")
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
