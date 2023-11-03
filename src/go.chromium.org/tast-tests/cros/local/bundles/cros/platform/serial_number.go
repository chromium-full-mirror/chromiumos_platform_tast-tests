// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package platform

import (
	"context"
	"fmt"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: SerialNumber,
		Desc: "Checks that device has serial number",
		Contacts: []string{
			"peep-fleet-infra-sw@google.com",
		},
		BugComponent: "b:1032353", // Chrome Operations > Fleet > Software > OS Fleet Automation
		Attr:         []string{"group:labqual_informational", "group:labqual_stable"},
	})
}

func SerialNumber(ctx context.Context, s *testing.State) {
	serialNumber := getSerialNumber(ctx, s)

	// Update Serial number
	updatedSerialNumber := serialNumber + "TEST"
	updateSerialNumber(ctx, s, updatedSerialNumber)
	serialNumberAfterUpdate := getSerialNumber(ctx, s)
	if updatedSerialNumber != serialNumberAfterUpdate {
		s.Fatalf("Serial number does not match the expectation after update. Got: %s; Expected: %s",
			serialNumberAfterUpdate, updatedSerialNumber)
	}

	// Reset back to original Serial number
	updateSerialNumber(ctx, s, serialNumber)
	serialNumberAfterRestore := getSerialNumber(ctx, s)
	if serialNumber != serialNumberAfterRestore {
		s.Fatalf("Serial number does not match the expectation after restore. Got: %s; Expected: %s",
			serialNumberAfterRestore, serialNumber)
	}
}

func updateSerialNumber(ctx context.Context, s *testing.State, serialNumber string) {
	cmd := testexec.CommandContext(ctx, "vpd", "-s", fmt.Sprintf("serial_number=%s", serialNumber))
	out, err := cmd.CombinedOutput()
	if err != nil {
		s.Fatalf("Failed to update serial number to %s : %s ", serialNumber, string(out))
	}
}

func getSerialNumber(ctx context.Context, s *testing.State) string {
	cmd := testexec.CommandContext(ctx, "vpd", "-g", "serial_number")
	out, err := cmd.CombinedOutput()
	if err != nil {
		s.Fatal("Failed to get serial number, command output: ", string(out))
	}
	s.Log("Found Device Serial Number : ", string(out))
	return string(out)
}
