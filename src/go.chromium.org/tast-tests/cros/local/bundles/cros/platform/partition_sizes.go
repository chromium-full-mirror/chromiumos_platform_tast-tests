// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package platform

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"go.chromium.org/tast-tests/cros/common/testexec"
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

func crosCommonFunc(ctx context.Context, s *testing.State, funcname string) string {
	// Return the result of funcname as a string.
	script := `
set -e
. /usr/sbin/write_gpt.sh
. /usr/share/misc/chromeos-common.sh
load_base_vars`
	script = fmt.Sprintf("%s\n%s", script, funcname)
	out, err := testexec.CommandContext(ctx, "sh", "-c", strings.TrimSpace(script)).Output(testexec.DumpLogOnError)
	if err != nil {
		s.Fatal("Failed to call: ", funcname)
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
	// Try getting the internal disk device name using write_gpt.sh.
	//
	// Note that this will return an empty string in the case where
	// disk_layout.json did not specify a `rootdev_base`.
	devPath := crosCommonFunc(ctx, s, "get_fixed_dst_drive")
	baseDev := filepath.Base(devPath)
	if baseDev == "." { // filepath.Base("") returns "."
		s.Log("Got empty device, attempting to discover")
		baseDev = "sda"
		// In the case where the fixed disk is an NVMe device, it will appear at a
		// different location under /sys/block, so fall back to that location in the
		// case where /sys/block/sda does not exist.
		if !devIsPresent(baseDev) {
			// Try getting the largest NVMe namespace, e.g. "nvme0n1".
			baseDev = crosCommonFunc(ctx, s, "get_largest_nvme_namespace")
			if !devIsPresent(baseDev) {
				s.Error("Failed to discover the internal disk device")
			}
		}
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
