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

func init() {
	testing.AddTest(&testing.Test{
		Func: Trim,
		Desc: "Performs data integrity trim testing of storage device",
		Contacts: []string{
			"chromeos-storage@google.com",
			"asavery@google.com", // Test author
		},
		BugComponent: "b:974567", // ChromeOS > Platform > System > Storage
		Data:         util.Configs,
		SoftwareDeps: []string{"crossystem"},
		Timeout:      120 * time.Minute,
		Requirements: []string{
			tdreq.StorageTrim,
		},
		Params: []testing.Param{
			{}, {
				Name: "iteration_2",
			},
		},
	})
}

func Trim(ctx context.Context, s *testing.State) {
	bootIDChecker := util.NewBootIDChecker(ctx, s.DUT(), s)
	defer util.FatalIfBootIDChanged(ctx, bootIDChecker, s)

	disk, err := util.GetStandbyRootfs(ctx, s.DUT())
	if err != nil {
		s.Fatal("Failed to get internal disk: ", err)
	}

	// Run Trim
	// Make file size multiple of 4 * chunk size to account for all passes,
	// i.e. 25% = 1/4, 75% = 3/4.
	var filesize uint64
	filesize = util.TrimFileSize - util.TrimFileSize%(4*util.TrimChunkSize)
	s.Log("Running initial trim")
	if err = util.RunTrim(ctx, s.DUT(), disk.Path, 0, filesize); err != nil {
		s.Fatal("Error running trim command: ", err)
	}

	// Write random data
	chunkCount := filesize / util.TrimChunkSize
	// Write random data to disk
	s.Log("Writing random data to disk: ", disk.Path)
	if err := util.WriteRandomData(ctx, s.DUT(), disk.Path, chunkCount); err != nil {
		s.Fatal("Error writing random data to disk: ", err)
	}

	// Get initial hashes
	s.Log("Getting initial hashes for all chunks")
	initialHash, err := util.GetCurrentHashes(ctx, s.DUT(), disk.Path, chunkCount)
	if err != nil {
		s.Fatal("Error getting hashes: ", err)
	}

	// Run trim and compare chunks
	trimCompFunc, err := disk.GetExpectedTrimValueFunc(ctx)
	if err != nil {
		s.Fatal("Could not determine expected trim value: ", err)
	}

	dataVerifyCount := 0
	dataVerifyMatch := 0
	trimVerifyCount := 0
	trimVerifyMatch := 0
	trimVerifyNonDelete := 0

	for _, ratio := range []float64{0, 0.25, 0.5, 0.75, 1} {
		s.Logf("Running trim for ratio: %2f", ratio)
		trimmed, count, err := util.RunTrimForRatio(ctx, s.DUT(), disk.Path, chunkCount, ratio)
		if err != nil {
			s.Fatal("Error running trim command: ", err)
		}
		trimVerifyCount += count

		dataVerifyCount = dataVerifyCount + int(chunkCount) - trimVerifyCount

		for i := uint64(0); i < chunkCount; i++ {
			currHash, err := util.GetCurrentHash(ctx, s.DUT(), disk.Path, i)
			if err != nil {
				s.Fatal("Error calculating current hash: ", err)
			}

			if trimmed[i] && trimCompFunc != nil {
				res, err := trimCompFunc(ctx, s.DUT(), disk.Path, i)
				if err != nil {
					s.Fatal("Error determining trimmed value: ", err)
				}
				if res {
					trimVerifyMatch++
				} else if currHash == initialHash[i] {
					trimVerifyNonDelete++
				}
			} else if currHash == initialHash[i] {
				dataVerifyMatch++
			}
		}
	}

	if dataVerifyMatch < dataVerifyCount {
		s.Fatal("Failed to verify untrimmed data")
	}

	if trimCompFunc == nil {
		s.Log("Trim value not reported")
		return
	}

	if trimVerifyNonDelete > 0 {
		s.Logf("Trimmed chunks with unchanged data: %d", trimVerifyNonDelete)
	}

	if trimVerifyMatch < trimVerifyCount {
		s.Fatalf("Trimmed data does not match expected value, matched %d out of expected %d", trimVerifyMatch, trimVerifyCount)
	}

}
