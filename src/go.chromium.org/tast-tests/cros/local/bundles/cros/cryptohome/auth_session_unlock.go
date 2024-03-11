// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cryptohome

import (
	"context"
	"time"

	uda "go.chromium.org/chromiumos/system_api/user_data_auth_proto"

	cryptohomecommon "go.chromium.org/tast-tests/cros/common/cryptohome"
	"go.chromium.org/tast-tests/cros/common/hwsec"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	hwseclocal "go.chromium.org/tast-tests/cros/local/hwsec"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: AuthSessionUnlock,
		Desc: "Check session unlock via AuthSession",
		Contacts: []string{
			"cryptohome-core@google.com",
			"emaxx@chromium.org",
		},
		BugComponent: "b:1088399", // ChromeOS > Security > Cryptohome
		Attr:         []string{"group:mainline", "group:cryptohome", "group:hw_agnostic"},
		Fixture:      "ussAuthSessionFixture",
	})
}

func AuthSessionUnlock(ctx context.Context, s *testing.State) {
	const (
		userName                = "foo@bar.baz"
		userPassword            = "secret"
		userPasswordAfterUpdate = "i-forgot-secret"
		secondUserName          = "doo@bar.baz"
		secondPassword          = "different-secret"
		passwordLabel           = "online-password"
	)

	ctxForCleanUp := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cmdRunner := hwseclocal.NewCmdRunner()
	client := hwsec.NewCryptohomeClient(cmdRunner)

	// verifyPassword will create a verify-only session that will ensure that
	// the given user cannot authenticate with the given "wrong" passwords and
	// can authenticate with the correct one. It will return an error on
	// failure and nil on success.
	verifyPassword := func(user, password, badPassword, otherUsersPassword string) error {
		return client.WithAuthSession(ctx, user, false /*isEphemeral*/, uda.AuthIntent_AUTH_INTENT_VERIFY_ONLY, func(authSessionID string) error {
			if _, err := client.AuthenticateAuthFactor(ctx, authSessionID, passwordLabel, badPassword); err == nil {
				return errors.New("authenticated user with the wrong password")
			}
			if _, err := client.AuthenticateAuthFactor(ctx, authSessionID, passwordLabel, otherUsersPassword); err == nil {
				return errors.New("authenticated user with the other user's password which should have failed")
			}
			authReply, err := client.AuthenticateAuthFactor(ctx, authSessionID, passwordLabel, password)
			if err != nil {
				return errors.Wrap(err, "failed to authenticate user")
			}
			if err := cryptohomecommon.ExpectContainsAuthIntent(
				authReply.AuthorizedFor, uda.AuthIntent_AUTH_INTENT_VERIFY_ONLY,
			); err != nil {
				return errors.Wrap(err, "unexpected AuthSession authorized intents")
			}
			return nil
		})
	}

	// Clean up old state or mounts for the test user, if any exists.
	if err := client.UnmountAll(ctx); err != nil {
		s.Fatal("Failed to unmount vaults for preparation: ", err)
	}
	if err := cryptohome.RemoveVault(ctx, userName); err != nil {
		s.Fatal("Failed to remove old vault for preparation: ", err)
	}
	if err := cryptohome.RemoveVault(ctx, secondUserName); err != nil {
		s.Fatal("Failed to remove old vault for preparation: ", err)
	}

	if err := client.WithAuthSession(ctx, userName, false /*isEphemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT, func(authSessionID string) error {
		replyWithError, err := client.AuthenticateAuthFactor(ctx, authSessionID, passwordLabel, userPassword)
		if err == nil {
			return errors.New("auth with non-existent user succeeded with AuthenticateAuthFactor")
		}
		if replyWithError == nil {
			return errors.Wrap(err, "invalid reply from AuthenticateAuthFactor when checking non-existent user")
		}
		if err = hwsec.CheckForPossibleAction(replyWithError.GetErrorInfo(), uda.PossibleAction_POSSIBLY_DEV_CHECK_UNEXPECTED_STATE); err != nil {
			return errors.Wrap(err, "non-existent user with AuthenticateAuthFactor doesn't raise PossibleAction_POSSIBLY_DEV_CHECK_UNEXPECTED_STATE")
		}

		return nil
	}); err != nil {
		s.Fatal("AuthenticateAuthFactor with non-existent user results in incorrect behaviour: ", err)
	}

	// Create and mount the user with a password auth factor.
	if err := client.WithAuthSession(ctx, userName, false /*isEphemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT, func(authSessionID string) error {
		if err := client.CreatePersistentUser(ctx, authSessionID); err != nil {
			return errors.Wrap(err, "failed to create persistent user")
		}
		if _, err := client.PreparePersistentVault(ctx, authSessionID, false /*ecryptfs*/); err != nil {
			return errors.Wrap(err, "failed to prepare new persistent vault")
		}
		if err := client.AddAuthFactor(ctx, authSessionID, passwordLabel, userPassword); err != nil {
			return errors.Wrap(err, "failed to add initial user password")
		}
		return nil
	}); err != nil {
		s.Fatal("Failed to create and set up the user: ", err)
	}
	defer cryptohome.RemoveVault(ctxForCleanUp, userName)

	// Create and mount the user with a second password auth factor.
	if err := client.WithAuthSession(ctx, secondUserName, false /*isEphemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT, func(authSessionID string) error {
		if err := client.CreatePersistentUser(ctx, authSessionID); err != nil {
			return errors.Wrap(err, "failed to create persistent user")
		}
		if _, err := client.PreparePersistentVault(ctx, authSessionID, false /*ecryptfs*/); err != nil {
			return errors.Wrap(err, "failed to prepare new persistent vault")
		}
		if err := client.AddAuthFactor(ctx, authSessionID, passwordLabel, secondPassword); err != nil {
			return errors.Wrap(err, "failed to add initial user password")
		}
		return nil
	}); err != nil {
		s.Fatal("Failed to create and set up the second user: ", err)
	}
	defer cryptohome.RemoveVault(ctxForCleanUp, secondUserName)

	// Verify that the user passwords can be used to authenticate.
	if err := verifyPassword(userName, userPassword, userPasswordAfterUpdate, secondPassword); err != nil {
		s.Fatal("Failed to authenticate first user with initial password: ", err)
	}
	if err := verifyPassword(secondUserName, secondPassword, userPasswordAfterUpdate, userPassword); err != nil {
		s.Fatal("Failed to authenticate second user: ", err)
	}

	// Change the user's password.
	if err := client.WithAuthSession(ctx, userName, false /*isEphemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT, func(authSessionID string) error {
		authReply, err := client.AuthenticateAuthFactor(ctx, authSessionID, passwordLabel, userPassword)
		if err != nil {
			return errors.Wrap(err, "failed to authenticate user")
		}
		if err := cryptohomecommon.ExpectContainsAuthIntent(
			authReply.AuthorizedFor, uda.AuthIntent_AUTH_INTENT_DECRYPT,
		); err != nil {
			return errors.Wrap(err, "unexpected AuthSession authorized intents")
		}
		if err := client.UpdatePasswordAuthFactor(ctx, authSessionID, passwordLabel, userPasswordAfterUpdate); err != nil {
			return errors.Wrap(err, "failed to update password")
		}
		return nil
	}); err != nil {
		s.Fatal("Failed to change the user password: ", err)
	}

	// Verify the new password can be used to authenticate.
	if err := verifyPassword(userName, userPasswordAfterUpdate, userPassword, secondPassword); err != nil {
		s.Fatal("Failed to authenticate with changed password: ", err)
	}
}
