// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cryptohome

import (
	"context"

	"google.golang.org/protobuf/proto"

	"chromiumos/system_api/cryptohome_proto"
	uda "chromiumos/system_api/user_data_auth_proto"

	"go.chromium.org/tast-tests/cros/common/hwsec"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	cryptochrome "go.chromium.org/tast-tests/cros/local/cryptohome/chrome"
	hwseclocal "go.chromium.org/tast-tests/cros/local/hwsec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: RecoverableKeyStores,
		// Due to the fact that we don't control server side certificate list update, and that it is
		// complicated to set up test support (like fake CAs, fake certificates), this test doesn't
		// cover behaviors caused by certificate list updates. There is a very small chance that a
		// server side certificate list update during the test might make the test run fail. It is
		// possible to add support to stop the certificate fetch routines during the test to prevent
		// this, but in practice it doesn't seem necessary.
		Desc:         "Test generation and update of recoverable key stores caused by adding, updating, or removing auth factors",
		LacrosStatus: testing.LacrosVariantUnneeded,
		Contacts: []string{
			"cros-hwsec@google.com",
			"hcyang@google.com",
		},
		BugComponent: "b:1188704",
		Attr:         []string{"group:mainline", "group:cryptohome", "informational"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "ussAuthSessionFixture",
	})
}

