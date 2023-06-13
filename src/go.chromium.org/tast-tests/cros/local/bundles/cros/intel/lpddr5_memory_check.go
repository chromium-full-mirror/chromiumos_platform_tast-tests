// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package intel

import (
	"context"
	"regexp"
	"strings"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         LPDDR5MemoryCheck,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies maximum data rate of LPDDR5 memory",
		Contacts:     []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		BugComponent: "b:157291", // ChromeOS > External > Intel
		Attr:         []string{"group:intel-nda"},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.Model("craask")),
		Fixture:      "chromeLoggedIn",
	})
}

func LPDDR5MemoryCheck(ctx context.Context, s *testing.State) {
	const cbmemCommand = "cbmem -c | grep SPD"
	cbmemOut, err := testexec.CommandContext(ctx, "sh", "-c", cbmemCommand).Output()
	if err != nil {
		s.Fatal("Failed to execute cbmem command: ", err)
	}

	spdModulePartRe := regexp.MustCompile(`SPD: module part number is ([A-Z0-9\-\s+\:]+)\n`)
	spdPartNumberMatches := spdModulePartRe.FindStringSubmatch(string(cbmemOut))
	if spdPartNumberMatches == nil {
		s.Fatal("Failed to get part number from cbmem output")
	}

	dmiDecodeCommand := "dmidecode --type memory"
	dmiDecodeOut, err := testexec.CommandContext(ctx, "sh", "-c", dmiDecodeCommand).Output()
	if err != nil {
		s.Fatal("Failed to execute dmidecode command: ", err)
	}

	partNumberRe := regexp.MustCompile(`Part Number: ([A-Z0-9\-\s+\:]+)\n`)
	partNumberMatches := partNumberRe.FindStringSubmatch(string(dmiDecodeOut))
	if partNumberMatches == nil {
		s.Fatal("Failed to get part number from dmi decode output")
	}

	if spdPartNumberMatches[1] != partNumberMatches[1] {
		s.Fatal("Part number of cbmem output did not match with part number of dmi decode output")
	}

	memoryTypeString := "Type: LPDDR5"
	if !strings.Contains(string(dmiDecodeOut), memoryTypeString) {
		s.Fatal("Failed: memory type is not LPDDR5")
	}

	memorySpeedString := "Speed: 4800 MT/s"
	if !strings.Contains(string(dmiDecodeOut), memorySpeedString) {
		s.Fatal("Failed: memory speed is not 4800 MT/s")
	}

	lshwClockCommand := "lshw -C memory | grep clock"
	lshwOut, err := testexec.CommandContext(ctx, "sh", "-c", lshwClockCommand).Output()
	if err != nil {
		s.Fatal("Failed to execute lshw clock command: ", err)
	}

	lshwClockString := "clock: 4800 MT/s"
	if !strings.Contains(string(lshwOut), lshwClockString) {
		s.Fatal("Failed: clock speed is not 4800 MT/s")
	}
}
