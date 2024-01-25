// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cryptohome

import (
	"context"
	"time"

	uda "go.chromium.org/chromiumos/system_api/user_data_auth_proto"

	"go.chromium.org/tast-tests/cros/common/hwsec"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	hwseclocal "go.chromium.org/tast-tests/cros/local/hwsec"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         UssMigrationUpdateAuthFactor,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test that USS migration succeeds during an UpdateAuthFactor operation",
		Contacts: []string{
			"cryptohome-core@google.com",
			"betuls@google.com",
		},
		BugComponent: "b:1088399", // ChromeOS > Security > Cryptohome
		SoftwareDeps: []string{"chrome", "pinweaver"},
		Attr:         []string{"group:mainline", "informational", "group:cryptohome"},
	})
}

// UssMigrationUpdateAuthFactor tests that the updated AuthFactor is working after USS
// migration during the UpdateAuthFactor operation. This test specifically tests the
// real use scenario that PIN is migrated during the UpdateAuthFactor when password is
// already backed by USS.
func UssMigrationUpdateAuthFactor(ctx context.Context, s *testing.State) {
	const (
		userName           = "test@test.com"
		userPassword       = "secret"
		userPin            = "1234"
		userPinNew         = "4321"
		wrongPin           = "1111"
		passwordLabel      = "gaia"
		pinLabel           = "pin"
		passwordKeysetFile = "master.0" // nocheck
		pinKeysetFile      = "master.1" // nocheck
		ussFile            = "/user_secret_stash/uss.0"
		pinAuthFactorFile  = "/auth_factors/pin.pin"
	)

	ctxForCleanup := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cmdRunner := hwseclocal.NewCmdRunner()
	client := hwsec.NewCryptohomeClient(cmdRunner)

	// Wait for cryptohomed to become available if needed.
	if err := cryptohome.CheckService(ctx); err != nil {
		s.Fatal("Failed to ensure cryptohomed: ", err)
	}

	// Clean up obsolete state, in case there's any.
	if err := client.UnmountAll(ctx); err != nil {
		s.Error("Failed to unmount vaults for preparation: ", err)
	}
	if _, err := client.RemoveVault(ctx, userName); err != nil {
		s.Fatal("Failed to remove old vault for preparation: ", err)
	}

	// 1. Create a new user with VaultKeysets.
	if err := func() error {
		// 1. Create a new user with VaultKeysets.
		if err := client.WithAuthSession(ctx, userName, false /*isEphemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT, func(authSessionID string) error {

			// Create user vault.
			if err := client.CreatePersistentUser(ctx, authSessionID); err != nil {
				return errors.Wrap(err, "failed to create user")
			}

			// Mount user home directories and daemon-store directories.
			if _, err := client.PreparePersistentVault(ctx, authSessionID, false /*ecryptfs*/); err != nil {
				return errors.Wrap(err, "failed to mount user profile after creation")
			}
			defer client.Unmount(ctxForCleanup, userName)

			// Add password VaultKeyset.
			if err := client.CreateVaultKeyset(ctx, authSessionID, userPassword /*keyDataLabel=*/, passwordLabel, uda.AuthFactorType_AUTH_FACTOR_TYPE_PASSWORD /*disableKeyData=*/, false); err != nil {
				return errors.Wrap(err, "failed to add password VaultKeyset")
			}

			// Check that the password VaultKeyset file is created.
			if err := cryptohome.CheckKeyBackingStoreExists(ctx, passwordKeysetFile, userName); err != nil {
				return errors.Wrap(err, "failed to check password VaultKeyset file")
			}

			// Add PIN VaultKeyset.
			if err := client.CreateVaultKeyset(ctx, authSessionID, userPin /*keyDataLabel=*/, pinLabel, uda.AuthFactorType_AUTH_FACTOR_TYPE_PIN /*disableKeyData=*/, false); err != nil {
				return errors.Wrap(err, "failed to add pin VaultKeyset")
			}

			// Check that the PIN VaultKeyset file is created.
			if err := cryptohome.CheckKeyBackingStoreExists(ctx, pinKeysetFile, userName); err != nil {
				return errors.Wrap(err, "failed to check PIN VaultKeyset file")
			}
			return nil
		}); err != nil {
			return errors.Wrap(err, "failed to create and set up the user password and PIN with uss and migration disabled")
		}
		return nil
	}(); err != nil {
		s.Fatal("Setup while USS migration was disabled failed: ", err)
	}
	// Cleanup user vault before UssMigrationUpdateAuthFactor exits.
	defer client.RemoveVault(ctxForCleanup, userName)

	// 2. Test that updating a PIN migrates the PIN factor to USS.

	// Authenticate with password to migrate password to USS.
	if err := client.WithAuthSession(ctx, userName, false /*isEphemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT, func(authSessionID string) error {
		if _, err := client.AuthenticateAuthFactor(ctx, authSessionID, passwordLabel, userPassword); err != nil {
			return errors.Wrap(err, "failed to authenticate with password AuthFactor")
		}
		return nil
	}); err != nil {
		s.Fatal("Failed to test authenticate before PIN AuthFactor migration: ", err)
	}

	// Test PIN migration during UpdateAuthFactor.
	if err := client.WithAuthSession(ctx, userName, false /*isEphemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT, func(authSessionID string) error {

		// Authenticate with password as a prerequisite to update PIN.
		if _, err := client.AuthenticateAuthFactor(ctx, authSessionID, passwordLabel, userPassword); err != nil {
			return errors.Wrap(err, "failed to authenticate with password AuthFactor")
		}

		// UpdateAuthFactor should migrate PIN to USS.
		if err := client.UpdatePinAuthFactor(ctx, authSessionID, pinLabel, userPinNew); err != nil {
			return errors.Wrap(err, "failed to update PIN AuthFactor")
		}

		// Check that the PIN AuthFactor file is created.
		if err := cryptohome.CheckKeyBackingStoreExists(ctx, pinAuthFactorFile, userName); err != nil {
			return errors.Wrap(err, "failed to check PIN AuthFactor file")
		}

		// Check that UserSecretStash file exists.
		if err := cryptohome.CheckKeyBackingStoreExists(ctx, ussFile, userName); err != nil {
			return errors.Wrap(err, "failed to stat UserSecretStash file")
		}

		// Authenticate with old PIN should fail.
		if _, err := client.AuthenticatePinAuthFactor(ctx, authSessionID, pinLabel, userPin); err == nil {
			return errors.Wrap(err, "authenticating with the old PIN succeeded but should have failed")
		}

		// Authenticate with updated PIN should succeed.
		if _, err := client.AuthenticatePinAuthFactor(ctx, authSessionID, pinLabel, userPinNew); err != nil {
			return errors.Wrap(err, "authenticating with the updated PIN failed")
		}

		// Test that PIN reset with correct password works after the update.
		if err := cryptohome.TestPinCounterMechanism(ctx, userName, passwordLabel, userPassword, pinLabel, userPinNew, wrongPin, client); err != nil {
			return errors.Wrap(err, "failed in testing PIN lockout and reset mechanism after PIN update")
		}
		return nil
	}); err != nil {
		s.Fatal("Failed to test update PIN AuthFactor after migration: ", err)
	}
}
