// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hwsec

import (
	"bytes"
	"context"
	"time"

	"chromiumos/tast/common/hwsec"
	"chromiumos/tast/ctxutil"
	"chromiumos/tast/errors"
	"chromiumos/tast/remote/bundles/cros/hwsec/util"
	hwsecremote "chromiumos/tast/remote/hwsec"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: VerifyUnusableVaultBehaviour,
		Desc: "Verifies that the vault is destroyed if unusable",
		Contacts: []string{
			"cros-hwsec@chromium.org",
			"dlunev@chromium.org",
		},
		BugComponent: "b:1188704",
		SoftwareDeps: []string{"reboot", "tpm"},
		Attr:         []string{"group:hwsec_destructive_func"},
		Timeout:      5 * time.Minute,
	})
}

func VerifyUnusableVaultBehaviour(ctx context.Context, s *testing.State) {
	// CRYPTOHOME_ERROR_UNUSABLE_VAULT is returned by cryptohome UserDataAuth proto binding.
	const CryptohomeUnusableVaultErrorNumber = 53
	// CRYPTOHOME_ERROR_MOUNT_MOUNT_POINT_BUSY is returned by cryptohome UserDataAuth proto binding.
	const CryptohomeErrorMountPointBusyErrorNumber = 6

	cmdRunner := hwsecremote.NewCmdRunner(s.DUT())

	helper, err := hwsecremote.NewHelper(cmdRunner, s.DUT())
	if err != nil {
		s.Fatal("Failed to create hwsec local helper: ", err)
	}

	utility := helper.CryptohomeClient()

	// Disable UserSecretStash to use VaultKeyset.
	cleanupFunction, err := helper.DisableUserSecretStash(ctx)
	if err != nil {
		s.Fatal("Failed to disable the UserSecretStash experiment: ", err)
	}
	defer cleanupFunction(ctx)

	// Reserve time for cleanupFunction.
	ctx, cancel := ctxutil.Shorten(ctx, 15*time.Second)
	defer cancel()

	// Resets the TPM and system states before running the tests.
	if err := helper.EnsureTPMAndSystemStateAreReset(ctx); err != nil {
		s.Fatal("Failed to ensure resetting TPM and system: ", err)
	}
	if err := helper.EnsureTPMIsReady(ctx, hwsec.DefaultTakingOwnershipTimeout); err != nil {
		s.Fatal("Failed to wait for TPM to be owned: ", err)
	}
	if _, err := utility.RemoveVault(ctx, util.FirstUsername); err != nil {
		s.Fatal("Failed to remove user vault: ", err)
	}

	s.Log("Phase 1: mounts vault for the test user")

	if err := utility.MountVault(ctx, util.Password1Label, hwsec.NewPassAuthConfig(util.FirstUsername, util.FirstPassword1), true, hwsec.NewVaultConfig()); err != nil {
		s.Fatal("Failed to create user vault: ", err)
	}
	if err := utility.CheckTPMWrappedUserKeyset(ctx, util.FirstUsername); err != nil {
		s.Fatal("Check user keyset failed: ", err)
	}
	if err := hwsec.WriteUserTestContent(ctx, utility, cmdRunner, util.FirstUsername, util.TestFileName1, util.TestFileContent); err != nil {
		s.Fatal("Failed to write user test content: ", err)
	}
	if _, err := utility.Unmount(ctx, util.FirstUsername); err != nil {
		s.Fatal("Failed to remove user vault: ", err)
	}

	s.Log("Phase 2: reboot and try mount user vault")

	// Reboot
	if err := helper.Reboot(ctx); err != nil {
		s.Fatal("Failed to reboot: ", err)
	}
	if err := utility.MountVault(ctx, util.Password1Label, hwsec.NewPassAuthConfig(util.FirstUsername, util.FirstPassword1), false, hwsec.NewVaultConfig()); err != nil {
		s.Fatal("Failed to mount user vault: ", err)
	}
	if err := utility.CheckTPMWrappedUserKeyset(ctx, util.FirstUsername); err != nil {
		s.Fatal("Check user keyset failed: ", err)
	}

	// User vault should already exist and shouldn't be destroyed.
	if content, err := hwsec.ReadUserTestContent(ctx, utility, cmdRunner, util.FirstUsername, util.TestFileName1); err != nil {
		s.Fatal("Failed to read user test content: ", err)
	} else if !bytes.Equal(content, []byte(util.TestFileContent)) {
		s.Fatalf("Unexpected test file content: got %q, want %q", string(content), util.TestFileContent)
	}

	if _, err := utility.Unmount(ctx, util.FirstUsername); err != nil {
		s.Fatal("Failed to remove user vault: ", err)
	}

	s.Log("Phase 3: destroy the keyset and see the vault destroyed upon mount")

	hash, err := utility.GetSanitizedUsername(ctx, util.FirstUsername, false)
	if err != nil {
		s.Fatal("Failed to get username's hash: ", err)
	}
	userShadowDir := "/home/.shadow/" + hash
	userKeysetFile := userShadowDir + "/master.0" // nocheck

	// Remove the keyset file to make decryption fail.
	if _, err := cmdRunner.Run(ctx, "rm", "-rf", userKeysetFile); err != nil {
		s.Fatal("Failed to remove the keyset file: ", err)
	}
	// Mount with no valid keyset shall vail...
	if err = utility.MountVault(ctx, util.Password1Label, hwsec.NewPassAuthConfig(util.FirstUsername, util.FirstPassword1), true, hwsec.NewVaultConfig()); err == nil {
		s.Fatal("Mount was expected to fail but succeeded")
	}
	var exitErr *hwsec.CmdExitError
	if !errors.As(err, &exitErr) || (exitErr.ExitCode != CryptohomeUnusableVaultErrorNumber && exitErr.ExitCode != CryptohomeErrorMountPointBusyErrorNumber) {
		s.Fatalf("Unexpected mount error: got %q; want exit status of either %d (CRYPTOHOME_ERROR_UNUSABLE_VAULT) or %d CRYPTOHOME_ERROR_MOUNT_MOUNT_POINT_BUSY", err, CryptohomeUnusableVaultErrorNumber, CryptohomeErrorMountPointBusyErrorNumber)
	}
}
