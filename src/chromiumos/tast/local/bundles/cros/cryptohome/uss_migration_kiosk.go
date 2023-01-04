// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cryptohome

import (
	"context"
	"time"

	uda "chromiumos/system_api/user_data_auth_proto"
	"chromiumos/tast/common/hwsec"
	"chromiumos/tast/ctxutil"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/cryptohome"
	cryptochrome "chromiumos/tast/local/cryptohome/chrome"
	hwseclocal "chromiumos/tast/local/hwsec"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         UssMigrationKiosk,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test migration to USS of Kiosk keysets",
		Contacts: []string{
			"cryptohome-core@google.com",
			"jadmanski@chromium.org",
		},
		BugComponent: "b:1088399",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
	})
}

func UssMigrationKiosk(ctx context.Context, s *testing.State) {
	const (
		ownerName       = "owner@bar.baz"
		cleanupTime     = 20 * time.Second
		kioskKeysetFile = "master.0" // nocheck
		ussFile         = "/user_secret_stash/uss.0"
		kioskFactorFile = "/auth_factors/kiosk.public_mount"
	)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, cleanupTime)
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

	// Clean up old state or mounts for the test user, if any exists.
	if err := client.UnmountAll(ctx); err != nil {
		s.Fatal("Failed to unmount vaults for preparation: ", err)
	}
	if err := cryptohome.RemoveVault(ctx, cryptohome.KioskUser); err != nil {
		s.Fatal("Failed to remove old vault for preparation: ", err)
	}

	// Set up an auth factor with USS migration disabled.
	if err := cryptochrome.WithUssMigration(ctx, false /*enabled*/, func() error {
		// Put the system into USS disabled mode, to ensure we get VK credentials.
		disableUssCleanup, err := helper.DisableUserSecretStash(ctx)
		if err != nil {
			return errors.Wrap(err, "unable to disable USS before creating credentials")
		}
		defer disableUssCleanup(cleanupCtx)

		// Create the user with a persistent vault and add a kiosk credential.
		// This should produce a VK factor.
		if err := client.WithAuthSession(ctx, cryptohome.KioskUser, false /*ephemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT, func(authSessionID string) error {
			if err := client.CreatePersistentUser(ctx, authSessionID); err != nil {
				return errors.Wrap(err, "failed to create persistent user")
			}
			if _, err := client.PreparePersistentVault(ctx, authSessionID, false /*ecryptfs*/); err != nil {
				return errors.Wrap(err, "failed to prepare new persistent vault")
			}
			if err := client.AddKioskAuthFactor(ctx, authSessionID); err != nil {
				return errors.Wrap(err, "failed to add kiosk credentials")
			}

			// Check that the kiosk VaultKeyset file is created.
			if err := cryptohome.CheckKeyBackingStoreExists(ctx, kioskKeysetFile, cryptohome.KioskUser); err != nil {
				return errors.Wrap(err, "kiosk keyset file was not created")
			}

			if err := cryptohome.WriteFileForPersistence(ctx, cryptohome.KioskUser); err != nil {
				return errors.Wrap(err, "failed to write test file")
			}
			return nil
		}); err != nil {
			return errors.Wrap(err, "failed to create and set up the user")
		}

		// Unmount all user vaults.
		if err := cryptohome.UnmountVault(ctx, cryptohome.KioskUser); err != nil {
			return errors.Wrap(err, "failed to unmount vault after pre-migration mount")
		}
		return nil
	}); err != nil {
		s.Fatal("Setup while USS migration was disabled failed: ", err)
	}
	defer client.RemoveVault(cleanupCtx, ownerName)

	// Enable migration to verify the migration process.
	if err := cryptochrome.WithUssMigration(ctx, true /*enabled*/, func() error {
		// Switch cryptohome into USS mode.
		enableUssCleanup, err := helper.EnableUserSecretStash(ctx)
		if err != nil {
			return errors.Wrap(err, "unable to enable USS after creating credentials")
		}
		defer enableUssCleanup(cleanupCtx)

		// Check that migrated Kiosk factor has not been migrated.
		if err := cryptohome.CheckKeyBackingStoreExists(ctx, kioskFactorFile, cryptohome.KioskUser); err == nil {
			return errors.New("kiosk auth factor file was created before migration should have happened")
		}

		// Start a new auth session and mount the persistent vault.
		// This should do migration.
		if err := client.WithAuthSession(ctx, cryptohome.KioskUser, false /*ephemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT, func(authSessionID string) error {
			if err := client.AuthenticateKioskAuthFactor(ctx, authSessionID); err != nil {
				return errors.Wrap(err, "failed to authenticate with kiosk credential")
			}
			if err := cryptohome.MountAndVerify(ctx, cryptohome.KioskUser, authSessionID, false /*ecryptfs*/); err != nil {
				return errors.Wrap(err, "failed to mount and verify persistence")
			}
			return nil
		}); err != nil {
			return errors.Wrap(err, "failed to authenticate and mount the user vault with migration")
		}

		// Check that migrated Kiosk factor has now been migrated. There should be both a USS
		// and a file for the kiosk auth factor.
		if err := cryptohome.CheckKeyBackingStoreExists(ctx, ussFile, cryptohome.KioskUser); err != nil {
			return errors.Wrap(err, "USS file was not created")
		}
		if err := cryptohome.CheckKeyBackingStoreExists(ctx, kioskFactorFile, cryptohome.KioskUser); err != nil {
			return errors.Wrap(err, "kiosk auth factor file was not created")
		}

		// Unmount user vault.
		if err := cryptohome.UnmountVault(ctx, cryptohome.KioskUser); err != nil {
			return errors.Wrap(err, "failed to unmount vault after migration mount")
		}
		return nil
	}); err != nil {
		s.Fatal("Validation during USS migration failed: ", err)
	}

	// Enable migration to verify that the migrated factor works.
	if err := cryptochrome.WithUssMigration(ctx, true /*enabled*/, func() error {
		// Switch cryptohome into USS mode.
		enableUssCleanup, err := helper.EnableUserSecretStash(ctx)
		if err != nil {
			return errors.Wrap(err, "unable to enable USS after creating credentials")
		}
		defer enableUssCleanup(cleanupCtx)

		// Start a new auth session and mount the persistent vault.
		// This should work with the migrated factor.
		if err := client.WithAuthSession(ctx, cryptohome.KioskUser, false /*ephemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT, func(authSessionID string) error {
			if err := client.AuthenticateKioskAuthFactor(ctx, authSessionID); err != nil {
				return errors.Wrap(err, "failed to authenticate with kiosk credential")
			}
			if err := cryptohome.MountAndVerify(ctx, cryptohome.KioskUser, authSessionID, false /*ecryptfs*/); err != nil {
				return errors.Wrap(err, "failed to mount and verify persistence")
			}
			return nil
		}); err != nil {
			return errors.Wrap(err, "failed to authenticate and mount the user vault")
		}

		// Unmount user vault.
		if err := cryptohome.UnmountVault(ctx, cryptohome.KioskUser); err != nil {
			return errors.Wrap(err, "failed to unmount vault after post-migration mount")
		}
		return nil
	}); err != nil {
		s.Fatal("Validation after USS migration failed: ", err)
	}

	// Disable migration and re-run authentication to verify the rollback process.
	if err := cryptochrome.WithUssMigration(ctx, false /*enabled*/, func() error {
		// Make sure cryptohome is still in USS mode, we're only disabling migration.
		enableUssCleanup, err := helper.EnableUserSecretStash(ctx)
		if err != nil {
			return errors.Wrap(err, "unable to enable USS with migration rollback")
		}
		defer enableUssCleanup(cleanupCtx)

		// Start a new auth session and mount the persistent vault.
		// This should work with the old VK credentials.
		if err := client.WithAuthSession(ctx, cryptohome.KioskUser, false /*ephemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT, func(authSessionID string) error {
			if err := client.AuthenticateKioskAuthFactor(ctx, authSessionID); err != nil {
				return errors.Wrap(err, "failed to authenticate with kiosk credential")
			}
			if err := cryptohome.MountAndVerify(ctx, cryptohome.KioskUser, authSessionID, false /*ecryptfs*/); err != nil {
				return errors.Wrap(err, "failed to mount and verify persistence")
			}
			return nil
		}); err != nil {
			return errors.Wrap(err, "failed to authenticate and mount the user vault with rollback")
		}

		// After migration is disabled, all of the backing stores (VK and USS) should still exist.
		if err := cryptohome.CheckKeyBackingStoreExists(ctx, kioskKeysetFile, cryptohome.KioskUser); err != nil {
			return errors.Wrap(err, "kiosk keyset file no longer exists")
		}
		if err := cryptohome.CheckKeyBackingStoreExists(ctx, ussFile, cryptohome.KioskUser); err != nil {
			return errors.Wrap(err, "USS file no longer exists")
		}
		if err := cryptohome.CheckKeyBackingStoreExists(ctx, kioskFactorFile, cryptohome.KioskUser); err != nil {
			return errors.Wrap(err, "kiosk auth factor file no longer exists")
		}

		// Unmount user vault.
		if err := cryptohome.UnmountVault(ctx, cryptohome.KioskUser); err != nil {
			return errors.Wrap(err, "failed to unmount vault after post-rollback mount")
		}
		return nil
	}); err != nil {
		s.Fatal("Validation while USS migration was rolled back failed: ", err)
	}
}
