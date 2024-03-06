// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cryptohome

import (
	"context"
	"os"

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
		Func: RecoveryLockedForAuth,
		Desc: "Test that authenticate recovery factor is disabled if it is locked, but the update recovery factor still works",
		Contacts: []string{
			"cryptohome-core@google.com",
			"betuls@chromium.org",
		},
		BugComponent: "b:1148604", // ChromeOS > Security > Cryptohome > Cryptohome Recovery
		Attr:         []string{"group:mainline", "group:cryptohome", "informational"},
		// For "no_tpm_dynamic" - see http://b/251789202.
		SoftwareDeps: []string{"pinweaver", "tpm", "no_tpm_dynamic"},
		Fixture:      "ussAuthSessionFixture",
	})
}

func RecoveryLockedForAuth(ctx context.Context, s *testing.State) {
	const (
		userPassword         = "secret"
		passwordLabel        = "online-password"
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

	// Create the user with a password and recovery factor and test recovery lock mechanism.
	if err := client.WithAuthSession(ctx, userName, false /*ephemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT, func(authSessionID string) error {

		// Set up the user with a password auth factor.
		if err := client.CreatePersistentUser(ctx, authSessionID); err != nil {
			return errors.Wrap(err, "failed to create persistent user")
		}
		if err := client.AddAuthFactor(ctx, authSessionID, passwordLabel, userPassword); err != nil {
			return errors.Wrap(err, "failed to add a password authfactor")
		}

		// Setup CryptohomeRecovery AuthFactor for the user.
		mediatorPubKey, err := testTool.FetchFakeMediatorPubKeyHex(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get mediator pub key")
		}
		if err := client.AddRecoveryAuthFactor(ctx, authSessionID, recoveryLabel, mediatorPubKey, recoveryUserGaiaID, recoveryDeviceUserID); err != nil {
			return errors.Wrap(err, "failed to add a recovery auth factor")
		}

		// Authenticate using recovery.
		epoch, err := testTool.FetchFakeEpochResponseHex(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get fake epoch response")
		}
		requestHex, err := client.FetchRecoveryRequest(ctx, authSessionID, recoveryLabel, epoch)
		if err != nil {
			return errors.Wrap(err, "failed to get recovery request")
		}
		response, err := testTool.FakeMediate(ctx, requestHex)
		if err != nil {
			return errors.Wrap(err, "failed to mediate")
		}
		ledgerInfo, err := testTool.FetchFakeLedgerInfo(ctx)
		if err != nil {
			s.Fatal("Failed to get ledger info: ", err)
		}
		if err := client.AuthenticateRecoveryAuthFactor(ctx, authSessionID, recoveryLabel, epoch, response, ledgerInfo.Name, ledgerInfo.KeyHash, ledgerInfo.PublicKey); err != nil {
			return errors.Wrap(err, "failed to authenticate recovery auth factor")
		}

		// Lock recovery factor for authentication.
		if err := client.LockRecoveryFactorUntilReboot(ctx); err != nil {
			return errors.Wrap(err, "failed to lock recovery auth factor")
		}
		// Locking the recovery factor is through a flag file. Remove the flag file on exit so that CryptohomeRecovery is unlocked.
		lockPath := "/run/cryptohome/crd_detected_on_login_screen"
		defer os.Remove(lockPath)

		// Authenticate with recovery should fail now.
		if err := client.AuthenticateRecoveryAuthFactor(ctx, authSessionID, recoveryLabel, epoch, response, ledgerInfo.Name, ledgerInfo.KeyHash, ledgerInfo.PublicKey); err == nil {
			return errors.Wrap(err, "authenticate recovery auth factor should have failed, but succeeded")
		}

		// Update recovery auth factor should still work.
		if err := client.UpdateRecoveryAuthFactor(ctx, authSessionID, recoveryLabel /*label*/, mediatorPubKey,
			recoveryUserGaiaID, recoveryDeviceUserID, false /*ensureFreshRecoveryID*/); err != nil {
			return errors.Wrap(err, "failed to update recovery factor")
		}
		return nil
	}); err != nil {
		s.Fatal("Failed to check the Recovery lockout mechanism when CRD is detected on login screen: ", err)
	}
}
