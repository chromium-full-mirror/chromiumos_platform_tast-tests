// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package platform

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strconv"
	"strings"
	"unicode"

	"chromiumos/tast/common/testexec"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type partitionTestParams struct {
	// Expected rootfs partition size in mebibytes.
	expectedRootfsSizes []int
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         PartitionSizes,
		Desc:         "Checks rootfs partition sizes",
		Contacts:     []string{"chromeos-storage@google.com"},
		BugComponent: "b:974567",
		Attr:         []string{"group:mainline"},
		Params: []testing.Param{{
			Val: partitionTestParams{
				expectedRootfsSizes: []int{2048, 4096},
			},
			ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel("reven")),
		}, {
			// Reven devices may have 4032 MB rootfs partitions.
			// See go/reven-embiggen-kernel-partitions-dd-v2.
			Name: "reven",
			Val: partitionTestParams{
				expectedRootfsSizes: []int{4032, 4096},
			},
			ExtraHardwareDeps: hwdep.D(hwdep.Model("reven")),
		}},
	})
}

func getRootdev(ctx context.Context, s *testing.State) string {
	// Returns the root device path.
	out, err := testexec.CommandContext(ctx, "rootdev", "-s", "-d").Output(testexec.DumpLogOnError)
	if err != nil {
		s.Error("Failed while running rootdev: ", err)
	}
	return strings.TrimSpace(string(out))
}

func devIsPresent(devname string) bool {
	// Returns true if devname is present and accessible.
	devPath := fmt.Sprintf("/sys/block/%s", devname)
	_, err := os.Stat(devPath)
	if err != nil {
		return false
	}
	return true
}

func PartitionSizes(ctx context.Context, s *testing.State) {
	testParam := s.Param().(partitionTestParams)
	s.Log("Using rootdev to get the booted device")
	_, baseDev := path.Split(getRootdev(ctx, s))
	if !devIsPresent(baseDev) {
		s.Error("Failed to discover the internal disk device")
	}
	s.Log("Checking partitions on device ", baseDev)

	// If the device ends in a digit, a "p" appears before the partition number.
	partPrefix := baseDev
	if unicode.IsDigit(rune(baseDev[len(baseDev)-1])) {
		partPrefix += "p"
	}
	// Convert mebibytes to bytes.
	var mib int
	var validSizes []int64
	for i := 0; i < len(testParam.expectedRootfsSizes); i++ {
		mib = testParam.expectedRootfsSizes[i]
		validSizes = append(validSizes, int64(mib)*1024*1024)
	}
	for _, partNum := range []int{3, 5} {
		partDev := partPrefix + strconv.Itoa(partNum)
		// This file contains the partition size in 512-byte sectors.
		// See https://patchwork.kernel.org/patch/7922301/ .
		sizePath := fmt.Sprintf("/sys/block/%s/%s/size", baseDev, partDev)
		out, err := ioutil.ReadFile(sizePath)
		if err != nil {
			s.Errorf("Failed to get %s size: %v", partDev, err)
			continue
		}
		sectors, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
		if err != nil {
			s.Errorf("Failed to parse %q from %s: %v", out, sizePath, err)
			continue
		}
		bytes := sectors * 512

		valid := false
		for _, vs := range validSizes {
			if bytes == vs {
				valid = true
				break
			}
		}
		if valid {
			s.Logf("%s is %d bytes", partDev, bytes)
		} else if partNum == 5 && bytes < 10*1024*1024 {
			// Test images tend to use a stub ROOT-B, so allow very small ones.
			s.Logf("%s is %d bytes; ignoring stub partition", partDev, bytes)
		} else {
			s.Errorf("%s is %d bytes; valid sizes are %v", partDev, bytes, validSizes)
		}
	}
}
