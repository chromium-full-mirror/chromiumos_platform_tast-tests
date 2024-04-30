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

	uda "go.chromium.org/chromiumos/system_api/user_data_auth_proto"

	cryptohomecommon "go.chromium.org/tast-tests/cros/common/cryptohome"
	"go.chromium.org/tast-tests/cros/common/hwsec"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	hwseclocal "go.chromium.org/tast-tests/cros/local/hwsec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type updateRecoveryTestParam struct {
	rotateRecoveryID  bool
	rotateMediatorKey bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func: UpdateRecovery,
		Desc: "Update recovery auth factor and authenticate again",
		Contacts: []string{
			"cryptohome-core@google.com",
			"cros-lurs@google.com",
			"iscsi@google.com",
		},
		BugComponent: "b:1148604", // ChromeOS > Security > Cryptohome > Cryptohome Recovery
		Attr:         []string{"group:mainline", "group:cryptohome"},
		// For "no_tpm_dynamic" - see http://b/251789202.
		SoftwareDeps: []string{"tpm", "no_tpm_dynamic"},
		Fixture:      "ussAuthSessionFixture",
		Params: []testing.Param{{
			Val: updateRecoveryTestParam{
				// Regular use case - we rotate recovery id after successful authentication,
				// but the mediator keys remain the same.
				rotateRecoveryID:  true,
				rotateMediatorKey: false,
			},
		}, {
			Name: "no_id_rotation",
			Val: updateRecoveryTestParam{
				rotateRecoveryID:  false,
				rotateMediatorKey: false,
			},
		}, {
			Name: "rotate_mediator_keys",
			Val: updateRecoveryTestParam{
				rotateRecoveryID:  false,
				rotateMediatorKey: true,
			},
		}},
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
		// Hardcoded fake mediator and epoch public and private keys. Do not use them in production!
		// Keys were generated at random using EllipticCurve::GenerateKeysAsSecureBlobs method and converted to hex.
		mediatorPubKey1Hex  = "3059301306072a8648ce3d020106082a8648ce3d03010703420004bcc6e3b259a79f8e31eb3f31801c4b2f9bf91c1bee253b9f6c8f8cbf1767b6908223a4c9ed6c671eb5d2715537d5b7c4bf9647f6fc5c64d1b11a26f5dd46e5d8"
		mediatorPrivKey1Hex = "dfce2cfba5986cc03b0b5e03a8b8e553d7a4efe3b330ad87eecef7e508d4e23f"
		mediatorPubKey2Hex  = "3059301306072a8648ce3d020106082a8648ce3d030107034200043b36a784ec23f118c23974af3cf2b3ed92a9ea0c0ac34f1fced904f88b6bbf2e7f6941b6ecd4814b855be96602472da5ea19e7d89e19dc6af0a993a0b4e814c6"
		mediatorPrivKey2Hex = "e41746214b760a4b59e41e79ef6e4250fb053e6566e5660619938e2e78024c63"
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

	testTool, err := cryptohomecommon.NewRecoveryTestTool(cmdRunner)
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

	authenticateWithRecovery := func(mediatorPrivKey string) (string, error) {
		// Authenticate a new auth session via the new added recovery auth factor and mount the user.
		_, authSessionID, err = client.StartAuthSession(ctx, userName, false /*ephemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT)
		if err != nil {
			return "", errors.Wrap(err, "failed to start auth session for re-mounting")
		}

		epoch, err := testTool.FetchFakeEpochResponseHex(ctx)
		if err != nil {
			return authSessionID, errors.Wrap(err, "failed to get fake epoch response")
		}

		prepareOutput, err := client.PrepareRecoveryAuthFactor(ctx, authSessionID, recoveryLabel, epoch)
		if err != nil {
			return authSessionID, errors.Wrap(err, "failed to get recovery request")
		}
		requestHex := hex.EncodeToString(prepareOutput.RecoveryRequest)

		response, err := testTool.FakeMediateWithPrivateKey(ctx, requestHex, mediatorPrivKey)
		if err != nil {
			return authSessionID, errors.Wrap(err, "failed to mediate")
		}

		ledgerInfo, err := testTool.FetchFakeLedgerInfo(ctx)
		if err != nil {
			return authSessionID, errors.Wrap(err, "failed to get ledger info")
		}

		if err := client.AuthenticateRecoveryAuthFactor(ctx, authSessionID, recoveryLabel, epoch, response,
			ledgerInfo.Name, ledgerInfo.KeyHash, ledgerInfo.PublicKey); err != nil {
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

	// Add a recovery auth factor to the user.
	if err := client.AddRecoveryAuthFactor(ctx, authSessionID, recoveryLabel, mediatorPubKey1Hex, userGaiaID, deviceUserID); err != nil {
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
	mediatorPubKey, err := hex.DecodeString(mediatorPubKey1Hex)
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
	authSessionID, err = authenticateWithRecovery(mediatorPrivKey1Hex)
	if err != nil {
		s.Fatal("Failed to authenticate with recovery factor: ", err)
	}

	param := s.Param().(updateRecoveryTestParam)
	newMediatorPrivKeyHex := mediatorPrivKey1Hex
	newMediatorPubKeyHex := mediatorPubKey1Hex
	if param.rotateMediatorKey {
		newMediatorPrivKeyHex = mediatorPrivKey2Hex
		newMediatorPubKeyHex = mediatorPubKey2Hex
	}

	// Update recovery auth factor.
	if err := client.UpdateRecoveryAuthFactor(ctx, authSessionID, recoveryLabel /*label*/, newMediatorPubKeyHex,
		userGaiaID, deviceUserID, param.rotateRecoveryID /*ensureFreshRecoveryID*/); err != nil {
		s.Fatal("Failed to update recovery factor: ", err)
	}

	newRecoveryIDs, err := client.FetchRecoveryIDs(ctx, userName, recoveryLabel)
	if err != nil {
		s.Fatal("Failed to get recovery ids after update: ", err)
	}
	recoveryID := recoveryIDs[0]
	if param.rotateRecoveryID {
		// Recovery id was rotated, we expect to have the list of 2 items: [new_id, previous_id]
		if len(newRecoveryIDs) != 2 {
			s.Fatalf("Got %v recovery IDs, expected 2", len(newRecoveryIDs))
		}
		newRecoveryID := newRecoveryIDs[0]
		previousRecoveryID := newRecoveryIDs[1]
		if newRecoveryID == recoveryID {
			s.Fatalf("Recovery ID is still %s, expected changed ID", newRecoveryID)
		}
		if previousRecoveryID != recoveryID {
			s.Fatalf("Old recovery ID changed, the value was %s and now %s ", recoveryID, previousRecoveryID)
		}
	} else {
		// Recovery id was not rotated, we expect to have the list of 1 item: [previous_id]
		if len(recoveryIDs) != 1 {
			s.Fatalf("Got %v recovery IDs, expected 1", len(recoveryIDs))
		}
		newRecoveryID := newRecoveryIDs[0]
		if newRecoveryID != recoveryID {
			s.Fatalf("Recovery ID is changed to %s, expected %s", newRecoveryID, recoveryID)
		}
	}

	// Unmount the user.
	if err := client.UnmountAll(ctx); err != nil {
		s.Fatal("Failed to unmount vaults for re-mounting: ", err)
	}

	// Successfully authenticate with recovery factor.
	if _, err := authenticateWithRecovery(newMediatorPrivKeyHex); err != nil {
		s.Fatal("Failed to authenticate with recovery factor after update: ", err)
	}

	// Confirm that recovery factor metadata has correct public key.
	newMediatorPubKey, err := hex.DecodeString(newMediatorPubKeyHex)
	if err != nil {
		s.Fatal("Failed to decode mediator pub key: ", err)
	}
	newActualPubKey := mediatorPubKeyFromMetadata()
	if !reflect.DeepEqual(newActualPubKey, newMediatorPubKey) {
		s.Fatalf("Incorrect new mediator pub key in recovery metadata; got %v, expected %v", newActualPubKey, newMediatorPubKey)
	}
}
