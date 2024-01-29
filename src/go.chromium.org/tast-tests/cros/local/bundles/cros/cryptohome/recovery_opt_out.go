// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cryptohome

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	uda "chromiumos/system_api/user_data_auth_proto"

	cryptohomecommon "go.chromium.org/tast-tests/cros/common/cryptohome"
	"go.chromium.org/tast-tests/cros/common/hwsec"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	hwseclocal "go.chromium.org/tast-tests/cros/local/hwsec"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: RecoveryOptOut,
		Desc: `Test the opt-out of recovery auth factor. After recovery auth factor
					 was removed, it's not possible to authenticate with it even when the
					 secrets are restored on disk`,
		Contacts: []string{
			"cryptohome-core@google.com",
			"cros-lurs@google.com",
			"anastasiian@chromium.org",
		},
		BugComponent: "b:1148604", // ChromeOS > Security > Cryptohome > Cryptohome Recovery
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
			"group:cryptohome",
		},
		SoftwareDeps: []string{"pinweaver"},
		Fixture:      "ussAuthSessionFixture",
	})
}

func RecoveryOptOut(ctx context.Context, s *testing.State) {
	const (
		userName      = "foo@bar.baz"
		userPassword  = "secret"
		passwordLabel = "online-password"
		recoveryLabel = "test-recovery"
		userGaiaID    = "123456789"
		deviceUserID  = "123-456-AA-BB"
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

	// Create and mount the persistent user.
	_, authSessionID, err := client.StartAuthSession(ctx, userName /*ephemeral*/, false, uda.AuthIntent_AUTH_INTENT_DECRYPT)
	if err != nil {
		s.Fatal("Failed to start auth session: ", err)
	}
	if err := client.CreatePersistentUser(ctx, authSessionID); err != nil {
		s.Fatal("Failed to create persistent user: ", err)
	}
	defer cryptohome.RemoveVault(ctxForCleanUp, userName)
	if _, err := client.PreparePersistentVault(ctx, authSessionID /*ecryptfs*/, false); err != nil {
		s.Fatal("Failed to prepare new persistent vault: ", err)
	}
	defer client.UnmountAll(ctxForCleanUp)

	// Add a password auth factor to the user.
	if err := client.AddAuthFactor(ctx, authSessionID, passwordLabel, userPassword); err != nil {
		s.Fatal("Failed to add a password authfactor: ", err)
	}

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

	authenticateWithRecoveryFactor := func(ctx context.Context, authSessionID, label string) error {
		// Start recovery.
		epoch, err := testTool.FetchFakeEpochResponseHex(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get fake epoch response")
		}
		requestHex, err := client.FetchRecoveryRequest(ctx, authSessionID, label, epoch)
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
		return client.AuthenticateRecoveryAuthFactor(ctx, authSessionID, label, epoch, response, ledgerInfo.Name, ledgerInfo.KeyHash, ledgerInfo.PublicKey)
	}

	// Add a recovery auth factor to the user.
	if err := client.AddRecoveryAuthFactor(ctx, authSessionID, recoveryLabel, mediatorPubKey, userGaiaID, deviceUserID); err != nil {
		s.Fatal("Failed to add a recovery auth factor: ", err)
	}

	// Unmount the user.
	if err := client.UnmountAll(ctx); err != nil {
		s.Fatal("Failed to unmount vaults for re-mounting: ", err)
	}

	// Create a temp directory for backup.
	loginDataBackup, err := ioutil.TempDir("", "login_data_backup*")
	if err != nil {
		s.Fatal("Could not create a temp directory: ", err)
	}
	defer os.RemoveAll(loginDataBackup)

	// Backup the login data.
	dataPath := filepath.Join(loginDataBackup, "data.tar.gz")
	s.Log("Preparing login data of current version")
	if err := helper.SaveLoginData(ctx, dataPath, false /*includeTpm*/); err != nil {
		s.Fatal("Failed to backup login data: ", err)
	}

	// Start auth session again. Password and Recovery factors are available.
	_, authSessionID, err = client.StartAuthSession(ctx, userName, false /*ephemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT)
	if err != nil {
		s.Fatal("Failed to start auth session for re-mounting: ", err)
	}

	// Authenticate with recovery factor.
	if err := authenticateWithRecoveryFactor(ctx, authSessionID, recoveryLabel); err != nil {
		s.Fatal("Failed to authenticate with recovery auth factor: ", err)
	}

	// Remove the recovery auth factor.
	if err := client.RemoveAuthFactor(ctx, authSessionID, recoveryLabel); err != nil {
		s.Fatal("Failed to remove recovery auth factor: ", err)
	}

	// Unmount the user.
	if err := client.UnmountAll(ctx); err != nil {
		s.Fatal("Failed to unmount vaults for re-mounting: ", err)
	}

	// Restore the login data.
	if err := helper.LoadLoginData(ctx, dataPath, false /*includeTpm*/, true /*resumeDaemos*/); err != nil {
		s.Fatal("Failed to restore login data: ", err)
	}

	// Start auth session again. Password and Recovery (restored) factors are available.
	_, authSessionID, err = client.StartAuthSession(ctx, userName, false /*ephemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT)
	if err != nil {
		s.Fatal("Failed to start auth session for re-mounting: ", err)
	}

	err = authenticateWithRecoveryFactor(ctx, authSessionID, recoveryLabel)
	if err := cryptohomecommon.ExpectCryptohomeErrorCode(err, uda.CryptohomeErrorCode_CRYPTOHOME_ERROR_AUTHORIZATION_KEY_FAILED); err != nil {
		s.Fatal("Failed to get the correct error code after auth factor removal: ", err)
	}
}
