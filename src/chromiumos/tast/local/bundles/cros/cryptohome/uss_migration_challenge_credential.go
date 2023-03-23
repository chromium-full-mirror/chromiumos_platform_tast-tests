// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cryptohome

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	cpb "chromiumos/system_api/cryptohome_proto"
	uda "chromiumos/system_api/user_data_auth_proto"
	cryptohomecommon "chromiumos/tast/common/cryptohome"
	"chromiumos/tast/common/hwsec"
	"chromiumos/tast/ctxutil"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/cryptohome"
	cryptochrome "chromiumos/tast/local/cryptohome/chrome"
	"chromiumos/tast/local/dbusutil"
	hwseclocal "chromiumos/tast/local/hwsec"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         UssMigrationChallengeCredential,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test that migration of challenge credential factor succeed during user login",
		Contacts: []string{
			"cryptohome-core@google.com",
			"betuls@google.com",
		},
		BugComponent: "b:1088399", // ChromeOS > Security > Cryptohome
		SoftwareDeps: []string{"chrome", "tpm"},
		Attr:         []string{"group:mainline"},
		Params: []testing.Param{{
			Name: "rsassa_sha1",
			Val: []cpb.ChallengeSignatureAlgorithm{
				cpb.ChallengeSignatureAlgorithm_CHALLENGE_RSASSA_PKCS1_V1_5_SHA1,
			},
		}, {
			Name: "rsassa_all",
			Val:  hwsec.SmartCardAlgorithms,
		}},
	})
}

// createSmartCardConfig sets up AuthConfig for smart card login.
func createSmartCardConfig(username string,
	keyAlgs []cpb.ChallengeSignatureAlgorithm, logFunction hwsec.LogFunc) (*hwsec.AuthConfig, func(), error) {
	const (
		keySizeBits = 2048
		dbusName    = "org.chromium.TestingCryptohomeKeyDelegate"
	)
	// Use a pseudorandom generator with a fixed seed, to make the values used by
	// the test predictable.
	randReader := rand.New(rand.NewSource(0 /* seed */))

	rsaKey, err := rsa.GenerateKey(randReader, keySizeBits)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to generate RSA key")
	}

	pubKeySPKIDER, err := x509.MarshalPKIXPublicKey(&rsaKey.PublicKey)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to generate SubjectPublicKeyInfo")
	}

	dbusConn, err := dbusutil.SystemBus()
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to connect to system D-Bus bus")
	}

	if _, err := dbusConn.RequestName(dbusName, 0 /* flags */); err != nil {
		return nil, nil, errors.Wrap(err, "failed to request the well-known D-Bus name")
	}

	keyDelegate, err := hwsec.NewCryptohomeKeyDelegate(
		logFunction, dbusConn, username, keyAlgs, rsaKey, pubKeySPKIDER)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to export D-Bus key delegate")
	}

	// Prepare Smart Card config.
	authConfig := hwsec.NewChallengeAuthConfig(username, dbusName, keyDelegate.DBusPath, pubKeySPKIDER, keyAlgs)

	cleanup := func() {
		dbusConn.ReleaseName(authConfig.KeyDelegateName)
		keyDelegate.Close()
	}

	return authConfig, cleanup, nil
}

