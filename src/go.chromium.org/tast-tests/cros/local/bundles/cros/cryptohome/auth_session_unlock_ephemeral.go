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

func init() {
	testing.AddTest(&testing.Test{
		Func: AuthSessionUnlockEphemeral,
		Desc: "Check session unlock for ephemeral users via AuthSession",
		Contacts: []string{
			"cryptohome-core@google.com",
			"emaxx@chromium.org",
		},
		BugComponent: "b:1088399", // ChromeOS > Security > Cryptohome
		Attr:         []string{"group:mainline", "group:cryptohome"},
		Data:         []string{"testcert.p12"},
		Fixture:      "ussAuthSessionFixture",
	})
}

func AuthSessionUnlockEphemeral(ctx context.Context, s *testing.State) {
	const (
		ownerName       = "owner@bar.baz"
		userName        = "foo@bar.baz"
		userPassword    = "secret"
		badUserPassword = "i-forgot-secret"
		passwordLabel   = "online-password"
	)

	ctxForCleanUp := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cmdRunner := hwseclocal.NewCmdRunner()
	client := hwsec.NewCryptohomeClient(cmdRunner)

	// Clean up old state or mounts for the test user, if any exists.
	if err := client.UnmountAll(ctx); err != nil {
		s.Fatal("Failed to unmount vaults for preparation: ", err)
	}
	if err := cryptohome.RemoveVault(ctx, userName); err != nil {
		s.Fatal("Failed to remove old vault for preparation: ", err)
	}

	// Set up an owner. This is needed for ephemeral users. Once this is done
	// unmount everything to put things in a clean state for the test proper.
	if err := hwseclocal.SetUpVaultAndUserAsOwner(ctx, s.DataPath("testcert.p12"), ownerName, "whatever", "whatever", client); err != nil {
		client.UnmountAll(ctx)
		client.RemoveVault(ctx, ownerName)
		s.Fatal("Failed to setup vault and user as owner: ", err)
	}
	if err := client.UnmountAll(ctx); err != nil {
		s.Fatal("Failed to unmount vaults for preparation: ", err)
	}
	defer client.RemoveVault(ctxForCleanUp, ownerName)

	// Create and mount the user with a password auth factor.
	if err := client.WithAuthSession(ctx, userName, true /*isEphemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT, func(authSessionID string) error {
		if err := client.PrepareEphemeralVault(ctx, authSessionID); err != nil {
			return errors.Wrap(err, "failed to prepare new ephemeral vault")
		}
		if err := client.AddAuthFactor(ctx, authSessionID, passwordLabel, userPassword); err != nil {
			return errors.Wrap(err, "failed to add initial user password")
		}
		return nil
	}); err != nil {
		s.Fatal("Failed to create and set up the user: ", err)
	}
	defer cryptohome.RemoveVault(ctxForCleanUp, userName)

	// Verify that the user password can be used to authenticate.
	if err := client.WithAuthSession(ctx, userName, true /*isEphemeral*/, uda.AuthIntent_AUTH_INTENT_VERIFY_ONLY, func(authSessionID string) error {
		if _, err := client.AuthenticateAuthFactor(ctx, authSessionID, passwordLabel, badUserPassword); err == nil {
			return errors.New("authenticated user with the wrong password")
		}
		authReply, err := client.AuthenticateAuthFactor(ctx, authSessionID, passwordLabel, userPassword)
		if err != nil {
			return errors.Wrap(err, "failed to authenticate user")
		}
		if err := cryptohomecommon.ExpectContainsAuthIntent(
			authReply.AuthorizedFor, uda.AuthIntent_AUTH_INTENT_VERIFY_ONLY,
		); err != nil {
			return errors.Wrap(err, "unexpected AuthSession authorized intents")
		}
		return nil
	}); err != nil {
		s.Fatal("Failed to authenticate first user with initial password: ", err)
	}
}
