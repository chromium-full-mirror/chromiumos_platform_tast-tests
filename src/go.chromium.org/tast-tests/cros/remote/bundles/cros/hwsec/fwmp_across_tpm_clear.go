// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hwsec

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/hwsec"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/hwsec/fwmp"
	hwsecremote "go.chromium.org/tast-tests/cros/remote/hwsec"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: FWMPAcrossTPMClear,
		Desc: "Verifies that FirmwareManagementParameters are working correctly across TPM clear",
		Contacts: []string{
			"cros-hwsec@google.com",
			"yich@google.com",
		},
		BugComponent: "b:1188704",
		SoftwareDeps: []string{"reboot", "tpm_clear_allowed", "gsc", "no_qemu"},
		Attr:         []string{"group:hwsec_destructive_func"},
		Timeout:      5 * time.Minute,
	})
}

// FWMPAcrossTPMClear checks that the firmware management parameters are functioning correctly across TPM clear.
func FWMPAcrossTPMClear(ctx context.Context, s *testing.State) {
	cmdRunner := hwsecremote.NewCmdRunner(s.DUT())

	helper, err := hwsecremote.NewHelper(cmdRunner, s.DUT())
	if err != nil {
		s.Fatal("Failed to create hwsec helper: ", err)
	}
	cryptohome := helper.CryptohomeClient()

	// Resets the TPM states before running the tests.
	if err := helper.EnsureTPMAndSystemStateAreReset(ctx); err != nil {
		s.Fatal("Failed to ensure resetting TPM: ", err)
	}
	if err := helper.EnsureTPMIsReady(ctx, hwsec.DefaultTakingOwnershipTimeout); err != nil {
		s.Fatal("Failed to wait for TPM to be owned: ", err)
	}

	// Clear FWMP before the start of the test.
	if err := fwmp.ClearFWMPAndCheck(ctx, cryptohome); err != nil {
		s.Fatal("Failed to clear FWMP at the start of the test: ", err)
	}

	// Now try to set it with the first value, then read it back to check.
	if err := fwmp.SetFWMPAndCheck(ctx, cryptohome, fwmp.TestFlags1, fwmp.TestHash1); err != nil {
		s.Fatal("Failed to set FWMP with test case 1: ", err)
	}

	// Clear the FWMP to make sure it can be cleared.
	if err := fwmp.ClearFWMPAndCheck(ctx, cryptohome); err != nil {
		s.Fatal("Failed to clear FWMP after setting the first test case: ", err)
	}

	// Test again with the second test case.
	if err := fwmp.SetFWMPAndCheck(ctx, cryptohome, fwmp.TestFlags2, fwmp.TestHash2); err != nil {
		s.Fatal("Failed to set FWMP with test case 2: ", err)
	}

	// Reboot the DUT.
	if err := helper.Reboot(ctx); err != nil {
		s.Fatal("Failed to reboot the DUT: ", err)
	}

	// Ensure the FWMP still there after reboot.
	if err := fwmp.CheckFWMPSet(ctx, cryptohome, fwmp.TestFlags2, fwmp.TestHash2); err != nil {
		s.Fatal("Failed to check the second FWMP after reboot the DUT: ", err)
	}

	// Resets the TPM states.
	if err := helper.EnsureTPMAndSystemStateAreReset(ctx); err != nil {
		s.Fatal("Failed to ensure resetting TPM: ", err)
	}

	// Ensure the FWMP still there after reset the TPM.
	if err := fwmp.CheckFWMPSet(ctx, cryptohome, fwmp.TestFlags2, fwmp.TestHash2); err != nil {
		s.Fatal("Failed to check the second FWMP after reset the TPM: ", err)
	}

	// Ensure TPM is ready.
	if err := helper.EnsureTPMIsReady(ctx, hwsec.DefaultTakingOwnershipTimeout); err != nil {
		s.Fatal("Failed to wait for TPM to be owned: ", err)
	}

	// Ensure the FWMP still there after TPM is ready.
	if err := fwmp.CheckFWMPSet(ctx, cryptohome, fwmp.TestFlags2, fwmp.TestHash2); err != nil {
		s.Fatal("Failed to check the second FWMP after TPM is ready: ", err)
	}
}
