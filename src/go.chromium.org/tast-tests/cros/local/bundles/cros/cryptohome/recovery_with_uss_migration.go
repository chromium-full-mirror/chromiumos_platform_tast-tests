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
	cryptochrome "go.chromium.org/tast-tests/cros/local/cryptohome/chrome"
	hwseclocal "go.chromium.org/tast-tests/cros/local/hwsec"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         RecoveryWithUSSMigration,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test addition and authentication of recovery auth factor with only password migration to USS",
		Contacts: []string{
			"cryptohome-core@google.com",
			"hardikgoyal@chromium.org",
		},
		BugComponent: "b:1148604", // ChromeOS > Security > Cryptohome > Cryptohome Recovery
		Attr:         []string{"informational", "group:mainline"},
		// For "no_tpm_dynamic" - see http://b/251789202.
		SoftwareDeps: []string{"pinweaver", "tpm", "no_tpm_dynamic", "chrome"},
	})
}

func RecoveryWithUSSMigration(ctx context.Context, s *testing.State) {
	const (
		userName             = "foo@bar.baz"
		userPassword         = "secret"
		userNewPassword      = "don't-forget-this-one"
		passwordLabel        = "online-password"
		passwordFactorFile   = "/auth_factors/password.online-password"
		userPin              = "123456"
		notUserPin           = "111111"
		pinLabel             = "pin"
		pinFactorFile        = "/auth_factors/pin.pin"
		recoveryLabel        = "test-recovery"
		recoveryUserGaiaID   = "123456789"
		recoveryDeviceUserID = "123-456-AA-BB"
		ussFile              = "/user_secret_stash/uss.0"
	)

	ctxForCleanUp := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cmdRunner := hwseclocal.NewCmdRunner()
	client := hwsec.NewCryptohomeClient(cmdRunner)
	helper, err := hwseclocal.NewHelper(cmdRunner)
	if err != nil {
		s.Fatal("Failed to create hwsec local helper: ", err)
	}

	// Clean up obsolete state, in case there's any.
	if err := client.UnmountAll(ctx); err != nil {
		s.Fatal("Failed to unmount vaults for preparation: ", err)
	}
	if err := cryptohome.RemoveVault(ctx, userName); err != nil {
		s.Fatal("Failed to remove old vault for preparation: ", err)
	}

	// Setup the recovery test tool and fakes.
	testTool, err := cryptohome.NewRecoveryTestToolWithFakeMediator()
	if err != nil {
		s.Fatal("Failed to initialize RecoveryTestTool: ", err)
	}
	defer func(s *testing.State, testTool *cryptohome.RecoveryTestTool) {
		if err := testTool.RemoveDir(); err != nil {
			s.Error("Failed to remove dir: ", err)
		}
	}(s, testTool)

	mediatorPubKey, err := testTool.FetchFakeMediatorPubKeyHex(ctx)
	if err != nil {
		s.Fatal("Failed to get mediator pub key: ", err)
	}

	// Set up an auth factor with USS migration disabled.
	if err := cryptochrome.WithUssMigration(ctx, false /*enabled*/, func() error {
		// Put the system into USS disabled mode, to ensure we get VK credentials.
		disableUssCleanup, err := helper.DisableUserSecretStash(ctx)
		if err != nil {
			return errors.Wrap(err, "unable to disable USS before creating credentials")
		}
		defer disableUssCleanup(ctxForCleanUp)

		setupUser := func(authSessionID string) error {
			// Set up the user with a password and PIN auth factor.
			if err := client.CreatePersistentUser(ctx, authSessionID); err != nil {
				return errors.Wrap(err, "failed to create persistent user")
			}
			if _, err := client.PreparePersistentVault(ctx, authSessionID /*ecryptfs=*/, false); err != nil {
				return errors.Wrap(err, "failed to prepare new persistent vault")
			}
			if err := client.AddAuthFactor(ctx, authSessionID, passwordLabel, userPassword); err != nil {
				return errors.Wrap(err, "failed to add a password authfactor")
			}
			if err := client.AddPinAuthFactor(ctx, authSessionID, pinLabel, userPin); err != nil {
				return errors.Wrap(err, "failed to add a pin authfactor")
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
		}

		// Create and mount the persistent user.
		if err := client.WithAuthSession(ctx, userName, false /*ephemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT, setupUser); err != nil {
			return errors.Wrap(err, "failed to create and set up the user")
		}

		// Unmount all user vaults.
		if err := cryptohome.UnmountVault(ctx, userName); err != nil {
			return errors.Wrap(err, "failed to unmount vault after pre-migration mount")
		}

		// Check that password factor has not been migrated.
		if err := cryptohome.CheckKeyBackingStoreExists(ctx, passwordFactorFile, userName); err == nil {
			return errors.New("Password auth factor file was created before migration should have happened")
		}
		return nil
	}); err != nil {
		s.Fatal("Setup while USS migration was disabled failed: ", err)
	}
	defer cryptohome.RemoveVault(ctxForCleanUp, userName)

	// Switch cryptohome into USS mode.
	enableUssCleanup, err := helper.EnableUserSecretStash(ctx)
	if err != nil {
		s.Fatal("Unable to enable USS after creating credentials: ", err)
	}
	defer enableUssCleanup(ctxForCleanUp)

	// Enable migration to verify the migration process.
	if err := cryptochrome.WithUssMigration(ctx, true /*enabled*/, func() error {

		authenticateAndMount := func(authSessionID string) error {
			if _, err := client.AuthenticateAuthFactor(ctx, authSessionID, passwordLabel, userPassword); err != nil {
				return errors.Wrap(err, "failed to authenticate password auth factor")
			}
			if err := cryptohome.MountAndVerify(ctx, userName, authSessionID, false /*ecryptfs*/); err != nil {
				return errors.Wrap(err, "failed to mount and verify persistence")
			}
			return nil
		}

		// Start a new auth session and mount the persistent vault.
		// This should do migration of password.
		if err := client.WithAuthSession(ctx, userName, false /*ephemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT, authenticateAndMount); err != nil {
			return errors.Wrap(err, "failed to authenticate and mount the user vault with migration")
		}

		// Check that password factor has now been migrated. There should be both a USS
		// and a file for the password auth factor.
		if err := cryptohome.CheckKeyBackingStoreExists(ctx, ussFile, userName); err != nil {
			return errors.Wrap(err, "USS file was not created")
		}

		if err := cryptohome.CheckKeyBackingStoreExists(ctx, passwordFactorFile, userName); err != nil {
			return errors.Wrap(err, "password auth factor file was not created")
		}

		addRecovery := func(authSessionID string) error {
			if _, err := client.AuthenticateAuthFactor(ctx, authSessionID, passwordLabel, userPassword); err != nil {
				return errors.Wrap(err, "failed to authenticate password auth factor")
			}

			// Add a recovery auth factor to the user.
			if err := client.AddRecoveryAuthFactor(ctx, authSessionID, recoveryLabel, mediatorPubKey, recoveryUserGaiaID, recoveryDeviceUserID); err != nil {
				return errors.Wrap(err, "failed to add a recovery auth factor")
			}

			return nil
		}

		// Start a new auth session and add recovery.
		if err := client.WithAuthSession(ctx, userName, false /*ephemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT, addRecovery); err != nil {
			return errors.Wrap(err, "failed to authenticate and mount the user vault with migration")
		}

		// Unmount user vault.
		if err := cryptohome.UnmountVault(ctx, userName); err != nil {
			return errors.Wrap(err, "failed to unmount vault after migration mount")
		}
		return nil
	}); err != nil {
		s.Fatal("Validation during USS migration failed: ", err)
	}

	// Use Recovery and update password.
	if err := cryptochrome.WithUssMigration(ctx, true /*enabled*/, func() error {
		performRecoveryAndUpdatePassword := func(authSessionID string) error {
			epoch, err := testTool.FetchFakeEpochResponseHex(ctx)
			if err != nil {
				return errors.Wrap(err, "failed to get fake epoch response")
			}

			requestHex, err := client.FetchRecoveryRequest(ctx, authSessionID, recoveryLabel, epoch)
			if err != nil {
				return errors.Wrap(err, "failed to get recovery request")
			}

			response, err := testTool.FakeMediateWithRequest(ctx, requestHex)
			if err != nil {
				return errors.Wrap(err, "failed to mediate")
			}

			ledgerInfo, err := testTool.FetchFakeLedgerInfo(ctx)
			if err != nil {
				s.Fatal("Failed to get ledger info: ", err)
			}

			// Authenticate using recovery.
			if err := client.AuthenticateRecoveryAuthFactor(ctx, authSessionID, recoveryLabel, epoch, response, ledgerInfo.Name, ledgerInfo.KeyHash, ledgerInfo.PublicKey); err != nil {
				return errors.Wrap(err, "failed to authenticate recovery auth factor")
			}

			if err := cryptohome.MountAndVerify(ctx, userName, authSessionID, false /*ecryptfs*/); err != nil {
				return errors.Wrap(err, "failed to mount and verify persistence")
			}
			defer client.UnmountAll(ctx)

			// Update the password auth factor.
			if err := client.UpdatePasswordAuthFactor(ctx, authSessionID, passwordLabel, passwordLabel, userNewPassword); err != nil {
				return errors.Wrap(err, "failed to update user password after recovery")
			}

			// Authenticating with the old password should fail.
			if _, err := client.AuthenticateAuthFactor(ctx, authSessionID, passwordLabel, userPassword); err == nil {
				return errors.New("was incorrectly able to authenticate with the old password after changing it during recovery")
			}
			return nil
		}
		// Authenticate a new auth session via the new added recovery auth factor and mount the user.
		if err := client.WithAuthSession(ctx, userName, false /*ephemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT, performRecoveryAndUpdatePassword); err != nil {
			return errors.Wrap(err, "failed to recover the user and update password")
		}
		return nil
	}); err != nil {
		s.Fatal("Validation during USS migration failed: ", err)
	}

	if err := cryptochrome.WithUssMigration(ctx, true /*enabled*/, func() error {
		// Check that pin factor has not been migrated.
		if err := cryptohome.CheckKeyBackingStoreExists(ctx, pinFactorFile, userName); err == nil {
			return errors.New("Pin auth factor file was created before migration should have happened")
		}
		// Test that PIN reset with correct password works.
		if err := cryptohome.TestPinCounterMechanism(ctx, userName, passwordLabel, userNewPassword, pinLabel, userPin, notUserPin, client); err != nil {
			return errors.Wrap(err, "failed in testing PIN lockout and reset mechanism after recovery")
		}

		// Check that pin factor has now been migrated. There should be still be a USS
		// and a new file for the pin auth factor.
		if err := cryptohome.CheckKeyBackingStoreExists(ctx, ussFile, userName); err != nil {
			return errors.Wrap(err, "USS file was not found")
		}
		if err := cryptohome.CheckKeyBackingStoreExists(ctx, pinFactorFile, userName); err != nil {
			return errors.Wrap(err, "pin auth factor file was not created")
		}

		return nil
	}); err != nil {
		s.Fatal("Validation of inmigrated pin lockout failed post recovery: ", err)
	}

	// Test that PIN reset with correct password works after the migration.
	if err := cryptohome.TestPinCounterMechanism(ctx, userName, passwordLabel, userNewPassword, pinLabel, userPin, notUserPin, client); err != nil {
		s.Fatal("Failed in testing PIN lockout and reset mechanism after migration: ", err)
	}
}
