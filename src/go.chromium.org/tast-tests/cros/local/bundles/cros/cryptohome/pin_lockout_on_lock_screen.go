// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cryptohome

import (
	"context"
	"time"

	uda "chromiumos/system_api/user_data_auth_proto"

	"go.chromium.org/tast-tests/cros/common/hwsec"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	hwseclocal "go.chromium.org/tast-tests/cros/local/hwsec"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PinLockoutOnLockScreen,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test that checks pin counter mechanism on lock screen",
		Contacts: []string{
			"cryptohome-core@google.com",
			"hardikgoyal@google.com",
		},
		BugComponent: "b:1088399", // ChromeOS > Security > Cryptohome
		SoftwareDeps: []string{"chrome", "pinweaver"},
		Attr:         []string{"group:mainline", "informational", "group:cryptohome"},
	})
}

// PinLockoutOnLockScreen tests the pin is unlocked on lock screen upon pin lockout + correct password auth.
func PinLockoutOnLockScreen(ctx context.Context, s *testing.State) {
	const (
		userName      = "test@test.com"
		userPassword  = "secret"
		wrongPassword = "wrong secret"
		userPin       = "1234"
		wrongPin      = "1111"
		passwordLabel = "gaia"
		pinLabel      = "pin"
	)

	ctxForCleanup := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cmdRunner := hwseclocal.NewCmdRunner()
	client := hwsec.NewCryptohomeClient(cmdRunner)

	// Wait for cryptohomed to become available if needed.
	if err := cryptohome.CheckService(ctx); err != nil {
		s.Fatal("Failed to ensure cryptohomed: ", err)
	}

	// Clean up obsolete state, in case there's any.
	if err := client.UnmountAll(ctx); err != nil {
		s.Error("Failed to unmount vaults for preparation: ", err)
	}
	if _, err := client.RemoveVault(ctx, userName); err != nil {
		s.Fatal("Failed to remove old vault for preparation: ", err)
	}

	// Setup user with password and pin.
	if err := client.WithAuthSession(ctx, userName, false /*isEphemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT, func(authSessionID string) error {
		// Create user vault.
		if err := client.CreatePersistentUser(ctx, authSessionID); err != nil {
			return errors.Wrap(err, "failed to create user")
		}
		// Mount user home directories and daemon-store directories.
		if _, err := client.PreparePersistentVault(ctx, authSessionID, false /*ecryptfs*/); err != nil {
			return errors.Wrap(err, "failed to mount user profile after creation")
		}
		defer client.Unmount(ctxForCleanup, userName)
		// Add password AuthFactor.
		if err := client.AddAuthFactor(ctx, authSessionID, passwordLabel, userPassword); err != nil {
			return errors.Wrap(err, "failed to add password AuthFactor")
		}
		// Add PIN AuthFactor.
		if err := client.AddPinAuthFactor(ctx, authSessionID, pinLabel, userPin); err != nil {
			return errors.Wrap(err, "failed to add PIN AuthFactor")
		}
		return nil
	}); err != nil {
		s.Fatal("Failed to create and set up the user password and PIN: ", err)
	}
	// Cleanup user vault before UssMigrationPasswordPin exits.
	defer client.RemoveVault(ctxForCleanup, userName)

	// Mount and then perform lock screen actions.
	if err := client.WithAuthSession(ctx, userName, false /*isEphemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT, func(authSessionID string) error {
		// Authenticate with password to use password verifier in verify intent auth.
		if _, err := client.AuthenticateAuthFactor(ctx, authSessionID, passwordLabel, userPassword); err != nil {
			return errors.Wrap(err, "failed to authenticate with password AuthFactor")
		}

		// Mount user home directories and daemon-store directories.
		if _, err := client.PreparePersistentVault(ctx, authSessionID, false /*ecryptfs*/); err != nil {
			return errors.Wrap(err, "failed to mount user profile after creation")
		}
		defer client.Unmount(ctxForCleanup, userName)

		// Test that PIN reset with password.
		if err := cryptohome.TestPinCounterWithAuthSession(ctx, authSessionID, passwordLabel, userPassword, pinLabel, userPin, wrongPin, client); err != nil {
			return errors.Wrap(err, "failed in testing PIN lockout and reset mechanism with decrypt intent")
		}

		// Test pin counter mechanism with lock screen.
		if err := client.WithAuthSession(ctx, userName, false /*isEphemeral*/, uda.AuthIntent_AUTH_INTENT_VERIFY_ONLY, func(authSessionID string) error {
			// Test that PIN reset with correct password works after the update.
			if err := cryptohome.TestPinCounterWithAuthSession(ctx, authSessionID, passwordLabel, userPassword, pinLabel, userPin, wrongPin, client); err != nil {
				return errors.Wrap(err, "failed in testing PIN lockout and reset mechanism with verify intent")
			}
			return nil
		}); err != nil {
			return errors.Wrap(err, "failed to check pin counter mechanism")
		}
		return nil
	}); err != nil {
		s.Fatal("Failed to verify pin counter mechanism: ", err)
	}
}
