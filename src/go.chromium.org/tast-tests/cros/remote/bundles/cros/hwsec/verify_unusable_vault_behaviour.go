// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hwsec

import (
	"bytes"
	"context"
	"time"

	uda "chromiumos/system_api/user_data_auth_proto"

	"go.chromium.org/tast-tests/cros/common/hwsec"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/hwsec/util"
	hwsecremote "go.chromium.org/tast-tests/cros/remote/hwsec"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: VerifyUnusableVaultBehaviour,
		Desc: "Verifies that the vault is destroyed if unusable",
		Contacts: []string{
			"cros-hwsec@google.com",
			"dlunev@chromium.org",
		},
		BugComponent: "b:1188704",
		SoftwareDeps: []string{"reboot", "tpm_clear_allowed"},
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
	client := hwsec.NewCryptohomeClient(cmdRunner)
	helper, err := hwsecremote.NewHelper(cmdRunner, s.DUT())
	if err != nil {
		s.Fatal("Failed to create hwsec local helper: ", err)
	}

	utility := helper.CryptohomeClient()

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

	if err := client.WithAuthSession(ctx, util.FirstUsername, false /*ephemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT, func(authSessionID string) error {

		if err := client.CreatePersistentUser(ctx, authSessionID); err != nil {
			return errors.Wrap(err, "failed to create persistent user")
		}
		if _, err := client.PreparePersistentVault(ctx, authSessionID, false /*ecryptfs*/); err != nil {
			return errors.Wrap(err, "failed to prepare new persistent vault")
		}
		// Add password VaultKeyset.
		if err := client.CreateVaultKeyset(ctx, authSessionID, util.FirstPassword1 /*keyDataLabel=*/, util.Password1Label, uda.AuthFactorType_AUTH_FACTOR_TYPE_PASSWORD /*disableKeyData=*/, false); err != nil {
			return errors.Wrap(err, "failed to add password VaultKeyset")
		}

		return nil
	}); err != nil {
		s.Fatal("Failed to create and set up the user: ", err)
	}
	defer utility.RemoveVault(ctx, util.FirstUsername)

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
	if err := client.WithAuthSession(ctx, util.FirstUsername, false /*ephemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT, func(authSessionID string) error {
		// Authenticate the same AuthSession using authSessionID.
		// If we cannot authenticate, do not proceed with mount and unmount.
		if _, err := client.AuthenticateAuthFactor(ctx, authSessionID, util.Password1Label, util.FirstPassword1); err != nil {
			return errors.Wrap(err, "failed to authenticate AuthFactor")
		}

		if _, err := client.PreparePersistentVault(ctx, authSessionID, false /*ecryptfs*/); err != nil {
			return errors.Wrap(err, "failed to prepare new persistent vault")
		}
		return nil
	}); err != nil {
		s.Fatal("Failed to create and set up the user: ", err)
	}
	defer utility.RemoveVault(ctx, util.FirstUsername)

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
	// Mount with no valid keyset shall fail...
	if err = client.WithAuthSession(ctx, util.FirstUsername, false /*ephemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT, func(authSessionID string) error {

		if err := client.CreatePersistentUser(ctx, authSessionID); err != nil {
			return errors.Wrap(err, "failed to create persistent user")
		}
		// Add password VaultKeyset.
		if err := client.CreateVaultKeyset(ctx, authSessionID, util.FirstPassword1 /*keyDataLabel=*/, util.Password1Label, uda.AuthFactorType_AUTH_FACTOR_TYPE_PASSWORD /*disableKeyData=*/, false); err != nil {
			return errors.Wrap(err, "failed to add password VaultKeyset")
		}

		if _, err := client.PreparePersistentVault(ctx, authSessionID, false /*ecryptfs*/); err != nil {
			return errors.Wrap(err, "failed to prepare new persistent vault")
		}
		return nil
	}); err == nil {
		s.Fatal("Mount was expected to fail but succeeded")
	}
	var exitErr *hwsec.CmdExitError
	if !errors.As(err, &exitErr) || (exitErr.ExitCode != CryptohomeUnusableVaultErrorNumber && exitErr.ExitCode != CryptohomeErrorMountPointBusyErrorNumber) {
		s.Fatalf("Unexpected mount error: got %q; want exit status of either %d (CRYPTOHOME_ERROR_UNUSABLE_VAULT) or %d CRYPTOHOME_ERROR_MOUNT_MOUNT_POINT_BUSY", err, CryptohomeUnusableVaultErrorNumber, CryptohomeErrorMountPointBusyErrorNumber)
	}
}
