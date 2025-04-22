// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/storage/util"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast-tests/cros/remote/firmware/reporters"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         StorageInitTime,
		Desc:         "Measures storage time against threshold",
		Contacts:     []string{"chromeos-firmware@google.com", "digehlot@google.com"},
		Attr:         []string{"group:firmware"},
		Fixture:      fixture.NormalMode,
		HardwareDeps: hwdep.D(hwdep.CPUSocFamily("intel")),
	})
}

func getStorageDeviceInitTime(ctx context.Context, s *testing.State) time.Duration {
	h := s.FixtValue().(*fixture.Value).Helper
	cbmemTs, err := h.Reporter.GetCBMEMTimestamps(ctx)
	if err != nil {
		s.Fatal("Failed to get CBMEM timestamps: ", err)
	}

	re := regexp.MustCompile(`finished storage device initialization.*\(([\d,]*)\)`)
	regResult := re.FindStringSubmatch(cbmemTs)
	if regResult == nil {
		s.Fatalf("Failed to match %q from output: %s", re, cbmemTs)
	}
	timeoutTs, err := strconv.Atoi(strings.Replace(regResult[1], ",", "", -1))
	if err != nil {
		s.Fatal("Failed to parse result to int: ", err)
	}
	initTime := time.Duration(timeoutTs) * time.Microsecond
	testing.ContextLog(ctx, "Storage Initialization Time: ", initTime)

	return initTime
}

func getStorageThreshold(ctx context.Context, s *testing.State) time.Duration {
	h := s.FixtValue().(*fixture.Value).Helper

	rootPart, err := reporters.RootPartition(ctx, reporters.New(s.DUT()))
	if err != nil {
		s.Fatal("Failed to get root partition: ", err)
	}

	var thresholdTime time.Duration = 0
	if util.IsEMMC(rootPart) {
		s.Logf("eMMC storage initialization threshold: %s ", h.Config.StorageInitEmmc)
		thresholdTime = h.Config.StorageInitEmmc
	} else if util.IsNVME(rootPart) {
		s.Logf("NVMe storage initialization threshold: %s ", h.Config.StorageInitNvme)
		thresholdTime = h.Config.StorageInitNvme
	} else if util.IsUFS(rootPart) {
		s.Logf("UFS storage initialization threshold: %s ", h.Config.StorageInitUfs)
		thresholdTime = h.Config.StorageInitUfs
	} else {
		s.Fatalf("Unable to get the storage type for root partition %s", rootPart)
	}

	return thresholdTime
}

func StorageInitTime(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper

	// Reboot DUT before collecting storage init time.
	ms, err := firmware.NewModeSwitcher(ctx, h)
	if err != nil {
		s.Fatal("Creating mode switcher: ", err)
	}
	if err := ms.ModeAwareReboot(ctx, firmware.WarmReset); err != nil {
		s.Fatal("Failed to reboot: ", err)
	}

	var thresholdTime time.Duration = getStorageThreshold(ctx, s)
	var initTime time.Duration = getStorageDeviceInitTime(ctx, s)

	if initTime > thresholdTime {
		s.Fatalf("Current storage initialization time: %s is higher than threshold: %s", initTime, thresholdTime)
	}
}
