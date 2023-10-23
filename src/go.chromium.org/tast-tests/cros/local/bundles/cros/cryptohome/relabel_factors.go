// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cryptohome

import (
	"context"

	uda "chromiumos/system_api/user_data_auth_proto"

	cryptohomecommon "go.chromium.org/tast-tests/cros/common/cryptohome"
	"go.chromium.org/tast-tests/cros/common/hwsec"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	hwseclocal "go.chromium.org/tast-tests/cros/local/hwsec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: RelabelFactors,
		Desc: "Test relabelling an auth factor",
		Contacts: []string{
			"cryptohome-core@google.com",
			"jadmanski@chromium.org",
		},
		BugComponent: "b:1088399", // ChromeOS > Security > Cryptohome
		Attr:         []string{"group:mainline", "group:cryptohome", "informational"},
		Fixture:      "ussAuthSessionFixture",
	})
}

func RelabelFactors(ctx context.Context, s *testing.State) {
	const (
		userPassword     = "secret"
		passwordLabel    = "online-password"
		newPasswordLabel = "offline-password"
	)

	fixture := s.FixtValue().(*cryptohome.AuthSessionFixture)
	userName := fixture.TestUserName

	cmdRunner := hwseclocal.NewCmdRunner()
	client := hwsec.NewCryptohomeClient(cmdRunner)

	// Wait for cryptohomed becomes available if needed.
	if err := cryptohome.CheckService(ctx); err != nil {
		s.Fatal("Failed to ensure cryptohomed: ", err)
	}

	// Helper function that will call ListAuthFactors and check that:
	//   - the configured factors are just a single password with the given label
	//   - the supported factors always contains password
	checkListAuthFactors := func(label string) error {
		listFactorsReply, err := client.ListAuthFactors(ctx, userName)
		if err != nil {
			return errors.Wrap(err, "failed to list auth factors")
		}
		expectedConfigured := []*uda.AuthFactorWithStatus{
			{AuthFactor: &uda.AuthFactor{
				Type:  uda.AuthFactorType_AUTH_FACTOR_TYPE_PASSWORD,
				Label: label,
			}},
		}
		if err := cryptohomecommon.ExpectAuthFactorsWithTypeAndLabel(
			listFactorsReply.ConfiguredAuthFactorsWithStatus, expectedConfigured); err != nil {
			return errors.Wrap(err, "mismatch in configured auth factors (-got, +want)")
		}
		if err := cryptohomecommon.ExpectContainsAuthFactorType(
			listFactorsReply.SupportedAuthFactors,
			uda.AuthFactorType_AUTH_FACTOR_TYPE_PASSWORD,
		); err != nil {
			return errors.Wrap(err, "mismatch in supported auth factors")
		}
		return nil
	}

	// Create and mount the persistent user with a password auth factor.
	if err := client.WithAuthSession(ctx, userName, false /*isEphemeral*/, uda.AuthIntent_AUTH_INTENT_VERIFY_ONLY, func(authSessionID string) error {
		if err := client.CreatePersistentUser(ctx, authSessionID); err != nil {
			return errors.Wrap(err, "failed to create persistent user")
		}
		if _, err := client.PreparePersistentVault(ctx, authSessionID, false /*ecryptfs*/); err != nil {
			return errors.Wrap(err, "failed to prepare new persistent vault")
		}
		if err := client.AddAuthFactor(ctx, authSessionID, passwordLabel, userPassword); err != nil {
			return errors.Wrap(err, "failed to add password auth factor")
		}
		return nil
	}); err != nil {
		s.Fatal("Failed to set up a user with a password: ", err)
	}
	if err := checkListAuthFactors(passwordLabel); err != nil {
		s.Fatal("Failed list auth factor checks after adding password: ", err)
	}

	// Verify that the user password can be used to authenticate.
	if err := client.WithAuthSession(ctx, userName, false /*isEphemeral*/, uda.AuthIntent_AUTH_INTENT_VERIFY_ONLY, func(authSessionID string) error {
		if _, err := client.AuthenticateAuthFactor(ctx, authSessionID, newPasswordLabel, userPassword); err == nil {
			return errors.New("renamed label worked before the rename")
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
		s.Fatal("Failed to authenticate user with original label: ", err)
	}

	// Change the password auth factor label.
	if err := client.WithAuthSession(ctx, userName, false /*isEphemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT, func(authSessionID string) error {
		if _, err := client.AuthenticateAuthFactor(ctx, authSessionID, passwordLabel, userPassword); err != nil {
			return errors.Wrap(err, "failed to authenticate user")
		}
		if err := client.RelabelAuthFactor(ctx, authSessionID, passwordLabel, newPasswordLabel); err != nil {
			return errors.Wrap(err, "RelabelAuthFactor operation failed")
		}
		return nil
	}); err != nil {
		s.Fatal("Failed to relabel auth factor: ", err)
	}
	if err := checkListAuthFactors(newPasswordLabel); err != nil {
		s.Fatal("Failed list auth factor checks after relabelling: ", err)
	}

	// Verify that the user password can be used to authenticate with the new label.
	if err := client.WithAuthSession(ctx, userName, false /*isEphemeral*/, uda.AuthIntent_AUTH_INTENT_VERIFY_ONLY, func(authSessionID string) error {
		if _, err := client.AuthenticateAuthFactor(ctx, authSessionID, passwordLabel, userPassword); err == nil {
			return errors.New("old label still unexpectedly works")
		}
		authReply, err := client.AuthenticateAuthFactor(ctx, authSessionID, newPasswordLabel, userPassword)
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
		s.Fatal("Failed to authenticate user with new label: ", err)
	}
}
