// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hwsec

import (
	"context"
	"time"

	fwmps "go.chromium.org/tast-tests/cros/remote/bundles/cros/hwsec/fwmp"
	hwsecremote "go.chromium.org/tast-tests/cros/remote/hwsec"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: FirmwareManagementParameters,
		Desc: "Verifies that FirmwareManagementParameters are working correctly",
		Contacts: []string{
			"cros-hwsec@google.com",
			"zuan@chromium.org",
		},
		BugComponent: "b:1188704",
		SoftwareDeps: []string{"tpm_clear_allowed"},
		Attr:         []string{"group:hwsec_destructive_func"},
	})
}

// FirmwareManagementParameters checks that the firmware management parameters are functioning correctly.
func FirmwareManagementParameters(ctx context.Context, s *testing.State) {
	cmdRunner := hwsecremote.NewCmdRunner(s.DUT())

	hwsecDUT, err := hwsecremote.NewHelper(cmdRunner, s.DUT())
	if err != nil {
		s.Fatal("Failed to create hwsec instance: ", err)
	}
	cryptohomeDUT := hwsecDUT.CryptohomeClient()

	// Resets the TPM states before running the tests.
	if err := hwsecDUT.EnsureTPMAndSystemStateAreReset(ctx); err != nil {
		s.Fatal("Failed to ensure resetting TPM: ", err)
	}

	// First backup the current FWMP so the test doesn't affect what's on DUT.
	fwmp, err := cryptohomeDUT.BackupFWMP(ctx)
	if err != nil {
		s.Fatal("Failed to backup FWMP: ", err)
	}

	ctxForRestoringFWMP := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second) // Give it 10 seconds to restore FWMP.
	defer cancel()

	// Remember to restore it at the end.
	defer func(ctx context.Context) {
		if err := cryptohomeDUT.RestoreFWMP(ctx, fwmp); err != nil {
			s.Error("Failed to restore FWMP: ", err)
		}
	}(ctxForRestoringFWMP)

	// Clear FWMP before the start of the test.
	if err := fwmps.ClearFWMPAndCheck(ctx, cryptohomeDUT); err != nil {
		s.Fatal("Failed to clear FWMP at the start of the test: ", err)
	}

	// Now try to set it with the first value, then read it back to check.
	if err := fwmps.SetFWMPAndCheck(ctx, cryptohomeDUT, fwmps.TestFlags1, fwmps.TestHash1); err != nil {
		s.Fatal("Failed to set FWMP with test case 1: ", err)
	}

	// Clear the FWMP to make sure it can be cleared.
	if err := fwmps.ClearFWMPAndCheck(ctx, cryptohomeDUT); err != nil {
		s.Fatal("Failed to clear FWMP after setting the first test case: ", err)
	}

	// Test again with the second test case.
	if err := fwmps.SetFWMPAndCheck(ctx, cryptohomeDUT, fwmps.TestFlags2, fwmps.TestHash2); err != nil {
		s.Fatal("Failed to set FWMP with test case 2: ", err)
	}
}
