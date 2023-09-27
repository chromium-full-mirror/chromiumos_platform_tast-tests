// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func generateRandomString() string {
	letters := []rune("abcdefghijklmnopqrstuvwxyz0123456789")
	s := make([]rune, 8)
	for i := range s {
		s[i] = letters[rand.Intn(len(letters))]
	}
	return string(s)
}

func init() {
	testing.AddTest(&testing.Test{
		Func: FWSysfsVPD,
		Desc: "Basic check for reading VPD data through sysfs",
		Contacts: []string{
			"chromeos-faft@google.com",
			"js@semihalf.com",
		},
		Requirements: []string{"sys-fw-0021-v01", "sys-fw-0024-v01", "sys-fw-0025-v01"},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Attr:         []string{"group:firmware", "firmware_bios", "firmware_level2"},
		Fixture:      fixture.NormalMode,
		HardwareDeps: hwdep.D(hwdep.ChromeEC(), hwdep.SkipOnModel("hana", "elm", "sycamore", "sycamore360", "telesu", "birch", "maple", "maple14")),
		Timeout:      20 * time.Minute,
		ServiceDeps:  []string{"tast.cros.firmware.BiosService"},
	})
}

// FWSysfsVPD checks for VPD data integrity between reboots
// via reading from sysfs values and logs them on test output.
func FWSysfsVPD(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}

	if err := h.RequireBiosServiceClient(ctx); err != nil {
		s.Fatal("Requiring BiosServiceClient: ", err)
	}

	s.Log("Generating random strings for VPD values")
	roSectionString := generateRandomString()
	rwSectionString := generateRandomString()

	// Log out current VPD values.
	currentVPDROout, err := h.DUT.Conn().CommandContext(ctx, "vpd", "-i", "RO_VPD", "-l").Output()
	if err != nil {
		s.Fatal("Failed to read current RO_VPD value: ", err)
	}

	currentVPDRWout, err := h.DUT.Conn().CommandContext(ctx, "vpd", "-i", "RW_VPD", "-l").Output()
	if err != nil {
		s.Fatal("Failed to read current RW_VPD value: ", err)
	}

	s.Log("Current RO_VPD values: ", string(currentVPDROout))
	s.Log("Current RW_VPD values: ", string(currentVPDRWout))

	s.Log("RW_TEST value: ", rwSectionString)
	s.Log("RO_TEST value: ", roSectionString)

	defer func(ctx context.Context) {
		s.Log("Removing keys from RW_VPD")
		h.EnsureDUTBooted(ctx)
		err = h.DUT.Conn().CommandContext(ctx, "vpd", "-i", "RW_VPD", "-d", "RW_TEST").Run()
		if err != nil {
			s.Error("Failed to delete random RW_VPD value: ", err)
		}

		s.Log("Removing keys from RO_VPD")
		err = h.DUT.Conn().CommandContext(ctx, "vpd", "-i", "RO_VPD", "-d", "RO_TEST").Run()
		if err != nil {
			s.Fatal("Failed to delete random RO_VPD value: ", err)
		}
	}(ctx)

	s.Log("Writing RO_VPD value")
	err = h.DUT.Conn().CommandContext(ctx, "vpd", "-i", "RO_VPD", "-s", fmt.Sprintf("RO_TEST=%s", roSectionString)).Run()
	if err != nil {
		s.Fatal("Failed to write random RO_VPD value: ", err)
	}

	s.Log("Writing RW_VPD value")
	err = h.DUT.Conn().CommandContext(ctx, "vpd", "-i", "RW_VPD", "-s", fmt.Sprintf("RW_TEST=%s", rwSectionString)).Run()
	if err != nil {
		s.Fatal("Failed to write random RW_VPD value: ", err)
	}

	s.Log("Rebooting DUT")
	h.CloseRPCConnection(ctx)
	if err := h.DUT.Reboot(ctx); err != nil {
		s.Fatal("Failed to reboot DUT: ", err)
	}

	s.Log("Verifying RO_VPD value")
	var newVPDROout []byte
	newVPDROout, err = h.DUT.Conn().CommandContext(ctx, "cat", "/sys/firmware/vpd/ro/RO_TEST").Output()
	if err != nil {
		s.Fatal("Failed to read new RO_VPD value after a reboot: ", err)
	}

	if string(newVPDROout) != roSectionString {
		s.Fatalf("RO_VPD section mismatch! (expected: %s, got: %s)", roSectionString, newVPDROout)
	}

	s.Log("Verifying RW_VPD value")
	var newVPDRWout []byte
	newVPDRWout, err = h.DUT.Conn().CommandContext(ctx, "cat", "/sys/firmware/vpd/rw/RW_TEST").Output()
	if err != nil {
		s.Fatal("Failed to read new RW_VPD value after a reboot: ", err)
	}

	if string(newVPDRWout) != rwSectionString {
		s.Fatalf("RO_VPD section mismtch! (expected: %s, got: %s)", rwSectionString, newVPDRWout)
	}
}
