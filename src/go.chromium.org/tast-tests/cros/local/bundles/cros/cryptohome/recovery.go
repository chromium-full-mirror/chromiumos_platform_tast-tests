// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cryptohome

import (
	"context"
	"encoding/hex"

	uda "go.chromium.org/chromiumos/system_api/user_data_auth_proto"

	cryptohomecommon "go.chromium.org/tast-tests/cros/common/cryptohome"
	"go.chromium.org/tast-tests/cros/common/hwsec"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	hwseclocal "go.chromium.org/tast-tests/cros/local/hwsec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: Recovery,
		Desc: "Test addition and authentication of recovery auth factor with password",
		Contacts: []string{
			"cryptohome-core@google.com",
			"iscsi@google.com",
		},
		BugComponent: "b:1148604", // ChromeOS > Security > Cryptohome > Cryptohome Recovery
		Attr:         []string{"group:mainline", "group:cryptohome"},
		// For "no_tpm_dynamic" - see http://b/251789202.
		SoftwareDeps: []string{"pinweaver", "tpm", "no_tpm_dynamic"},
		Fixture:      "ussAuthSessionFixture",
	})
}

func Recovery(ctx context.Context, s *testing.State) {
	const (
		userPassword         = "secret"
		userNewPassword      = "don't-forget-this-one"
		passwordLabel        = "online-password"
		userPin              = "123456"
		userNewPin           = "654321"
		pinLabel             = "pin"
		recoveryLabel        = "test-recovery"
		recoveryUserGaiaID   = "123456789"
		recoveryDeviceUserID = "123-456-AA-BB"
	)

	fixture := s.FixtValue().(*cryptohome.AuthSessionFixture)
	userName := fixture.TestUserName

	cmdRunner := hwseclocal.NewCmdRunner()
	client := hwsec.NewCryptohomeClient(cmdRunner)

	// Setup the recovery test tool and fakes.
	testTool, err := cryptohomecommon.NewRecoveryTestTool(cmdRunner)
	if err != nil {
		s.Fatal("Failed to initialize RecoveryTestTool: ", err)
	}
	defer func(s *testing.State, testTool *cryptohomecommon.RecoveryTestTool) {
		if err := testTool.RemoveDir(); err != nil {
			s.Error("Failed to remove dir: ", err)
		}
	}(s, testTool)

	mediatorPubKey, err := testTool.FetchFakeMediatorPubKeyHex(ctx)
	if err != nil {
		s.Fatal("Failed to get mediator pub key: ", err)
	}

	// Create and mount the persistent user.
	if err := client.WithAuthSession(ctx, userName, false /*ephemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT, func(authSessionID string) error {
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

		// Add a recovery auth factor to the user.
		if err := client.AddRecoveryAuthFactor(ctx, authSessionID, recoveryLabel, mediatorPubKey, recoveryUserGaiaID, recoveryDeviceUserID); err != nil {
			return errors.Wrap(err, "failed to add a recovery auth factor")
		}

		// Unmount the user.
		if err := client.UnmountAll(ctx); err != nil {
			return errors.Wrap(err, "failed to unmount vaults for re-mounting")
		}

		return nil
	}); err != nil {
		s.Fatal("Failed to create and set up the user: ", err)
	}

	// Authenticate a new auth session via the new added recovery auth factor and mount the user.
	if err := client.WithAuthSession(ctx, userName, false /*ephemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT, func(authSessionID string) error {
		epoch, err := testTool.FetchFakeEpochResponseHex(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get fake epoch response")
		}

		prepareOutput, err := client.PrepareRecoveryAuthFactor(ctx, authSessionID, recoveryLabel, epoch)
		if err != nil {
			return errors.Wrap(err, "failed to prepare recovery request")
		}
		requestHex := hex.EncodeToString(prepareOutput.RecoveryRequest)

		response, err := testTool.FakeMediate(ctx, requestHex)
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
		if _, err := client.PreparePersistentVault(ctx, authSessionID, false /*ecryptfs*/); err != nil {
			return errors.Wrap(err, "failed to prepare persistent vault")
		}

		// Verify that the test file is still there.
		if err := cryptohome.VerifyFileForPersistence(ctx, userName); err != nil {
			return errors.Wrap(err, "failed to verify file persistence")
		}

		// Update the password and pin auth factors.
		if err := client.UpdatePasswordAuthFactor(ctx, authSessionID, passwordLabel, userNewPassword); err != nil {
			return errors.Wrap(err, "failed to update user password after recovery")
		}
		if err := client.UpdatePinAuthFactor(ctx, authSessionID, pinLabel, userNewPin); err != nil {
			return errors.Wrap(err, "failed to update user PIN after recovery")
		}

		// Remove the recovery auth factor.
		if err := client.RemoveAuthFactor(ctx, authSessionID, recoveryLabel); err != nil {
			return errors.Wrap(err, "failed to remove recovery auth factor")
		}

		// Re-authentication via recovery should fail now should fail now.
		err = client.AuthenticateRecoveryAuthFactor(ctx, authSessionID, recoveryLabel, epoch, response, ledgerInfo.Name, ledgerInfo.KeyHash, ledgerInfo.PublicKey)
		if err := cryptohomecommon.ExpectCryptohomeErrorCode(err, uda.CryptohomeErrorCode_CRYPTOHOME_ERROR_KEY_NOT_FOUND); err != nil {
			return errors.Wrap(err, "failed to get the correct error code after auth factor removal")
		}

		// Unmount the user.
		if err := client.UnmountAll(ctx); err != nil {
			return errors.Wrap(err, "failed to unmount vaults for re-mounting")
		}
		return nil
	}); err != nil {
		s.Fatal("Failed to recover the user: ", err)
	}

	// Authenticate a new auth session via their updated password.
	if err := client.WithAuthSession(ctx, userName, false /*ephemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT, func(authSessionID string) error {
		// Authenticating with the old password should fail.
		if _, err := client.AuthenticateAuthFactor(ctx, authSessionID, passwordLabel, userPassword); err == nil {
			return errors.New("was incorrectly able to authenticate with the old password after changing it during recovery")
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
		s.Fatal("Failed to authenticate the user with their new password after recovery: ", err)
	}

	// Authenticate a new auth session via their updated PIN.
	if err := client.WithAuthSession(ctx, userName, false /*ephemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT, func(authSessionID string) error {
		// Authenticating with the old PIN should fail
		if _, err := client.AuthenticatePinAuthFactor(ctx, authSessionID, pinLabel, userPin); err == nil {
			return errors.New("was incorrectly able to authenticate with the old PIN after changing it during recovery")
		}

		// Authenticate using the changed PIN.
		if _, err := client.AuthenticatePinAuthFactor(ctx, authSessionID, pinLabel, userNewPin); err != nil {
			return errors.Wrap(err, "failed to authenticate PIN auth factor")
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
		s.Fatal("Failed to authenticate the user with their new PIN after recovery: ", err)
	}
}
