// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cryptohome

import (
	"bytes"
	"context"
	"encoding/hex"
	"io/ioutil"
	"path/filepath"
	"reflect"

	uda "chromiumos/system_api/user_data_auth_proto"

	cryptohomecommon "go.chromium.org/tast-tests/cros/common/cryptohome"
	"go.chromium.org/tast-tests/cros/common/hwsec"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	hwseclocal "go.chromium.org/tast-tests/cros/local/hwsec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: UpdateRecovery,
		Desc: "Update recovery auth factor and authenticate again",
		Contacts: []string{
			"cryptohome-core@google.com",
			"cros-lurs@google.com",
			"anastasiian@chromium.org",
		},
		BugComponent: "b:1148604", // ChromeOS > Security > Cryptohome > Cryptohome Recovery
		Attr:         []string{"group:mainline", "group:cryptohome"},
		// For "no_tpm_dynamic" - see http://b/251789202.
		SoftwareDeps: []string{"tpm", "no_tpm_dynamic"},
		Fixture:      "ussAuthSessionFixture",
	})
}

func UpdateRecovery(ctx context.Context, s *testing.State) {
	const (
		userPassword    = "secret"
		passwordLabel   = "online-password"
		recoveryLabel   = "test-recovery"
		testFile        = "file"
		testFileContent = "content"
		shadow          = "/home/.shadow"
		userGaiaID      = "123456789"
		deviceUserID    = "123-456-AA-BB"
	)

	fixture := s.FixtValue().(*cryptohome.AuthSessionFixture)
	userName := fixture.TestUserName

	cmdRunner := hwseclocal.NewCmdRunner()
	client := hwsec.NewCryptohomeClient(cmdRunner)

	// Create and mount the persistent user.
	_, authSessionID, err := client.StartAuthSession(ctx, userName /*ephemeral=*/, false, uda.AuthIntent_AUTH_INTENT_DECRYPT)
	if err != nil {
		s.Fatal("Failed to start auth session: ", err)
	}
	if err := client.CreatePersistentUser(ctx, authSessionID); err != nil {
		s.Fatal("Failed to create persistent user: ", err)
	}
	if _, err := client.PreparePersistentVault(ctx, authSessionID /*ecryptfs=*/, false); err != nil {
		s.Fatal("Failed to prepare persistent vault: ", err)
	}

	// Write a test file to verify persistence.
	userPath, err := cryptohome.UserPath(ctx, userName)
	if err != nil {
		s.Fatal("Failed to get user vault path: ", err)
	}
	filePath := filepath.Join(userPath, testFile)
	if err := ioutil.WriteFile(filePath, []byte(testFileContent), 0644); err != nil {
		s.Fatal("Failed to write a file to the vault: ", err)
	}

	testTool, err := cryptohomecommon.NewRecoveryTestToolWithFakeMediator(cmdRunner)
	if err != nil {
		s.Fatal("Failed to initialize RecoveryTestTool: ", err)
	}
	defer func(s *testing.State, testTool *cryptohomecommon.RecoveryTestTool) {
		if err := testTool.RemoveDir(); err != nil {
			s.Error("Failed to remove dir: ", err)
		}
	}(s, testTool)

	mediatorPubKeyFromMetadata := func() []byte {
		listFactors, err := client.ListAuthFactors(ctx, userName)
		if err != nil {
			s.Fatal("Failed to list auth factors: ", err)
			return []byte{}
		}
		for _, factor := range listFactors.ConfiguredAuthFactorsWithStatus {
			if factor.AuthFactor.Type == uda.AuthFactorType_AUTH_FACTOR_TYPE_CRYPTOHOME_RECOVERY {
				return factor.AuthFactor.GetCryptohomeRecoveryMetadata().MediatorPubKey
			}
		}
		s.Fatal("Failed to find recovery factor")
		return []byte{}
	}

	authenticateWithRecovery := func() (string, error) {
		// Authenticate a new auth session via the new added recovery auth factor and mount the user.
		_, authSessionID, err = client.StartAuthSession(ctx, userName, false /*ephemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT)
		if err != nil {
			return "", errors.Wrap(err, "failed to start auth session for re-mounting")
		}

		epoch, err := testTool.FetchFakeEpochResponseHex(ctx)
		if err != nil {
			return authSessionID, errors.Wrap(err, "failed to get fake epoch response")
		}

		requestHex, err := client.FetchRecoveryRequest(ctx, authSessionID, recoveryLabel, epoch)
		if err != nil {
			return authSessionID, errors.Wrap(err, "failed to get recovery request")
		}

		response, err := testTool.FakeMediateWithRequest(ctx, requestHex)
		if err != nil {
			return authSessionID, errors.Wrap(err, "failed to mediate")
		}

		ledgerInfo, err := testTool.FetchFakeLedgerInfo(ctx)
		if err != nil {
			return authSessionID, errors.Wrap(err, "failed to get ledger info")
		}

		if err := client.AuthenticateRecoveryAuthFactor(ctx, authSessionID, recoveryLabel, epoch, response, ledgerInfo.Name, ledgerInfo.KeyHash, ledgerInfo.PublicKey); err != nil {
			return authSessionID, errors.Wrap(err, "failed to authenticate recovery auth factor")
		}
		if _, err := client.PreparePersistentVault(ctx, authSessionID, false /*ecryptfs*/); err != nil {
			return authSessionID, errors.Wrap(err, "failed to prepare persistent vault")
		}

		// Verify that the test file is still there.
		if content, err := ioutil.ReadFile(filePath); err != nil {
			return authSessionID, errors.Wrap(err, "failed to read back test file")
		} else if bytes.Compare(content, []byte(testFileContent)) != 0 {
			return authSessionID, errors.Errorf("incorrect tests file content. got: %q, want: %q", content, testFileContent)
		}
		return authSessionID, nil
	}

	// Add a password auth factor to the user.
	if err := client.AddAuthFactor(ctx, authSessionID, passwordLabel, userPassword); err != nil {
		s.Fatal("Failed to create persistent user: ", err)
	}

	mediatorPubKeyHex, err := testTool.FetchFakeMediatorPubKeyHex(ctx)
	if err != nil {
		s.Fatal("Failed to get mediator pub key: ", err)
	}

	// Add a recovery auth factor to the user.
	if err := client.AddRecoveryAuthFactor(ctx, authSessionID, recoveryLabel, mediatorPubKeyHex, userGaiaID, deviceUserID); err != nil {
		s.Fatal("Failed to add a recovery auth factor: ", err)
	}

	// Confirm that a recovery ID was created.
	recoveryIDs, err := client.FetchRecoveryIDs(ctx, userName, recoveryLabel)
	if err != nil {
		s.Fatal("Failed to get recovery ids: ", err)
	}
	if len(recoveryIDs) != 1 {
		s.Fatalf("Got %v recovery IDs, expected 1", len(recoveryIDs))
	}

	// Confirm that recovery factor metadata has correct public key.
	mediatorPubKey, err := hex.DecodeString(mediatorPubKeyHex)
	if err != nil {
		s.Fatal("Failed to decode mediator pub key: ", err)
	}
	actualPubKey := mediatorPubKeyFromMetadata()
	if !reflect.DeepEqual(actualPubKey, mediatorPubKey) {
		s.Fatalf("Incorrect mediator pub key in recovery metadata; got %v, expected %v", actualPubKey, mediatorPubKey)
	}

	// Unmount the user.
	if err := client.UnmountAll(ctx); err != nil {
		s.Fatal("Failed to unmount vaults for re-mounting: ", err)
	}

	// Successfully authenticate with recovery.
	authSessionID, err = authenticateWithRecovery()
	if err != nil {
		s.Fatal("Failed to authenticate with recovery factor: ", err)
	}

	// Update recovery auth factor.
	if err := client.UpdateRecoveryAuthFactor(ctx, authSessionID, recoveryLabel /*label*/, mediatorPubKeyHex, userGaiaID, deviceUserID); err != nil {
		s.Fatal("Failed to update recovery factor: ", err)
	}

	newRecoveryIDs, err := client.FetchRecoveryIDs(ctx, userName, recoveryLabel)
	if err != nil {
		s.Fatal("Failed to get recovery ids after update: ", err)
	}
	if len(newRecoveryIDs) != 2 {
		s.Fatalf("Got %v recovery IDs, expected 2", len(newRecoveryIDs))
	}
	if newRecoveryIDs[0] == recoveryIDs[0] {
		s.Fatalf("Recovery ID is still %s, expected changed ID", newRecoveryIDs[0])
	}
	if newRecoveryIDs[1] != recoveryIDs[0] {
		s.Fatalf("Old recovery ID changed, the value was %s and now %s ", recoveryIDs[0], newRecoveryIDs[1])
	}

	// Unmount the user.
	if err := client.UnmountAll(ctx); err != nil {
		s.Fatal("Failed to unmount vaults for re-mounting: ", err)
	}

	// Successfully authenticate with recovery factor.
	if _, err := authenticateWithRecovery(); err != nil {
		s.Fatal("Failed to authenticate with recovery factor after update: ", err)
	}

	// TODO(b/289178330): Update recovery auth factor with a different public key;
	// confirm that recovery factor metadata has a new public key.
}
