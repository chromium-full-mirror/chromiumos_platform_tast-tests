// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/google/uuid"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast-tests/cros/remote/firmware/reporters"

	fwUtils "go.chromium.org/tast-tests/cros/remote/bundles/cros/firmware/utils"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: UpdateDutFirmwareServo,
		Desc: "Update AP and EC firmware from Servo",
		Contacts: []string{
			"peep-fleet-infra-sw@google.com",
		},
		BugComponent: "b:1032353", // Chrome Operations > Fleet > Software > OS Fleet Automation
		Attr:         []string{"group:labqual_informational"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      fixture.NormalMode,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Timeout:      90 * time.Minute, // 1hr30min.
	})
}

// UpdateDutFirmwareServo reads the current AP firmware and flashes it back from the servo using futility
func UpdateDutFirmwareServo(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}

	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to get config: ", err)
	}

	if err := h.EnsureDUTBooted(ctx); err != nil {
		s.Fatal("Failed to reconnect to DUT after unsuspending: ", err)
	}

	// Confirm the CCD is open.
	hasCCD, err := h.Servo.HasCCD(ctx)
	if err != nil {
		s.Fatal("Failed while checking if servo has a CCD connection: ", err)
	}
	if hasCCD {
		if val, err := h.Servo.GetString(ctx, servo.GSCCCDLevel); err != nil {
			s.Fatal("Failed to get gsc_ccd_level: ", err)
		} else if val != servo.Open {
			s.Logf("CCD is not open, got %q. Attempting to unlock", val)
			if err := h.Servo.SetString(ctx, servo.CR50Testlab, servo.Open); err != nil {
				s.Fatal("Failed to unlock CCD: ", err)
			}
		}
	}

	s.Log("Disabling hardware write protect")
	if err := h.Servo.SetFWWPState(ctx, servo.FWWPStateOff); err != nil {
		s.Fatal("Failed to disable hardware write protect: ", err)
	}
	s.Log("Disabling software write protect")
	if err := h.ServoProxy.RunCommand(ctx, true, "futility", "flash", "--wp-disable", fmt.Sprintf("--servo_port=%d", h.ServoProxy.GetPort())); err != nil {
		s.Fatalf("write protect disable failed at %q", err)
	}
	s.Log("Disabling software write protect completed")

	// Check that the DUT is booted after disabling write protect
	if err := h.EnsureDUTBooted(ctx); err != nil {
		s.Fatal("Failed to reconnect to DUT after unsuspending: ", err)
	}

	uuid, _ := uuid.NewRandom()
	const (
		tmpFirmwareDir     = "/mnt/stateful_partition/tmp"
		backupFirmwareFile = "backupfw.bin"
	)
	servoTmpDir := fmt.Sprintf("%s-%s", tmpFirmwareDir, uuid)
	if err := h.ServoProxy.RunCommand(ctx, false, "mkdir", "-p", servoTmpDir); err != nil {
		s.Fatal("Failed to create temp directory for saving existing firmware: ", err)
	}
	// Delete the tmp directory on the servo at the end
	defer func() {
		s.Log("Deleting tmp directory on servo: ", servoTmpDir)
		if err := h.ServoProxy.RunCommand(ctx, false, "rm", "-rf", servoTmpDir); err != nil {
			s.Fatal("Failed to delete temp directory for saving existing firmware: ", err)
		}
	}()

	// Get the initial fwid from 'crossystem fwid'.
	initialRwFwid, err := h.Reporter.CrossystemParam(ctx, reporters.CrossystemParamFwid)
	if err != nil {
		s.Fatal("Failed to get crossystem fwid: ", err)
	}
	re := regexp.MustCompile(`Google_([a-z-A-Z]*)\.(\d*\.\d*.\d*)`)
	match := re.FindStringSubmatch(initialRwFwid)
	if len(match) != 3 {
		s.Fatalf("Unexpected fw id format from crossystem %v, got: %s", reporters.CrossystemParamFwid, initialRwFwid)
	}
	initialRwFwid = match[2]

	// Get the RO firmware version ID available on the DUT.
	initialROFwid, err := fwUtils.GetFwVersion(ctx, h, reporters.CrossystemParamRoFwid)
	if err != nil {
		s.Fatal("Failed to get AP RO ID: ", err)
	}

	s.Log("Backing up AP firmware")
	if err := h.ServoProxy.RunCommand(ctx, false, "futility", "read", fmt.Sprintf("--servo_port=%d", h.ServoProxy.GetPort()), fmt.Sprintf("%s/%s", servoTmpDir, backupFirmwareFile)); err != nil {
		s.Fatal("Failed to read fw using futility: ", err)
	}

	s.Log("Completed backup of existing fw")
	if err := h.EnsureDUTBooted(ctx); err != nil {
		s.Fatal("Failed to reconnect to DUT after unsuspending: ", err)
	}

	s.Log("Flashing DUT with backup firmware file")
	if err := h.ServoProxy.RunCommand(ctx, false, "futility", "update", "-i", fmt.Sprintf("%s/%s", servoTmpDir, backupFirmwareFile), fmt.Sprintf("--servo_port=%d", h.ServoProxy.GetPort()), "--gbb_flags=0x18"); err != nil {
		s.Fatal("Failed to flash DUT bin file: ", err)
	}
	s.Log("Completed flashing of backup fw")
	if err := h.EnsureDUTBooted(ctx); err != nil {
		s.Fatal("Failed to reconnect to DUT after unsuspending: ", err)
	}

	// Verify RO/RW firmware versions are the prior ones after flashing.
	// This is when RO and RW have the same version ids (i.e., RO_old + RW_old).
	if err = fwUtils.VerifyFwIDs(ctx, h, initialROFwid, initialRwFwid); err != nil {
		s.Fatal("Failed while verifying firmware IDs after flashing at the end of test: ", err)
	}
}
