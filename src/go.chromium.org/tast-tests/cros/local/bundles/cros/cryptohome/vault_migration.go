// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cryptohome

import (
	"context"
	"time"

	uda "go.chromium.org/chromiumos/system_api/user_data_auth_proto"

	"go.chromium.org/tast-tests/cros/common/hwsec"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	hwseclocal "go.chromium.org/tast-tests/cros/local/hwsec"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: VaultMigration,
		Desc: "Test vault encryption migration from ecryptfs to fscrypt",
		Contacts: []string{
			"cryptohome-core@google.com",
			"dlunev@chromium.org",
		},
		BugComponent: "b:974567", // ChromeOS > Platform> Technologies > Storage
		Attr:         []string{"group:mainline", "group:cryptohome", "informational"},
		Params: []testing.Param{{
			Name:              "fscrypt_v1",
			ExtraSoftwareDeps: []string{"use_fscrypt_v1"},
		}, {
			Name:              "fscrypt_v2",
			ExtraSoftwareDeps: []string{"use_fscrypt_v2"},
		}},
		Timeout: 60 * time.Second,
	})
}

func VaultMigration(ctx context.Context, s *testing.State) {
	const (
		userName     = "foo@bar.baz"
		userPassword = "secret"
		keyLabel     = "fake_label"
	)

	ctxForCleanUp := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	s.Log("Prepare environment")

	cmdRunner := hwseclocal.NewCmdRunner()
	client := hwsec.NewCryptohomeClient(cmdRunner)

	// Ensure cryptohomed is started and wait for it to be available.
	if err := cryptohome.CheckService(ctx); err != nil {
		s.Fatal("Failed to ensure cryptohomed: ", err)
	}

	if err := client.UnmountAll(ctx); err != nil {
		s.Fatal("Failed to unmount vaults for preparation: ", err)
	}

	if err := cryptohome.RemoveVault(ctx, userName); err != nil {
		s.Fatal("Failed to remove old vault for preparation: ", err)
	}

	// Create and set up the user.
	if err := client.WithAuthSession(ctx, userName, false /*isEphemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT, func(authSessionID string) error {
		if err := client.CreatePersistentUser(ctx, authSessionID); err != nil {
			return errors.Wrap(err, "failed to create persistent user")
		}
		s.Log("Create ecryptfs vault with a file")
		if _, err := client.PreparePersistentVault(ctx, authSessionID, true /*ecryptfs*/); err != nil {
			return errors.Wrap(err, "failed to prepare new persistent vault")
		}
		if err := client.AddAuthFactor(ctx, authSessionID, keyLabel, userPassword); err != nil {
			return errors.Wrap(err, "failed to add initial user password")
		}
		// Write a test file to verify persistence.
		if err := cryptohome.WriteFileForPersistence(ctx, userName); err != nil {
			return errors.Wrap(err, "failed to write test file")
		}
		return nil
	}); err != nil {
		s.Fatal("Failed to create and set up the user: ", err)
	}
	defer cryptohome.RemoveVault(ctxForCleanUp, userName)

	// Unmount the user and verify that we cannot read the persistent file.
	if err := client.UnmountAll(ctx); err != nil {
		s.Fatal("Failed to unmount vaults for re-mounting: ", err)
	}

	if err := cryptohome.VerifyFileUnreadability(ctx, userName); err != nil {
		s.Fatal("File is readable after unmount")
	}

	// Migrate the user vault.
	if err := client.WithAuthSession(ctx, userName, false /*isEphemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT, func(authSessionID string) error {
		if _, err := client.AuthenticateAuthFactor(ctx, authSessionID, keyLabel, userPassword); err != nil {
			return errors.Wrap(err, "failed to authenticate user")
		}
		if err := client.PrepareVaultForMigration(ctx, authSessionID); err != nil {
			return errors.Wrap(err, "failed to prepare vault for migration")
		}
		if err := client.MigrateToDircrypto(ctx, userName); err != nil {
			return errors.Wrap(err, "failed to migrate vault to dircrypto")
		}
		return nil
	}); err != nil {
		s.Fatal("Failed to migrate the user vault: ", err)
	}

	// Mount the user vault as fscrypt.
	if err := client.WithAuthSession(ctx, userName, false /*isEphemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT, func(authSessionID string) error {
		if _, err := client.AuthenticateAuthFactor(ctx, authSessionID, keyLabel, userPassword); err != nil {
			return errors.Wrap(err, "failed to authenticate user")
		}
		s.Log("Mount as fscrypt")
		if _, err := client.PreparePersistentVault(ctx, authSessionID, false /*ecryptfs*/); err != nil {
			return errors.Wrap(err, "failed to mount the user vault")
		}
		return nil
	}); err != nil {
		s.Fatal("Failed to mount the user fscrypt vault: ", err)
	}
	defer client.UnmountAll(ctxForCleanUp)

	// Verify that file is still there.
	if err := cryptohome.VerifyFileForPersistence(ctx, userName); err != nil {
		s.Fatal("Failed to verify file persistence: ", err)
	}
}
