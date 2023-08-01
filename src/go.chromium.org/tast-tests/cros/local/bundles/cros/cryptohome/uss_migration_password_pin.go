// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cryptohome

import (
	"context"
	"time"

	uda "chromiumos/system_api/user_data_auth_proto"

	"go.chromium.org/tast-tests/cros/common/hwsec"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	hwseclocal "go.chromium.org/tast-tests/cros/local/hwsec"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         UssMigrationPasswordPin,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test that migration of two factors succeed when password is migrated first and PIN is migrated second",
		Contacts: []string{
			"cryptohome-core@google.com",
			"betuls@google.com",
		},
		BugComponent: "b:1088399", // ChromeOS > Security > Cryptohome
		SoftwareDeps: []string{"chrome", "pinweaver"},
		Attr:         []string{"group:mainline", "group:cryptohome"},
	})
}

// UssMigrationPasswordPin tests the AuthFactors are working after USS migration during the user
// login. This test specifically test the scenario that password is migrated first and PIN is
// migrated second.
func UssMigrationPasswordPin(ctx context.Context, s *testing.State) {
	const (
		userName               = "test@test.com"
		userPassword           = "secret"
		wrongPassword          = "wrong secret"
		userPin                = "1234"
		userPinNew             = "4321"
		wrongPin               = "1111"
		passwordLabel          = "gaia"
		pinLabel               = "pin"
		passwordKeysetFile     = "master.0" // nocheck
		pinKeysetFile          = "master.1" // nocheck
		ussFile                = "/user_secret_stash/uss.0"
		passwordAuthFactorFile = "/auth_factors/password.gaia"
		pinAuthFactorFile      = "/auth_factors/pin.pin"
	)

	ctxForCleanup := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cmdRunner := hwseclocal.NewCmdRunner()
	client := hwsec.NewCryptohomeClient(cmdRunner)
	helper, err := hwseclocal.NewHelper(cmdRunner)
	if err != nil {
		s.Fatal("Failed to create hwsec local helper: ", err)
	}
	daemonController := helper.DaemonController()

	// Wait for cryptohomed to become available if needed.
	if err := daemonController.Ensure(ctx, hwsec.CryptohomeDaemon); err != nil {
		s.Fatal("Failed to ensure cryptohomed: ", err)
	}

	// Clean up obsolete state, in case there's any.
	if err := client.UnmountAll(ctx); err != nil {
		s.Error("Failed to unmount vaults for preparation: ", err)
	}
	if _, err := client.RemoveVault(ctx, userName); err != nil {
		s.Fatal("Failed to remove old vault for preparation: ", err)
	}

	// 1. Create a new user with VaultKeysets. Disable USS and USS migration for this initial
	// setup.
	if err := func() error {
		// Disable UserSecretStash.
		cleanupUSSDisabled, err := helper.DisableUserSecretStash(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to disable UserSecretStash")
		}
		defer cleanupUSSDisabled(ctxForCleanup)

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
			// Add password AuthFactor.
			if err := client.AddAuthFactor(ctx, authSessionID, passwordLabel, userPassword); err != nil {
				return errors.Wrap(err, "failed to add password AuthFactor")
			}
			// Check that the password VaultKeyset file is created.
			if err := cryptohome.CheckKeyBackingStoreExists(ctx, passwordKeysetFile, userName); err != nil {
				return errors.Wrap(err, "failed to check password VaultKeyset file")
			}
			// Add PIN AuthFactor.
			if err := client.AddPinAuthFactor(ctx, authSessionID, pinLabel, userPin); err != nil {
				return errors.Wrap(err, "failed to add PIN AuthFactor")
			}
			// Check that the PIN VaultKeyset file is created.
			if err := cryptohome.CheckKeyBackingStoreExists(ctx, pinKeysetFile, userName); err != nil {
				return errors.Wrap(err, "failed to check PIN VaultKeyset file")
			}
			if err := cryptohome.WriteFileForPersistence(ctx, userName); err != nil {
				return errors.Wrap(err, "failed to write test file")
			}
			return nil
		}); err != nil {
			return errors.Wrap(err, "failed to create and set up the user password and PIN with uss and migration disabled")
		}
		return nil
	}(); err != nil {
		s.Fatal("Setup while USS migration was disabled failed: ", err)
	}
	// Cleanup user vault before UssMigrationPasswordPin exists.
	defer client.RemoveVault(ctxForCleanup, userName)

	// 2. Test the migration of the two created factors. Test that only successful
	// authentication migrates the password and PIN factors, and after the migration
	// PIN reset mechanism works. Also updating a PIN succeeds after the migration.

	// Enable USS and USS migration for the second phase of the test.

	// Enable UserSecretStash.
	cleanupUSSExperiment, err := helper.EnableUserSecretStash(ctx)
	if err != nil {
		s.Fatal("Failed to enable the UserSecretStash experiment: ", err)
	}
	defer cleanupUSSExperiment(ctxForCleanup)

	// 2.1. Test password migration.
	if err := client.WithAuthSession(ctx, userName, false /*isEphemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT, func(authSessionID string) error {

		// Try authenticate with wrong password.
		_, err = client.AuthenticateAuthFactor(ctx, authSessionID, passwordLabel, wrongPassword)
		if err == nil {
			return errors.Wrap(err, "AuthenticateAuthFactor succeeded with wrong password, should have failed")
		}
		// Check that the password VaultKeyset file is not migrated.
		err = cryptohome.CheckKeyBackingStoreExists(ctx, passwordAuthFactorFile, userName)
		if err == nil {
			return errors.Wrap(err, "AuthFactor file shouldn't have created, but exists")
		}
		// Authenticate with correct password and migrate backing store to USS.
		if _, err := client.AuthenticateAuthFactor(ctx, authSessionID, passwordLabel, userPassword); err != nil {
			return errors.Wrap(err, "USS migration test failed at authentication step with password keyset")
		}
		// Check that the password AuthFactor and USS files are created.
		if err := cryptohome.CheckKeyBackingStoreExists(ctx, ussFile, userName); err != nil {
			return errors.Wrap(err, "failed to check USS file after migration")
		}
		if err := cryptohome.CheckKeyBackingStoreExists(ctx, passwordAuthFactorFile, userName); err != nil {
			return errors.Wrap(err, "failed to check password AuthFactor file after migration")
		}
		if err := cryptohome.MountAndVerify(ctx, userName, authSessionID, false /*ecryptfs*/); err != nil {
			return errors.Wrap(err, "failed to mount and verify persistence after password migration")
		}
		defer client.Unmount(ctxForCleanup, userName)
		// Authenticate with correct password factor should succeed after migration.
		if _, err := client.AuthenticateAuthFactor(ctx, authSessionID, passwordLabel, userPassword); err != nil {
			return errors.Wrap(err, "USS migration test failed at authentication step after the migration of password")
		}
		// Test that lock screen works after migration.
		if err := cryptohome.TestLockScreen(ctx, userName, userPassword, wrongPassword, passwordLabel, client); err != nil {
			return errors.Wrap(err, "lock screen test with password failed after USS migration of password")
		}
		return nil
	}); err != nil {
		s.Fatal("Failed to test password migration: ", err)
	}

	// 2.2 Run a soft test on the PIN lock counter. Since entering correct PIN migrates
	// the PIN run this test partially to see password can reset the counter.
	if err := client.WithAuthSession(ctx, userName, false /*isEphemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT, func(authSessionID string) error {
		const numberOfWrongAttemptToLock = 6

		// Try authenticate with wrong PIN 6 times to lock out the PIN.
		replyError := &uda.AuthenticateAuthFactorReply{}
		for i := 0; i < numberOfWrongAttemptToLock; i++ {
			replyError, err = client.AuthenticatePinAuthFactor(ctx, authSessionID, pinLabel, wrongPin)
			if err == nil {
				return errors.Wrap(err, "authentication with wrong PIN succeeded unexpectedly")
			}
		}
		if replyError.Error != uda.CryptohomeErrorCode_CRYPTOHOME_ERROR_TPM_DEFEND_LOCK {
			return errors.Errorf("PIN is not locked out after too many wrong attempts. The received wrong error message is: %v", replyError.Error)
		}

		// Authenticate with correct PIN should fail since the PIN is locked out.
		replyError, err = client.AuthenticatePinAuthFactor(ctx, authSessionID, pinLabel, userPin)
		if err == nil {
			return errors.Wrap(err, "authenticating with correct PIN after PIN lock out succeded unexpectedly, should have failed")
		}
		if replyError.Error != uda.CryptohomeErrorCode_CRYPTOHOME_ERROR_TPM_DEFEND_LOCK {
			return errors.Errorf("PIN should have been in locked out state, but it is not. The error message received is: %v", replyError.Error)
		}

		// Authenticate with password AuthFactor and resets the PIN counter.
		if _, err := client.AuthenticateAuthFactor(ctx, authSessionID, passwordLabel, userPassword); err != nil {
			return errors.Wrap(err, "failed to authenticate with password AuthFactor after PIN is locked")
		}
		return nil
	}); err != nil {
		s.Fatal("Failed to test update PIN AuthFactor after migration: ", err)
	}

	// 2.3. Test PIN migration.
	if err := client.WithAuthSession(ctx, userName, false /*isEphemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT, func(authSessionID string) error {

		// Try authenticate with wrong PIN.
		if _, err := client.AuthenticatePinAuthFactor(ctx, authSessionID, pinLabel, wrongPin); err == nil {
			return errors.Wrap(err, "authenticateAuthFactor succeeded with wrong PIN, should have failed")
		}
		// Check that the PIN AuthFactor file doesn't exist; PIN is not migrated.
		err = cryptohome.CheckKeyBackingStoreExists(ctx, pinAuthFactorFile, userName)
		if err == nil {
			return errors.Wrap(err, "PIN AuthFactor file shouldn't have created, but exists")
		}
		// Authenticate with correct PIN and migrate the key backing store to USS.
		if _, err := client.AuthenticatePinAuthFactor(ctx, authSessionID, pinLabel, userPin); err != nil {
			return errors.Wrap(err, "USS migration test failed at authentication step with PIN keyset")
		}
		if err := cryptohome.MountAndVerify(ctx, userName, authSessionID, false /*ecryptfs*/); err != nil {
			return errors.Wrap(err, "failed to mount and verify persistence after PIN migration")
		}
		defer client.Unmount(ctxForCleanup, userName)
		// Check that the PIN AuthFactor file is created.
		if err := cryptohome.CheckKeyBackingStoreExists(ctx, pinAuthFactorFile, userName); err != nil {
			return errors.Wrap(err, "failed to check PIN AuthFactor file")
		}
		// Check that UserSecretStash file exists.
		if err := cryptohome.CheckKeyBackingStoreExists(ctx, ussFile, userName); err != nil {
			return errors.Wrap(err, "failed to stat UserSecretStash file")
		}
		// Authenticate with correct PIN factor should succeed after the migration.
		if _, err := client.AuthenticatePinAuthFactor(ctx, authSessionID, pinLabel, userPin); err != nil {
			return errors.Wrap(err, "USS migration test failed at authentication step after the migration of PIN keyset")
		}
		// Test that PIN reset with correct password works after the migration.
		if err := cryptohome.TestPinCounterMechanism(ctx, userName, passwordLabel, userPassword, pinLabel, userPin, wrongPin, client); err != nil {
			return errors.Wrap(err, "failed in testing PIN lockout and reset mechanism after PIN migration")
		}
		return nil
	}); err != nil {
		s.Fatal("Failed to test PIN migration: ", err)
	}

	// 2.4 Test update PIN AuthFactor after migration.
	if err := client.WithAuthSession(ctx, userName, false /*isEphemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT, func(authSessionID string) error {

		// Authenticate with password as a prerequisite to update PIN.
		if _, err := client.AuthenticateAuthFactor(ctx, authSessionID, passwordLabel, userPassword); err != nil {
			return errors.Wrap(err, "failed to authenticate with password AuthFactor")
		}
		if err := client.UpdatePinAuthFactor(ctx, authSessionID, pinLabel, userPinNew); err != nil {
			return errors.Wrap(err, "failed to update PIN AuthFactor")
		}
		// Authenticate with old PIN should fail.
		if _, err := client.AuthenticatePinAuthFactor(ctx, authSessionID, pinLabel, userPin); err == nil {
			return errors.Wrap(err, "authenticating with the old PIN succeeded but should have failed")
		}
		// Authenticate with updated PIN.
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

	// 2.5 Test removing PIN AuthFactor after migration.
	if err := client.WithAuthSession(ctx, userName, false /*isEphemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT, func(authSessionID string) error {

		// Authenticate with password AuthFactor to remove the PIN factor.
		if _, err := client.AuthenticateAuthFactor(ctx, authSessionID, passwordLabel, userPassword); err != nil {
			return errors.Wrap(err, "failed to authenticate with password AuthFactor")
		}
		if err := client.RemoveAuthFactor(ctx, authSessionID, pinLabel); err != nil {
			return errors.Wrap(err, "removing the updated PIN failed")
		}
		// Test that user data is preserved after the migrated PIN is removed.
		if err := cryptohome.MountAndVerify(ctx, userName, authSessionID, false /*ecryptfs*/); err != nil {
			return errors.Wrap(err, "failed to mount and verify persistence after password migration")
		}
		defer client.Unmount(ctxForCleanup, userName)
		// Test that lock screen works after the migrated PIN is removed.
		if err := cryptohome.TestLockScreen(ctx, userName, userPassword, wrongPassword, passwordLabel, client); err != nil {
			return errors.Wrap(err, "lock screen test with password failed after USS migration of password")
		}
		// Check that the PIN AuthFactor file is deleted.
		if err := cryptohome.CheckKeyBackingStoreExists(ctx, pinAuthFactorFile, userName); err == nil {
			return errors.Wrap(err, "PIN AuthFactor file exists, should have been deleted")
		}
		// Check that the USS file is not deleted.
		if err := cryptohome.CheckKeyBackingStoreExists(ctx, ussFile, userName); err != nil {
			return errors.Wrap(err, "failed to check USS file after removing PIN factor")
		}
		// Authenticate with the removed factor should now fail.
		if _, err := client.AuthenticatePinAuthFactor(ctx, authSessionID, pinLabel, userPinNew); err == nil {
			return errors.Wrap(err, "authenticating with PIN should have failed after removal, but succeeded")
		}
		// Invalidate AuthSession and unmount user directories.
		if err := client.CleanupSession(ctx, authSessionID); err != nil {
			return errors.Wrap(err, "failed to cleanup user AuthSession")
		}
		return nil
	}); err != nil {
		s.Fatal("Failed to test remove PIN AuthFactor after migration: ", err)
	}
}
