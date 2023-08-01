// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hwsec

import (
	"context"
	"time"

	uda "chromiumos/system_api/user_data_auth_proto"

	"go.chromium.org/tast-tests/cros/common/hwsec"
	"go.chromium.org/tast-tests/cros/common/pkcs11"
	"go.chromium.org/tast-tests/cros/common/pkcs11/pkcs11test"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/hwsec/util"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	hwseclocal "go.chromium.org/tast-tests/cros/local/hwsec"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         UssMigrationChapsRemount,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies chaps works correctly after USS migration",
		Contacts: []string{
			"cryptohome-core@google.com",
			"betuls@google.com",
		},
		BugComponent: "b:1088399", // ChromeOS > Security > Cryptohome
		Timeout:      4 * time.Minute,
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:hwsec", "hwsec_nightly"},
		Params: []testing.Param{{
			ExtraSoftwareDeps: []string{"tpm", "no_tpm_dynamic"},
			// checkSoftwareBacked: For devices with official TPM, we do not allow fallback
			// to software backed keys so we need to check if the keys are not software backed
			// when they're not supposed to be.
			Val: true,
		}, {
			Name:              "tpm_dynamic",
			ExtraSoftwareDeps: []string{"tpm_dynamic"},
			Val:               false,
		}},
	})
}

// UssMigrationChapsRemount tests the Chaps functions properly after USS migration with user
// login.
func UssMigrationChapsRemount(ctx context.Context, s *testing.State) {
	const (
		userName      = "test@test.com"
		userPassword  = "secret"
		passwordLabel = "gaia"
		ussFile       = "/user_secret_stash/uss.0"
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

	// Create a new user when USS is disabled, and create Chaps keys.

	cleanupUSSDisabled, err := helper.DisableUserSecretStash(ctx)
	if err != nil {
		s.Fatal("Failed to disable UserSecretStash: ", err)
	}
	defer cleanupUSSDisabled(ctxForCleanup)

	pkcs11Util, err := pkcs11.NewChaps(ctx, cmdRunner, client)
	if err != nil {
		s.Fatal("Failed to create PKCS#11 Utility: ", err)
	}

	const scratchpadPath = "/tmp/ChapsRemountTest"

	// Remove all keys/certs before the test as well.
	if err := pkcs11test.CleanupScratchpad(ctx, cmdRunner, scratchpadPath); err != nil {
		s.Fatal("Failed to clean scratchpad before the start of test: ", err)
	}
	util.CleanupKeysBeforeTest(ctx, pkcs11Util, client)

	// Prepare the scratchpad.
	f1, f2, err := pkcs11test.PrepareScratchpadAndTestFiles(ctx, cmdRunner, scratchpadPath)
	if err != nil {
		s.Fatal("Failed to initialize the scratchpad space: ", err)
	}
	// Remove all keys/certs, if any at the end. i.e. Cleanup after ourselves.
	defer pkcs11test.CleanupScratchpad(ctx, cmdRunner, scratchpadPath)

	// Create the various keys.
	keys, err := util.CreateKeysForTesting(ctx, cmdRunner, pkcs11Util, client, scratchpadPath, util.RSAKey, s.Param().(bool))
	if err != nil {
		s.Fatal("Failed to create keys for testing: ", err)
	}
	defer func() {
		if err := util.CleanupTestingKeys(ctx, keys, pkcs11Util, client); err != nil {
			s.Error("Failed to cleanup testing keys: ", err)
		}
	}()
	// Give the cleanup 10 seconds to finish.
	shortenedCtx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Test the various keys.
	for _, k := range keys {
		// Test the various mechanisms.
		for _, m := range []pkcs11.MechanismInfo{pkcs11.SHA1RSAPKCS, pkcs11.SHA256RSAPKCS} {
			if err := pkcs11test.SignAndVerify(shortenedCtx, pkcs11Util, k, f1, f2, &m); err != nil {
				s.Error("SignAndVerify failed: ", err)
			}
		}
	}
	if err := client.UnmountAll(ctx); err != nil {
		s.Fatal("Failed to unmount vault: ", err)
	}

	// Enable USS and USS migration and test that Chaps function correctly after migration.

	cleanupUSSExperiment, err := helper.EnableUserSecretStash(ctx)
	if err != nil {
		s.Fatal("Failed to enable the UserSecretStash experiment: ", err)
	}
	defer cleanupUSSExperiment(ctxForCleanup)

	if err := client.WithAuthSession(ctx, util.FirstUsername, false /*isEphemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT, func(authSessionID string) error {
		// Authenticate with correct password and migrate backing store to USS.
		if _, err := client.AuthenticateAuthFactor(ctx, authSessionID, util.PasswordLabel, util.FirstPassword); err != nil {
			return errors.Wrap(err, "USS migration test failed at authentication step with password keyset")
		}
		// Check that the migration completed by checking USS file.
		if err := cryptohome.CheckKeyBackingStoreExists(ctx, ussFile, util.FirstUsername); err != nil {
			return errors.Wrap(err, "failed to check USS file after migration")
		}

		if _, err := client.PreparePersistentVault(ctx, authSessionID, false /*ecryptfs*/); err != nil {
			return errors.Wrap(err, "failed to mount user vault after password migration")
		}
		defer client.Unmount(ctxForCleanup, util.FirstUsername)

		// Authenticate with correct password factor should succeed after migration.
		if _, err := client.AuthenticateAuthFactor(ctx, authSessionID, util.PasswordLabel, util.FirstPassword); err != nil {
			return errors.Wrap(err, "USS migration test failed at authentication step after the migration of password")
		}
		// Test the various keys after the migration.
		for _, key := range keys {
			// Test the various mechanisms.
			for _, m := range []pkcs11.MechanismInfo{pkcs11.SHA1RSAPKCS, pkcs11.SHA256RSAPKCS} {
				if err := pkcs11test.SignAndVerify(shortenedCtx, pkcs11Util, key, f1, f2, &m); err != nil {
					s.Error("SignAndVerify failed: ", err)
				}
			}
		}
		return nil
	}); err != nil {
		s.Fatal("Failed to test password migration: ", err)
	}
}
