// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cryptohome

import (
	"context"
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

type ussMigrationKioskParam struct {
	// Specify if the test should use a legacy as kiosk-as-password VK or a
	// more modern factor that explicitly identifies itself.
	testLegacyKiosk bool
	// The label of the kiosk factor to test.
	kioskLabel string
	// Specify if the test should also verify mounting the user directory. If
	// not specified then only authentication is tested.
	testMount bool
	// The path of the auth factor file for the kiosk factor.
	factorFile string
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         UssMigrationKiosk,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test migration to USS of Kiosk keysets",
		Contacts: []string{
			"cryptohome-core@google.com",
			"jadmanski@chromium.org",
		},
		BugComponent: "b:1088399", // ChromeOS > Security > Cryptohome
		Attr:         []string{"group:mainline", "group:cryptohome", "informational"},
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{{
			Name: "legacy",
			Val: ussMigrationKioskParam{
				testLegacyKiosk: true,
				kioskLabel:      "legacy-0",
				testMount:       false,
				factorFile:      "/auth_factors/kiosk.legacy-0",
			},
		}, {
			Name: "modern",
			Val: ussMigrationKioskParam{
				testLegacyKiosk: false,
				kioskLabel:      "public_mount",
				testMount:       true,
				factorFile:      "/auth_factors/kiosk.public_mount",
			},
		}},
	})
}

func UssMigrationKiosk(ctx context.Context, s *testing.State) {
	const (
		cleanupTime     = 20 * time.Second
		kioskKeysetFile = "master.0" // nocheck
		ussFile         = "/user_secret_stash/uss.0"
	)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, cleanupTime)
	defer cancel()

	userParam := s.Param().(ussMigrationKioskParam)

	cmdRunner := hwseclocal.NewCmdRunner()
	client := hwsec.NewCryptohomeClient(cmdRunner)
	testTool, err := cryptohomecommon.NewRecoveryTestToolWithFakeMediator(cmdRunner)
	if err != nil {
		s.Fatal("Failed to initialize RecoveryTestTool: ", err)
	}

	// Wait for cryptohomed to become available if needed.
	if err := cryptohome.CheckService(ctx); err != nil {
		s.Fatal("Failed to ensure cryptohomed: ", err)
	}

	// Clean up old state or mounts for the test user, if any exists.
	if err := client.UnmountAll(ctx); err != nil {
		s.Fatal("Failed to unmount vaults for preparation: ", err)
	}
	if err := cryptohome.RemoveVault(ctx, cryptohome.KioskUser); err != nil {
		s.Fatal("Failed to remove old vault for preparation: ", err)
	}

	// Create the user with a persistent vault and add a VK kiosk credential.
	if err := client.WithAuthSession(ctx, cryptohome.KioskUser, false /*ephemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT, func(authSessionID string) error {
		if err := client.CreatePersistentUser(ctx, authSessionID); err != nil {
			return errors.Wrap(err, "failed to create persistent user")
		}
		if _, err := client.PreparePersistentVault(ctx, authSessionID, false /*ecryptfs*/); err != nil {
			return errors.Wrap(err, "failed to prepare new persistent vault")
		}

		// Create the actual kiosk credential, depending on the current modeof operation.
		creationLabel := userParam.kioskLabel
		if userParam.testLegacyKiosk {
			creationLabel = ""
		}
		// In the typed setup, create a modern kiosk VK using the standard auth factor API.
		// In the legacy setup, create a keydata-less credential with no identifying info.
		if err := testTool.CreateVaultKeyset(ctx, authSessionID, cryptohome.KioskUser /*keyDataLabel=*/, creationLabel, uda.AuthFactorType_AUTH_FACTOR_TYPE_KIOSK, userParam.testLegacyKiosk); err != nil {
			return errors.Wrap(err, "failed to create VaultKeyset")
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
		s.Fatal("Failed to create and set up the user: ", err)
	}

	// Unmount all user vaults.
	if err := cryptohome.UnmountVault(ctx, cryptohome.KioskUser); err != nil {
		s.Fatal("Failed to unmount vault after pre-migration mount: ", err)
	}
	defer client.RemoveVault(cleanupCtx, cryptohome.KioskUser)

	// Check that migrated Kiosk factor has not been migrated.
	if err := cryptohome.CheckKeyBackingStoreExists(ctx, userParam.factorFile, cryptohome.KioskUser); err == nil {
		s.Fatal("kiosk auth factor file was created before migration should have happened")
	}

	// Start a new auth session and mount the persistent vault.
	// This should do migration.
	if err := client.WithAuthSession(ctx, cryptohome.KioskUser, false /*ephemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT, func(authSessionID string) error {
		if err := client.AuthenticateKioskAuthFactorWithLabel(ctx, authSessionID, userParam.kioskLabel); err != nil {
			return errors.Wrap(err, "failed to authenticate with kiosk credential")
		}
		if userParam.testMount {
			if err := cryptohome.MountAndVerify(ctx, cryptohome.KioskUser, authSessionID, false /*ecryptfs*/); err != nil {
				return errors.Wrap(err, "failed to mount and verify persistence")
			}
		}
		return nil
	}); err != nil {
		s.Fatal("Failed to authenticate and mount the user vault with migration: ", err)
	}

	// Check that migrated Kiosk factor has now been migrated. There should be both a USS
	// and a file for the kiosk auth factor.
	if err := cryptohome.CheckKeyBackingStoreExists(ctx, ussFile, cryptohome.KioskUser); err != nil {
		s.Fatal("USS file was not created: ", err)
	}
	if err := cryptohome.CheckKeyBackingStoreExists(ctx, userParam.factorFile, cryptohome.KioskUser); err != nil {
		s.Fatal("Kiosk auth factor file was not created: ", err)
	}

	// Unmount user vault.
	if err := cryptohome.UnmountVault(ctx, cryptohome.KioskUser); err != nil {
		s.Fatal("Failed to unmount vault after migration mount: ", err)
	}

	// Start a new auth session and mount the persistent vault.
	// This should work with the migrated factor.
	if err := client.WithAuthSession(ctx, cryptohome.KioskUser, false /*ephemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT, func(authSessionID string) error {
		if err := client.AuthenticateKioskAuthFactorWithLabel(ctx, authSessionID, userParam.kioskLabel); err != nil {
			return errors.Wrap(err, "failed to authenticate with kiosk credential")
		}
		if userParam.testMount {
			if err := cryptohome.MountAndVerify(ctx, cryptohome.KioskUser, authSessionID, false /*ecryptfs*/); err != nil {
				return errors.Wrap(err, "failed to mount and verify persistence")
			}
		}
		return nil
	}); err != nil {
		s.Fatal("Failed to authenticate and mount the user vault: ", err)
	}

	// Unmount user vault.
	if err := cryptohome.UnmountVault(ctx, cryptohome.KioskUser); err != nil {
		s.Fatal("Failed to unmount vault after post-migration mount: ", err)
	}
}