func RecoverableKeyStores(ctx context.Context, s *testing.State) {
	const (
		userPassword            = "secret"
		passwordLabel           = "password"
		passwordLabelNoKeyStore = "password-no-key-store"
		userPassword2           = "secret2"
		passwordLabel2          = "password-2"
		userPassword3           = "secret3"
		passwordLabel3          = "password-3"
		userPin                 = "12345"
		pinLabel                = "pin"
	)

	// Hash info used for each of the auth factors to test.
	hashInfo := uda.KnowledgeFactorHashInfo{
		Algorithm:              cryptohome_proto.KnowledgeFactorHashAlgorithm_HASH_TYPE_SHA256_TOP_HALF,
		Salt:                   []byte("salt"),
		ShouldGenerateKeyStore: true,
	}
	hashInfo2 := uda.KnowledgeFactorHashInfo{
		Algorithm:              cryptohome_proto.KnowledgeFactorHashAlgorithm_HASH_TYPE_SHA256_TOP_HALF,
		Salt:                   []byte("salt2"),
		ShouldGenerateKeyStore: true,
	}
	hashInfo3 := uda.KnowledgeFactorHashInfo{
		Algorithm:              cryptohome_proto.KnowledgeFactorHashAlgorithm_HASH_TYPE_SHA256_TOP_HALF,
		Salt:                   []byte("salt3"),
		ShouldGenerateKeyStore: true,
	}
	hashInfoNoKeyStore := uda.KnowledgeFactorHashInfo{
		Algorithm:              cryptohome_proto.KnowledgeFactorHashAlgorithm_HASH_TYPE_SHA256_TOP_HALF,
		Salt:                   []byte("salt4"),
		ShouldGenerateKeyStore: false,
	}
	hashInfoPin := uda.KnowledgeFactorHashInfo{
		Algorithm:              cryptohome_proto.KnowledgeFactorHashAlgorithm_HASH_TYPE_PBKDF2_AES256_1234,
		Salt:                   []byte("salt-pin"),
		ShouldGenerateKeyStore: true,
	}

	fixture := s.FixtValue().(*cryptohome.AuthSessionFixture)
	userName := fixture.TestUserName

	cmdRunner := hwseclocal.NewCmdRunner()
	client := hwsec.NewCryptohomeClient(cmdRunner)

	// Determine if the client supports using PIN auth.
	supportsPIN, err := client.SupportsLECredentials(ctx)
	if err != nil {
		s.Fatal("Unable to determine if PINs are supported: ", err)
	}

	// Setup a new user with password while the recoverable key store generation is disabled.
	if err := cryptochrome.WithGenerateRecoverableKeyStoreDisabled(ctx, func() error {
		_, authSessionID, err := client.StartAuthSession(ctx, userName /*ephemeral=*/, false, uda.AuthIntent_AUTH_INTENT_DECRYPT)
		if err != nil {
			return errors.Wrap(err, "failed to start auth session")
		}
		if err := client.CreatePersistentUser(ctx, authSessionID); err != nil {
			return errors.Wrap(err, "failed to create persistent user")
		}
		if _, err := client.PreparePersistentVault(ctx, authSessionID, false /*ecryptfs*/); err != nil {
			return errors.Wrap(err, "failed to prepare new persistent vault")
		}
		if err := client.AddAuthFactorWithHashInfo(ctx, authSessionID, passwordLabel, userPassword, &hashInfo); err != nil {
			return errors.Wrap(err, "failed to add password auth factor")
		}
		// Add another password to authenticate the auth session again later without using
		// |passwordLabel| (so that we can separately test its behavior later).
		if err := client.AddAuthFactorWithHashInfo(ctx, authSessionID, passwordLabelNoKeyStore, userPassword, &hashInfoNoKeyStore); err != nil {
			return errors.Wrap(err, "failed to add password auth factor")
		}
		return nil
	}); err != nil {
		s.Fatal("Failed to set up user with password: ", err)
	}

	// The recoverable key store generation was disabled when we add the password, so there should be
	// no key stores.
	if _, err := getRecoverableKeyStoresAndCheckSize(ctx, client, userName, 0); err != nil {
		s.Fatal("Failed to get recoverable key stores with correct size: ", err)
	}

	var expectedKeyStoreLen int
	if err := cryptochrome.WithGenerateRecoverableKeyStore(ctx, func() error {
		_, authSessionID, err := client.StartAuthSession(ctx, userName /*ephemeral=*/, false, uda.AuthIntent_AUTH_INTENT_DECRYPT)
		if err != nil {
			s.Fatal("Failed to start auth session: ", err)
		}
		// Authenticate the auth session. Note that it is set to not generate key stores, so we should still
		// have 0 key stores here.
		if _, err := client.AuthenticateAuthFactor(ctx, authSessionID, passwordLabelNoKeyStore, userPassword); err != nil {
			return errors.Wrap(err, "failed to authenticate password auth factor")
		}
		keyStores, err := getRecoverableKeyStoresAndCheckSize(ctx, client, userName, 0)
		if err != nil {
			return errors.Wrap(err, "failed to get recoverable key stores with correct size")
		}

		// Add another password factor. This time the key store should be generated.
		if err := client.AddAuthFactorWithHashInfo(ctx, authSessionID, passwordLabel2, userPassword2, &hashInfo2); err != nil {
			return errors.Wrap(err, "failed to add password auth factor")
		}
		keyStores, err = getRecoverableKeyStoresAndCheckSize(ctx, client, userName, 1)
		if err != nil {
			return errors.Wrap(err, "failed to get recoverable key stores with correct size")
		}
		originalKeyStore := keyStores[0]

		// Changing the label won't regenerate the key store.
		if err := client.RelabelAuthFactor(ctx, authSessionID, passwordLabel2, passwordLabel3); err != nil {
			return errors.Wrap(err, "failed to relabel auth factor")
		}
		keyStores, err = getRecoverableKeyStoresAndCheckSize(ctx, client, userName, 1)
		if err != nil {
			return errors.Wrap(err, "failed to get recoverable key stores with correct size")
		}
		if !proto.Equal(keyStores[0], originalKeyStore) {
			return errors.New("unexpected update of recoverable key store happened")
		}

		// Updating the auth factor will regenerate the key store.
		if err := client.UpdatePasswordAuthFactorWithHashInfo(ctx, authSessionID, passwordLabel3, userPassword3, &hashInfo3); err != nil {
			return errors.Wrap(err, "failed to update auth factor")
		}
		keyStores, err = getRecoverableKeyStoresAndCheckSize(ctx, client, userName, 1)
		if err != nil {
			return errors.Wrap(err, "failed to get recoverable key stores with correct size")
		}
		if proto.Equal(keyStores[0], originalKeyStore) {
			return errors.New("failed to update recoverable key stores")
		}

		// Replacing the auth factor will regenerate the key store. Change both the label and secret back to label2's
		// but without metadata. This should remove the key store.
		if err := client.ReplacePasswordAuthFactor(ctx, authSessionID, passwordLabel3, passwordLabel2, userPassword2); err != nil {
			return errors.Wrap(err, "failed to replace auth factor")
		}
		keyStores, err = getRecoverableKeyStoresAndCheckSize(ctx, client, userName, 0)
		if err != nil {
			return errors.Wrap(err, "failed to get recoverable key stores with correct size")
		}

		// Authenticating with the first password factor with decrypt intent will generate its recoverable key store.
		if _, err := client.AuthenticateAuthFactor(ctx, authSessionID, passwordLabel, userPassword); err != nil {
			return errors.Wrap(err, "failed to authenticate password auth factor")
		}
		if _, err = getRecoverableKeyStoresAndCheckSize(ctx, client, userName, 1); err != nil {
			return errors.Wrap(err, "failed to get recoverable key stores with correct size")
		}

		// Removing the first password factor should remove its recoverable key store as well.
		if err := client.RemoveAuthFactor(ctx, authSessionID, passwordLabel); err != nil {
			return errors.Wrap(err, "failed to remove password auth factor")
		}
		if _, err = getRecoverableKeyStoresAndCheckSize(ctx, client, userName, 0); err != nil {
			return errors.Wrap(err, "failed to get recoverable key stores with correct size")
		}

		// Add it back so we have at least 1 store to test cross-session behavior.
		if err := client.AddAuthFactorWithHashInfo(ctx, authSessionID, passwordLabel, userPassword, &hashInfo); err != nil {
			return errors.Wrap(err, "failed to add password auth factor")
		}
		if _, err = getRecoverableKeyStoresAndCheckSize(ctx, client, userName, 1); err != nil {
			return errors.Wrap(err, "failed to get recoverable key stores with correct size")
		}

		if supportsPIN {
			// Add a PIN auth factor. PIN auth factor should generate recoverable key store as well.
			if err := client.AddPinAuthFactorWithHashInfo(ctx, authSessionID, pinLabel, userPin, &hashInfoPin); err != nil {
				s.Fatal("Failed to add PIN auth factor: ", err)
			}
			if _, err = getRecoverableKeyStoresAndCheckSize(ctx, client, userName, 2); err != nil {
				return errors.Wrap(err, "failed to get recoverable key stores with correct size")
			}
			expectedKeyStoreLen = 2
		} else {
			expectedKeyStoreLen = 1
		}

		return nil
	}); err != nil {
		s.Fatal("Failed to verify recoverable key store generation behavior: ", err)
	}

	// Unmount the user.
	if err := client.UnmountAll(ctx); err != nil {
		s.Fatal("Failed to unmount vaults: ", err)
	}

	// Even after unmount we should still be able to list the recoverable key stores.
	if _, err = getRecoverableKeyStoresAndCheckSize(ctx, client, userName, expectedKeyStoreLen); err != nil {
		s.Fatal("Failed to get recoverable key stores with correct size: ", err)
	}
}

func getRecoverableKeyStoresAndCheckSize(ctx context.Context, client *hwsec.CryptohomeClient, userName string, expectedLen int) ([]*cryptohome_proto.RecoverableKeyStore, error) {
	reply, err := client.GetRecoverableKeyStores(ctx, userName)
	if err != nil {
		return []*cryptohome_proto.RecoverableKeyStore{}, errors.Wrap(err, "failed to get recoverable key stores")
	}
	if len(reply.KeyStores) != expectedLen {
		return []*cryptohome_proto.RecoverableKeyStore{}, errors.Errorf("unexpected key stores length: got %v, want %v", len(reply.KeyStores), expectedLen)
	}
	return reply.KeyStores, nil
}
