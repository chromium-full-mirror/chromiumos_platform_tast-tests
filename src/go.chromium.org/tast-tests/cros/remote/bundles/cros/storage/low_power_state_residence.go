// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package storage

import (
	"context"
	"time"

	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/storage/util"
	"go.chromium.org/tast/core/testing"
)

const probeCount = 500
const minProbeTimeDelta = 500 * time.Millisecond
const errorThreshold = probeCount / 100 * 5

// 50 % is way too high, but eMMC have a pretty high runtime_suspend delay
// It doesn't look immediately that reducing it will help anything
// (see b/289845817), so for now make the threshold high for we pretty
// much aim to detect the situation when the disk doesn't to low power
// state at all or almost at all.
const highStateThreshold = probeCount / 100 * 50

func init() {
	testing.AddTest(&testing.Test{
		Func: LowPowerStateResidence,
		Desc: "Check runtime power state management",
		Contacts: []string{
			"chromeos-storage@google.com",
			"dlunev@google.com", // Test author
		},
		//TODO(dlunev): should it report percent high power time to crosbolt?
		BugComponent: "b:974567", // ChromeOS > Platform > System > Storage
		LacrosStatus: testing.LacrosVariantUnneeded,
		Attr:         []string{"group:mainline", "informational"},
		Requirements: []string{
			tdreq.StorageLPST,
		},
	})
}

func LowPowerStateResidence(ctx context.Context, s *testing.State) {
	disk, err := util.GetInternalStorage(ctx, s.DUT())
	if err != nil {
		s.Fatal("Failed to get internal disk: ", err)
	}

	if disk.Type == util.EmmcOverNvmeDisk {
		// BH799 doesn't have presently any way to detect power state
		// transition. It seems to have been working and as long as
		// we don't change its FW, it should be fine.
		// TODO: figure how to check BH799 power state.
		return
	}

	highCount := 0
	lowCount := 0
	errCount := 0

	for i := 0; i < probeCount; i++ {
		probeStartTime := time.Now()
		state, err := disk.CurrentPowerState(ctx)
		if err != nil {
			errCount++
		} else if state == util.DiskHighPowerState {
			highCount++
		} else if state == util.DiskLowPowerState {
			lowCount++
		} else {
			s.Fatalf("Inconsistent state %v %v", state, err)
		}
		// GoBigSleepLint: short sleep to ensure spacing of probes.
		testing.Sleep(ctx, minProbeTimeDelta-time.Since(probeStartTime))
	}

	if errCount > errorThreshold {
		s.Fatalf("Too many errors: %d out of %d probes", errCount, probeCount)
	}

	if highCount > highStateThreshold {
		s.Fatalf("Too much high state detected: %d out of %d probes", highCount, probeCount)
	}
}
