// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package storage

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/perf"
	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/storage/util"
	"go.chromium.org/tast/core/testing"
)

const wbFeatureBit = 8

func init() {
	testing.AddTest(&testing.Test{
		Func: UfsConfiguration,
		Desc: "Check UFS paramters are as expected",
		Contacts: []string{
			"chromeos-storage@google.com",
			"dlunev@google.com", // Test author
		},
		BugComponent: "b:974567", // ChromeOS > Platform > System > Storage
		LacrosStatus: testing.LacrosVariantUnneeded,
		Requirements: []string{
			tdreq.UfsStorageControllerVersion,
			tdreq.UfsStorageControllerGear,
			tdreq.UfsStorageControllerTxLanes,
			tdreq.UfsStorageControllerRxLanes,
			tdreq.UfsStorageDeviceVersion,
			tdreq.UfsStorageDeviceGear,
			tdreq.UfsStorageDeviceTxLanes,
			tdreq.UfsStorageDeviceRxLanes,
			tdreq.UfsStorageProvisioningType,
			tdreq.UfsWriteBoosterSupport,
			tdreq.UfsWriteBoosterSpaceMode,
			tdreq.UfsWriteBoosterType,
			tdreq.UfsMinWriteBoosterSize,
			tdreq.UfsRecomendedWriteBoosterSize,
		},
	})
}

func UfsConfiguration(ctx context.Context, s *testing.State) {
	const storageInfoPath = "/mnt/stateful_partition/encrypted/var/log/storage_info.txt"
	storageInfo, err := util.RunCmdWithStringOutput(ctx, s.DUT(), "cat", storageInfoPath)
	if err != nil {
		s.Fatal("Could not read storage info from the device: ", err)
	}

	perfValues := perf.NewValues()

	singularKeys := []string{
		"wSpecVersion",
		"bProvisioningType",
		"bWriteBoosterBufferPreserveUserSpaceEn",
		"bWriteBoosterBufferType",
	}

	localPeerKeys := []string{
		"PA_HSSeries",
		"PA_TxGear",
		"PA_RxGear",
		"PA_AvailTxDataLanes",
		"PA_AvailRxDataLanes",
	}

	var missingKeys []string

	for _, key := range singularKeys {
		value, ok := util.FindSingleInt64ValueUfs(ctx, storageInfo, key)
		if !ok {
			missingKeys = append(missingKeys, "_"+key)
			continue
		}
		perfValues.Set(perf.Metric{
			Name:      "_" + key,
			Unit:      "value",
			Direction: perf.BiggerIsBetter,
		}, float64(value))
	}

	for _, key := range localPeerKeys {
		local, peer, ok := util.FindLocalPeerInt64PairUfs(ctx, storageInfo, key)
		if !ok {
			missingKeys = append(missingKeys, "_local_"+key)
			missingKeys = append(missingKeys, "_peer_"+key)
			continue
		}
		perfValues.Set(perf.Metric{
			Name:      "_local_" + key,
			Unit:      "value",
			Direction: perf.BiggerIsBetter,
		}, float64(local))
		perfValues.Set(perf.Metric{
			Name:      "_peer_" + key,
			Unit:      "value",
			Direction: perf.BiggerIsBetter,
		}, float64(peer))
	}

	for _, key := range missingKeys {
		perfValues.Set(perf.Metric{
			Name:      key,
			Unit:      "value",
			Direction: perf.BiggerIsBetter,
		}, float64(-1))
	}

	ufsFeatures, ok := util.FindSingleInt64ValueUfs(ctx, storageInfo, "dExtendedUFSFeaturesSupport")
	if !ok {
		s.Fatal("Could not verify UFS extended features")
	}

	wbSupported := float64(0)

	if ufsFeatures&(1<<wbFeatureBit) != 0 {
		wbSupported = float64(1)
	}

	perfValues.Set(perf.Metric{
		Name:      "_WriteBoosterSupport",
		Unit:      "value",
		Direction: perf.BiggerIsBetter,
	}, wbSupported)

	wbFraction := float64(0)

	if wbSupported > 0 {
		wbSize, ok := util.FindSingleInt64ValueUfs(ctx, storageInfo, "dNumSharedWriteBoosterBufferAllocUnits")
		if !ok {
			s.Fatal("Could not read wb size")
		}
		lun0Size, ok := util.FindSingleInt64ValueUfs(ctx, storageInfo, "dNumAllocUnits")
		if !ok {
			s.Fatal("Could not read lun0 size")
		}

		wbFraction = float64(wbSize) / float64(lun0Size)
	}

	perfValues.Set(perf.Metric{
		Name:      "_WriteBoosterFraction",
		Unit:      "value",
		Direction: perf.BiggerIsBetter,
	}, wbFraction)

	if err := perfValues.Save(s.OutDir()); err != nil {
		s.Fatal("Can't save keyval results: ", err)
	}
}