// UssMigrationChallengeCredential tests the smartcard AuthFactor works after USS migration
// during the user login.
func UssMigrationChallengeCredential(ctx context.Context, s *testing.State) {
	const (
		smartCardLabel          = "SmartCardTest"
		testUser                = "testUser@test.com"
		smartcardKeysetFile     = "master.0" // nocheck
		ussFile                 = "/user_secret_stash/uss.0"
		smartcardAuthFactorFile = "/auth_factors/smart_card.SmartCardTest"
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
	if _, err := client.RemoveVault(ctx, testUser); err != nil {
		s.Fatal("Failed to remove old vault for preparation: ", err)
	}

	// Setup smartcard config that is used for the entire test.
	userParam := s.Param().([]cpb.ChallengeSignatureAlgorithm)
	authConfig, cleanup, err := createSmartCardConfig(testUser, userParam, s.Logf)
	if err != nil {
		s.Fatal("Failed to setup smartcard config: ", err)
	}
	defer cleanup()

	// 1. Create a new smartcard (challenge credential) user with VaultKeysets. Disable USS
	// and USS migration for this initial setup.
	if err := cryptochrome.WithUssMigration(ctx, false /*enabled*/, func() error {
		cleanupUSSDisabled, err := helper.DisableUserSecretStash(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to disable UserSecretStash")
		}
		defer cleanupUSSDisabled(ctxForCleanup)

		if err := client.WithAuthSession(ctx, testUser, false /*isEphemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT, func(authSessionID string) error {
			// Create user vault.
			if err := client.CreatePersistentUser(ctx, authSessionID); err != nil {
				return errors.Wrap(err, "failed to create user")
			}
			// Mount user home directories and daemon-store directories.
			if _, err := client.PreparePersistentVault(ctx, authSessionID, false /*ecryptfs*/); err != nil {
				return errors.Wrap(err, "failed to mount user profile after creation")
			}
			defer client.Unmount(ctxForCleanup, testUser)
			// Add smartcard AuthFactor.
			if err := client.AddSmartCardAuthFactor(ctx, authSessionID, smartCardLabel, authConfig); err != nil {
				return errors.Wrap(err, "failed to add smartcard AuthFactor")
			}
			// Check that the smartcard VaultKeyset file is created.
			if err := cryptohome.CheckKeyBackingStoreExists(ctx, smartcardKeysetFile, testUser); err != nil {
				return errors.Wrap(err, "failed to check smartcard VaultKeyset file")
			}
			if err := cryptohome.WriteFileForPersistence(ctx, testUser); err != nil {
				return errors.Wrap(err, "failed to write test file")
			}
			return nil
		}); err != nil {
			return errors.Wrap(err, "failed to create and set up smartcard with uss and migration disabled")
		}
		return nil
	}); err != nil {
		s.Fatal("Setup while USS migration was disabled failed: ", err)
	}
	// Cleanup user vault before UssMigrationChallengeCredential exits.
	defer client.RemoveVault(ctxForCleanup, testUser)

	// 2. Enable USS and USS migration for the second phase of the test. Test
	// that only successful authentication migrates the smartcard factor.
	if err := cryptochrome.WithUssMigration(ctx, true /*enabled*/, func() error {
		cleanupUSSExperiment, err := helper.EnableUserSecretStash(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to enable the UserSecretStash experiment")
		}
		defer cleanupUSSExperiment(ctxForCleanup)

		if err := client.WithAuthSession(ctx, testUser, false /*isEphemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT, func(authSessionID string) error {

			// Authenticate with smartcard and migrate backing store to USS.
			if _, err := client.AuthenticateSmartCardAuthFactor(ctx, authSessionID, smartCardLabel, authConfig); err != nil {
				return errors.Wrap(err, "USS migration test failed at authentication step with smartcard")
			}
			// Check that the smartcard AuthFactor and USS files are created.
			if err := cryptohome.CheckKeyBackingStoreExists(ctx, ussFile, testUser); err != nil {
				return errors.Wrap(err, "failed to check USS file after migration")
			}
			if err := cryptohome.CheckKeyBackingStoreExists(ctx, smartcardAuthFactorFile, testUser); err != nil {
				return errors.Wrap(err, "failed to check smartcard AuthFactor file after migration")
			}

			// Remove VaultKeyset file to ensure that the authentication takes AuthFactor path.
			const shadowDir = "/home/.shadow"
			hash, _ := cryptohome.UserHash(ctx, testUser)
			if err := os.Remove(filepath.Join(shadowDir, hash, smartcardKeysetFile)); err != nil {
				return errors.Wrap(err, "failed to remove the smartcard VaultKeyset file")
			}

			// Authenticate should succeed after migration.
			if _, err := client.AuthenticateSmartCardAuthFactor(ctx, authSessionID, smartCardLabel, authConfig); err != nil {
				return errors.Wrap(err, "USS migration of smartcard failed at authentication step after the migration")
			}

			// Test that test file is still there.
			if err := cryptohome.MountAndVerify(ctx, testUser, authSessionID, false /*ecryptfs*/); err != nil {
				return errors.Wrap(err, "failed to mount and verify persistence after migration")
			}
			return nil
		}); err != nil {
			return errors.Wrap(err, "failed to test smartcard migration")
		}

		// Test that lock screen works with smartcard after the migration.
		if err := client.WithAuthSession(ctx, testUser, false /*isEphemeral*/, uda.AuthIntent_AUTH_INTENT_VERIFY_ONLY, func(authSessionID string) error {
			// Call AuthenticateSmartCardAuthFactor again, this time to test the lock screen.
			reply, err := client.AuthenticateSmartCardAuthFactor(ctx, authSessionID, smartCardLabel, authConfig)
			if err != nil {
				return errors.Wrap(err, "failed to test lock screen unlock with smartcard")
			}
			// Check that reply matches with the lock screen AuthIntent.
			if err = cryptohomecommon.ExpectAuthIntents(reply.AuthorizedFor, []uda.AuthIntent{uda.AuthIntent_AUTH_INTENT_VERIFY_ONLY}); err != nil {
				return errors.Wrap(err, "unexpected AuthSession authorized intents")
			}
			return nil
		}); err != nil {
			return errors.Wrap(err, "failed to test smartcard migration")
		}
		return nil
	}); err != nil {
		s.Fatal("Setup while USS migration was enabled failed: ", err)
	}
}
