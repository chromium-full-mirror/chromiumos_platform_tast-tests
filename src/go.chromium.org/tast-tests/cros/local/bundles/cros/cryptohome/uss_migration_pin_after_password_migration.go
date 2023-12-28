// Copyright 2023 The ChromiumOS Authors
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
		Func: USSMigrationPinAfterPasswordMigration, LacrosStatus: testing.LacrosVariantUnneeded, Desc: "Checks that adding a pin works correctly when password is migrate",
		Contacts: []string{
			"cryptohome-core@google.com",
			"hardikgoyal@chromium.org",
		},
		BugComponent: "b:1088399", // ChromeOS > Security > Cryptohome
		Attr:         []string{"group:mainline"},
		SoftwareDeps: []string{"pinweaver", "tpm", "chrome"},
	})
}

func USSMigrationPinAfterPasswordMigration(ctx context.Context, s *testing.State) {
	const (
		userName           = "foo@bar.baz"
		userPassword       = "secret"
		userNewPassword    = "don't-forget-this-one"
		passwordLabel      = "online-password"
		passwordFactorFile = "/auth_factors/password.online-password"
		userPin            = "123456"
		notUserPin         = "111111"
		pinLabel           = "pin"
		pinFactorFile      = "/auth_factors/pin.pin"
		ussFile            = "/user_secret_stash/uss.0"
	)

	ctxForCleanUp := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cmdRunner := hwseclocal.NewCmdRunner()
	client := hwsec.NewCryptohomeClient(cmdRunner)

	// Clean up obsolete state, in case there's any.
	if err := client.UnmountAll(ctx); err != nil {
		s.Fatal("Failed to unmount vaults for preparation: ", err)
	}
	if err := cryptohome.RemoveVault(ctx, userName); err != nil {
		s.Fatal("Failed to remove old vault for preparation: ", err)
	}

	// Set up an auth factor with Vault Keyset backing.
	if err := func() error {
		// Create and mount the persistent user.
		if err := client.WithAuthSession(ctx, userName, false /*ephemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT, func(authSessionID string) error {
			// Set up the user with a password auth factor.
			if err := client.CreatePersistentUser(ctx, authSessionID); err != nil {
				return errors.Wrap(err, "failed to create persistent user")
			}
			if _, err := client.PreparePersistentVault(ctx, authSessionID /*ecryptfs=*/, false); err != nil {
				return errors.Wrap(err, "failed to prepare new persistent vault")
			}
			if err := client.CreateVaultKeyset(ctx, authSessionID, userPassword /*keyDataLabel=*/, passwordLabel, uda.AuthFactorType_AUTH_FACTOR_TYPE_PASSWORD /*disableKeyData=*/, false); err != nil {
				return errors.Wrap(err, "failed to create password VaultKeyset")
			}

			// Write a test file to verify persistence.
			if err := cryptohome.WriteFileForPersistence(ctx, userName); err != nil {
				return errors.Wrap(err, "failed to write test file")
			}

			// Unmount the user.
			if err := client.UnmountAll(ctx); err != nil {
				return errors.Wrap(err, "failed to unmount vaults for re-mounting")
			}

			return nil
		}); err != nil {
			return errors.Wrap(err, "failed to create and set up the user")
		}

		// Unmount all user vaults.
		if err := cryptohome.UnmountVault(ctx, userName); err != nil {
			return errors.Wrap(err, "failed to unmount vault after pre-migration mount")
		}
		return nil
	}(); err != nil {
		s.Fatal("Setup while USS migration was disabled failed: ", err)
	}
	defer cryptohome.RemoveVault(ctxForCleanUp, userName)

	// Enable migration to verify the migration process.
	if err := func() error {
		// Check that password factor has not been migrated.
		if err := cryptohome.CheckKeyBackingStoreExists(ctx, passwordFactorFile, userName); err == nil {
			return errors.New("Password auth factor file was created before migration should have happened")
		}

		// Start a new auth session and mount the persistent vault.
		// This should do migration.
		if err := client.WithAuthSession(ctx, userName, false /*ephemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT, func(authSessionID string) error {
			if _, err := client.AuthenticateAuthFactor(ctx, authSessionID, passwordLabel, userPassword); err != nil {
				return errors.Wrap(err, "failed to authenticate password auth factor")
			}
			if err := cryptohome.MountAndVerify(ctx, userName, authSessionID, false /*ecryptfs*/); err != nil {
				return errors.Wrap(err, "failed to mount and verify persistence")
			}
			return nil
		}); err != nil {
			return errors.Wrap(err, "failed to authenticate and mount the user vault with migration")
		}

		// Check that migrated Password factor has now been migrated. There should be both a USS
		// and a file for the password auth factor.
		if err := cryptohome.CheckKeyBackingStoreExists(ctx, ussFile, userName); err != nil {
			return errors.Wrap(err, "USS file was not created")
		}
		if err := cryptohome.CheckKeyBackingStoreExists(ctx, passwordFactorFile, userName); err != nil {
			return errors.Wrap(err, "password auth factor file was not created")
		}

		// Add Pin AuthFactor now.
		if err := client.WithAuthSession(ctx, userName, false /*ephemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT, func(authSessionID string) error {
			if _, err := client.AuthenticateAuthFactor(ctx, authSessionID, passwordLabel, userPassword); err != nil {
				return errors.Wrap(err, "failed to authenticate password auth factor")
			}

			if err := client.AddPinAuthFactor(ctx, authSessionID, pinLabel, userPin); err != nil {
				return errors.Wrap(err, "failed to add a pin authfactor")
			}

			if err := cryptohome.CheckKeyBackingStoreExists(ctx, pinFactorFile, userName); err != nil {
				return errors.Wrap(err, "pin auth factor file was not created")
			}
			return nil
		}); err != nil {
			return errors.Wrap(err, "failed to authenticate and mount the user vault with migration")
		}

		// Unmount user vault.
		if err := cryptohome.UnmountVault(ctx, userName); err != nil {
			return errors.Wrap(err, "failed to unmount vault after migration mount")
		}
		return nil
	}(); err != nil {
		s.Fatal("Validation during USS migration failed: ", err)
	}

	if err := func() error {
		// Start a new auth session and mount the persistent vault and update auth factor.
		if err := client.WithAuthSession(ctx, userName, false /*ephemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT, func(authSessionID string) error {
			if _, err := client.AuthenticateAuthFactor(ctx, authSessionID, passwordLabel, userPassword); err != nil {
				return errors.Wrap(err, "failed to authenticate password auth factor")
			}
			if err := cryptohome.MountAndVerify(ctx, userName, authSessionID, false /*ecryptfs*/); err != nil {
				return errors.Wrap(err, "failed to mount and verify persistence")
			}

			// Update the password auth factor.
			if err := client.UpdatePasswordAuthFactor(ctx, authSessionID, passwordLabel, passwordLabel, userNewPassword); err != nil {
				return errors.Wrap(err, "failed to update user password after migration")
			}
			return nil
		}); err != nil {
			return errors.Wrap(err, "failed to authenticate and mount the user vault with migration")
		}

		// Unmount user vault.
		if err := cryptohome.UnmountVault(ctx, userName); err != nil {
			return errors.Wrap(err, "failed to unmount vault after migration mount")
		}
		return nil
	}(); err != nil {
		s.Fatal("Validation during USS migration failed: ", err)
	}

	// Authenticate a new auth session via the wrong Pin five times - lock out, authenticate with correct password then.
	if err := client.WithAuthSession(ctx, userName, false /*ephemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT, func(authSessionID string) error {

		for i := 0; i < 5; /*no of wrong attempts allowed*/ i++ {
			// Authenticating with the wrong PIN should fail.
			if _, err := client.AuthenticatePinAuthFactor(ctx, authSessionID, pinLabel, notUserPin); err == nil {
				return errors.New("was incorrectly able to authenticate with the wrong PIN")
			}
		}
		// Authenticating with the old password should fail.
		if _, err := client.AuthenticateAuthFactor(ctx, authSessionID, passwordLabel, userPassword); err == nil {
			return errors.New("was incorrectly able to authenticate with the old password")
		}

		// Authenticating with the correct Pin should also fail as it is locked out.
		if _, err := client.AuthenticatePinAuthFactor(ctx, authSessionID, pinLabel, userPin); err == nil {
			return errors.New("was incorrectly able to authenticate with the correct PIN when it is locked out")
		}

		// Authenticate using the changed password.
		if _, err := client.AuthenticateAuthFactor(ctx, authSessionID, passwordLabel, userNewPassword); err != nil {
			return errors.Wrap(err, "failed to authenticate password auth factor")
		}

		if _, err := client.PreparePersistentVault(ctx, authSessionID, false /*ecryptfs*/); err != nil {
			return errors.Wrap(err, "failed to prepare persistent vault")
		}

		// Verify that the test file is still there.
		if err := cryptohome.VerifyFileForPersistence(ctx, userName); err != nil {
			return errors.Wrap(err, "failed to verify file persistence")
		}

		// Unmount the user.
		if err := client.UnmountAll(ctx); err != nil {
			return errors.Wrap(err, "failed to unmount vaults for re-mounting")
		}

		return nil
	}); err != nil {
		s.Fatal("Failed to lock pin and then unlock with it new password: ", err)
	}

	// Enable migration to verify the migration process.
	if err := func() error {
		// Authenticate a new auth session via their old PIN and verify migration.
		if err := client.WithAuthSession(ctx, userName, false /*ephemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT, func(authSessionID string) error {
			// Authenticating with the old PIN should pass.
			if _, err := client.AuthenticatePinAuthFactor(ctx, authSessionID, pinLabel, userPin); err != nil {
				return errors.New("was not able to authenticate with the old PIN")
			}
			if _, err := client.PreparePersistentVault(ctx, authSessionID, false /*ecryptfs*/); err != nil {
				return errors.Wrap(err, "failed to prepare persistent vault")
			}

			// Verify that the test file is still there.
			if err := cryptohome.VerifyFileForPersistence(ctx, userName); err != nil {
				return errors.Wrap(err, "failed to verify file persistence")
			}

			// Unmount the user.
			if err := client.UnmountAll(ctx); err != nil {
				return errors.Wrap(err, "failed to unmount vaults for re-mounting")
			}

			return nil
		}); err != nil {
			return errors.Wrap(err, "failed to authenticate user with old pin after lockout")
		}
		return nil
	}(); err != nil {
		s.Fatal("Validation of pin lockout failed during migration: ", err)
	}
}
