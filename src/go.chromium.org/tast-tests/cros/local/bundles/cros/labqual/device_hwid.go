// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package labqual

import (
	"context"
	"strings"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: DeviceHwid,
		Desc: "Checks that device has hwid set and tests whether hwid can be updated",
		Contacts: []string{
			"peep-fleet-infra-sw@google.com",
		},
		BugComponent: "b:1032353", // Chrome Operations > Fleet > Software > OS Fleet Automation
		Attr:         []string{"group:labqual_informational", "group:labqual_stable"},
	})
}

func DeviceHwid(ctx context.Context, s *testing.State) {
	hwid := getHwid(ctx, s)

	// Update Hwid
	updatedHwid := hwid + "TEST"
	updateHWID(ctx, s, updatedHwid)
	defer func() {
		// Reset back to original Hwid
		updateHWID(ctx, s, hwid)
		hwidAfterRestore := getHwid(ctx, s)
		if hwid != getHwid(ctx, s) {
			s.Fatalf("HWID does not match the expectation after restore. Got: %s; Expected: %s",
				hwidAfterRestore, hwid)
		}
	}()
	hwidAfterUpdate := getHwid(ctx, s)
	if updatedHwid != getHwid(ctx, s) {
		s.Fatalf("HWID does not match the expectation after update. Got: %s; Expected: %s",
			hwidAfterUpdate, updatedHwid)
	}
}

func getHwid(ctx context.Context, s *testing.State) string {
	cmd := testexec.CommandContext(ctx, "futility", "gbb", "-g", "--flash", "--hwid")

	out, err := cmd.CombinedOutput()
	if err != nil {
		cmd.DumpLog(ctx)
		s.Fatalf("Failed to get hwid, command output: %s", string(out))
	}
	hwid := strings.SplitAfter(strings.Trim(string(out), "\n"), "hardware_id: ")
	// If the command ran successfully, after parsing the output, we should have an array with two values
	// and the last value represents the hardware id.
	if len(hwid) != 2 {
		s.Fatalf("Failed to parse hwid, command output: %s", string(out))
	}
	s.Log("Found Device HWID : ", hwid[1])
	return hwid[1]
}

func updateHWID(ctx context.Context, s *testing.State, hwid string) {
	// Update HWID to the AP firmware.
	s.Log(ctx, "Updating HWID from host: update HWID %s in AP firmware: ", hwid)
	out, _ := testexec.CommandContext(ctx, "futility", "gbb", "--flash", "--set", "--hwid", hwid).CombinedOutput()
	s.Log(ctx, "Updating HWID from host: updated HWID in AP file (output): %s", string(out))
}
