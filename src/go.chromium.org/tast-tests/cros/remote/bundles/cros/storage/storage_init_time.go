// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package storage

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"time"

	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"
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
		Contacts:     []string{"chromeos-storage@google.com", "digehlot@google.com"},
		Attr:         []string{"group:storage-qual", "storage-qual_avl_v3"},
		Fixture:      fixture.NormalMode,
		HardwareDeps: hwdep.D(hwdep.CPUSocFamily("intel")),
		Requirements: []string{tdreq.StorageStable},
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

func getStorageThreshold(ctx context.Context, s *testing.State) (time.Duration, string) {
	h := s.FixtValue().(*fixture.Value).Helper

	rootPart, err := reporters.RootPartition(ctx, reporters.New(s.DUT()))
	if err != nil {
		s.Fatal("Failed to get root partition: ", err)
	}

	var thresholdTime time.Duration = 0
	var storageType string = "undefined"
	if util.IsEMMC(rootPart) {
		s.Logf("eMMC storage initialization threshold: %s ", h.Config.StorageInitEmmc)
		thresholdTime = h.Config.StorageInitEmmc
		storageType = "eMMC"
	} else if util.IsNVME(rootPart) {
		s.Logf("NVMe storage initialization threshold: %s ", h.Config.StorageInitNvme)
		thresholdTime = h.Config.StorageInitNvme
		storageType = "NVMe"
	} else if util.IsUFS(rootPart) {
		s.Logf("UFS storage initialization threshold: %s ", h.Config.StorageInitUfs)
		thresholdTime = h.Config.StorageInitUfs
		storageType = "UFS"
	} else {
		s.Fatalf("Unable to get the storage type for root partition %s", rootPart)
	}

	return thresholdTime, storageType
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

	thresholdTime, storageType := getStorageThreshold(ctx, s)
	initTime := getStorageDeviceInitTime(ctx, s)

	if initTime > thresholdTime {
		qualMessage := "please note, failure is only informational, and won't block the qualification"
		s.Fatalf("\"%s\" initialization time: %s is higher than threshold: %s, %s", storageType, initTime, thresholdTime, qualMessage)
	} else {
		s.Logf("\"%s\" initialization time: %s is within threshold: %s", storageType, initTime, thresholdTime)
	}
}
