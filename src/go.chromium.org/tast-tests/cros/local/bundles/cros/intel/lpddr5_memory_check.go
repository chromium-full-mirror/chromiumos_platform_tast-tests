// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package intel

import (
	"context"
	"regexp"
	"strings"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/errors"
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
		HardwareDeps: hwdep.D(hwdep.Model("rex", "craask")),
		Fixture:      "chromeLoggedIn",
	})
}

func LPDDR5MemoryCheck(ctx context.Context, s *testing.State) {
	const cbmemCommand = "cbmem -c | grep SPD"
	cbmemOut, err := testexec.CommandContext(ctx, "sh", "-c", cbmemCommand).Output()
	if err != nil {
		s.Fatal("Failed to execute cbmem command: ", err)
	}

	// Example string for matching regexp: SPD: module part number is K3KL6L60GM-MGCT .
	spdModulePartRe := regexp.MustCompile(`SPD: module part number is ([A-Z0-9\-\:]+)\n`)
	spdPartNumberMatches := spdModulePartRe.FindStringSubmatch(string(cbmemOut))
	if spdPartNumberMatches == nil {
		s.Fatal("Failed to get part number from cbmem output")
	}

	dmiDecodeCommand := "dmidecode --type memory"
	dmiDecodeOut, err := testexec.CommandContext(ctx, "sh", "-c", dmiDecodeCommand).Output()
	if err != nil {
		s.Fatal("Failed to execute dmidecode command: ", err)
	}

	// Example string for matching regexp: Part Number: K3KL6L60GM-MGCT .
	partNumberRe := regexp.MustCompile(`Part Number: ([A-Z0-9\-\:]+)\n`)
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

	modelName, err := modelName(ctx)
	if err != nil {
		s.Fatal("Failed to get model name: ", err)
	}

	if modelName == "rex" {
		memorySpeedStringRex := "Speed: 6400 MT/s"
		if !strings.Contains(string(dmiDecodeOut), memorySpeedStringRex) {
			s.Fatalf("Failed to get required memory speed got %v want %v", dmiDecodeOut, memorySpeedStringRex)
		}
	} else {
		memorySpeedStringDefault := "Speed: 4800 MT/s"
		if !strings.Contains(string(dmiDecodeOut), memorySpeedStringDefault) {
			s.Fatalf("Failed to get required memory speed got %v want %v", dmiDecodeOut, memorySpeedStringDefault)
		}
	}

	lshwClockCommand := "lshw -C memory | grep clock"
	lshwOut, err := testexec.CommandContext(ctx, "sh", "-c", lshwClockCommand).Output()
	if err != nil {
		s.Fatal("Failed to execute lshw clock command: ", err)
	}

	if modelName == "rex" {
		lshwClockStringRex := "clock: 3172MHz"
		if !(strings.Contains(string(lshwOut), lshwClockStringRex)) {
			s.Fatalf("Failed to get required clock speed got %v want %v", lshwOut, lshwClockStringRex)
		}
	} else {
		lshwClockStringDefault := "clock: 4800MHz"
		if !(strings.Contains(string(lshwOut), lshwClockStringDefault)) {
			s.Fatalf("Failed to get required clock speed got %v want %v", lshwOut, lshwClockStringDefault)
		}

	}
}

// modelName finds the model of the current DUT.
func modelName(ctx context.Context) (string, error) {
	stdout, err := testexec.CommandContext(ctx, "cros_config", "/", "name").Output(testexec.DumpLogOnError)
	if err != nil {
		return "", errors.Wrap(err, "failed to query cros_config for model name")
	}
	return string(stdout), nil
}
