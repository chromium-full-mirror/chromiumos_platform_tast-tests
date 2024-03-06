// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cryptohome

import (
	"context"

	uda "go.chromium.org/chromiumos/system_api/user_data_auth_proto"

	"go.chromium.org/tast-tests/cros/common/hwsec"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	hwseclocal "go.chromium.org/tast-tests/cros/local/hwsec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: KioskPersistentMount,
		Desc: "Ensures that cryptohome correctly mounts kiosk sessions with persistent vaults",
		Contacts: []string{
			"cryptohome-core@google.com",
			"hardikgoyal@chromium.org",
		},
		BugComponent: "b:1088399", // ChromeOS > Security > Cryptohome
		Attr:         []string{"group:mainline", "group:cryptohome", "group:hw_agnostic", "informational"},
		Fixture:      "ussAuthSessionFixture",
	})
}

func KioskPersistentMount(ctx context.Context, s *testing.State) {
	cmdRunner := hwseclocal.NewCmdRunner()
	client := hwsec.NewCryptohomeClient(cmdRunner)

	// Clean up old state or mounts for the test user, if any exists.
	if err := client.UnmountAll(ctx); err != nil {
		s.Fatal("Failed to unmount vaults for preparation: ", err)
	}
	if err := cryptohome.RemoveVault(ctx, cryptohome.KioskUser); err != nil {
		s.Fatal("Failed to remove old vault for preparation: ", err)
	}

	// Create the user with a persistent vault and add a kiosk credential.
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
		if err := cryptohome.WriteFileForPersistence(ctx, cryptohome.KioskUser); err != nil {
			return errors.Wrap(err, "failed to write test file")
		}
		return nil
	}); err != nil {
		s.Fatal("Failed to create and set up the user: ", err)
	}

	// Unmount all user vaults before we start.
	if err := cryptohome.UnmountAll(ctx); err != nil {
		s.Fatal("Failed to unmount all before test starts: ", err)
	}

	// Start a new auth session and mount the persistent vault.
	if err := client.WithAuthSession(ctx, cryptohome.KioskUser, false /*ephemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT, func(authSessionID string) error {
		if err := client.AuthenticateKioskAuthFactor(ctx, authSessionID); err != nil {
			return errors.Wrap(err, "failed to authenticate with kiosk credential")
		}
		if err := cryptohome.MountAndVerify(ctx, cryptohome.KioskUser, authSessionID, false /*ecryptfs*/); err != nil {
			return errors.Wrap(err, "failed to mount and verify persistence")
		}
		return nil
	}); err != nil {
		s.Fatal("Failed to authenticate and mount the user vault: ", err)
	}

	// Unmount user vault.
	if err := cryptohome.UnmountVault(ctx, cryptohome.KioskUser); err != nil {
		s.Fatal("Failed to unmount vault after remount: ", err)
	}
}
