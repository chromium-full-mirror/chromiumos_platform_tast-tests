// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cryptohome

import (
	"context"
	"time"

	uda "chromiumos/system_api/user_data_auth_proto"

	cryptocommon "go.chromium.org/tast-tests/cros/common/cryptohome"
	"go.chromium.org/tast-tests/cros/common/hwsec"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	hwseclocal "go.chromium.org/tast-tests/cros/local/hwsec"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type testParam struct {
	disableKeyData bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func: LegacyLabelAuthSession,
		Desc: "Test AuthSession with a cryptohome created via legacy APIs without key labels",
		Contacts: []string{
			"cryptohome-core@google.com",
			"emaxx@chromium.org",
		},
		BugComponent: "b:1088399", // ChromeOS > Security > Cryptohome
		Attr:         []string{"group:mainline", "group:cryptohome"},
		Params: []testing.Param{
			{
				Name: "with_key_data",
				Val:  testParam{disableKeyData: false},
			}, {
				Name: "without_key_data",
				Val:  testParam{disableKeyData: true},
			},
		},
		Timeout: 60 * time.Second,
	})
}

func LegacyLabelAuthSession(ctx context.Context, s *testing.State) {
	const (
		userName       = "foo@bar.baz"
		userPassword   = "secret"
		wrongPassword  = "wrong-password"
		legacyKeyLabel = "legacy-0"
		wrongKeyLabel  = "gaia"
	)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cmdRunner := hwseclocal.NewCmdRunner()
	testTool, err := cryptocommon.NewRecoveryTestToolWithFakeMediator(cmdRunner)
	if err != nil {
		s.Fatal("Failed to initialize RecoveryTestTool: ", err)
	}

	// Ensure cryptohomed is started and wait for it to be available.
	if err := cryptohome.CheckService(ctx); err != nil {
		s.Fatal("Failed to ensure cryptohomed: ", err)
	}

	client := hwsec.NewCryptohomeClient(cmdRunner)
	if err := client.UnmountAll(ctx); err != nil {
		s.Fatal("Failed to unmount vaults for preparation: ", err)
	}

	if err := cryptohome.RemoveVault(ctx, userName); err != nil {
		s.Fatal("Failed to remove old vault for preparation: ", err)
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
		if err := cryptocommon.ExpectAuthFactorsWithTypeAndLabel(
			listFactorsReply.ConfiguredAuthFactorsWithStatus, expectedConfigured); err != nil {
			return errors.Wrap(err, "mismatch in configured auth factors (-got, +want)")
		}
		if err := cryptocommon.ExpectContainsAuthFactorType(
			listFactorsReply.SupportedAuthFactors,
			uda.AuthFactorType_AUTH_FACTOR_TYPE_PASSWORD,
		); err != nil {
			return errors.Wrap(err, "mismatch in supported auth factors")
		}
		return nil
	}

	// Create the user with a persistent vault and add a kiosk credential.
	// This should produce a VK factor.
	if err := client.WithAuthSession(ctx, userName, false /*ephemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT, func(authSessionID string) error {
		if err := client.CreatePersistentUser(ctx, authSessionID); err != nil {
			return errors.Wrap(err, "failed to create persistent user")
		}
		if _, err := client.PreparePersistentVault(ctx, authSessionID, false /*ecryptfs*/); err != nil {
			return errors.Wrap(err, "failed to prepare new persistent vault")
		}

		// Create persistent user with a vault keyset that has an empty label.
		disableKeyData := s.Param().(testParam).disableKeyData
		if err := testTool.CreateVaultKeyset(ctx, authSessionID, userPassword /*keyDataLabel=*/, "", uda.AuthFactorType_AUTH_FACTOR_TYPE_PASSWORD, disableKeyData); err != nil {
			return errors.Wrap(err, "failed to create VaultKeyset")
		}

		if err := checkListAuthFactors(legacyKeyLabel); err != nil {
			return errors.Wrap(err, "failed list auth factor checks")
		}

		if err := client.UnmountAll(ctx); err != nil {
			return errors.Wrap(err, "failed to unmount vaults for re-mounting")
		}
		return nil
	}); err != nil {
		s.Fatal("Failed to create and set up the user: ", err)
	}
	defer cryptohome.RemoveVault(cleanupCtx, userName)

	// Verify authentication using the new APIs.
	if err := client.WithAuthSession(ctx, userName, false /*isEphemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT, func(authSessionID string) error {
		// Verify authenticate fails if no or incorrect label is passed.
		if _, err = client.AuthenticateAuthFactor(ctx, authSessionID, "" /*keyLabel*/, userPassword); err == nil {
			return errors.Wrap(err, "authentication with empty label succeeded unexpectedly")
		}
		if _, err = client.AuthenticateAuthFactor(ctx, authSessionID, wrongKeyLabel, userPassword); err == nil {
			return errors.Wrap(err, "authentication with incorrect label succeeded unexpectedly")
		}
		// Verify authentication succeeds if the legacy label is explicitly passed.
		if _, err = client.AuthenticateAuthFactor(ctx, authSessionID, legacyKeyLabel, userPassword); err != nil {
			return errors.Wrap(err, "failed to authenticate user")
		}
		// Verify mounting succeeds.
		if _, err := client.PreparePersistentVault(ctx, authSessionID, false); err != nil {
			return errors.Wrap(err, "failed to prepare persistent vault")
		}
		return nil
	}); err != nil {
		s.Fatal("Failed to authenticate the legacy user: ", err)
	}
	defer client.UnmountAll(cleanupCtx)

	// Verify lock-screen check passes with an explicit or wildcard label.
	accepted, err := client.CheckVault(ctx, legacyKeyLabel, hwsec.NewPassAuthConfig(userName, userPassword))
	if err != nil {
		s.Fatal("Failed to check key with explicit label: ", err)
	}
	if !accepted {
		s.Fatal("Key check with explicit label failed despite no error")
	}
	accepted, err = client.CheckVault(ctx /*keyLabel=*/, "", hwsec.NewPassAuthConfig(userName, userPassword))
	if err == nil {
		s.Fatal("Key check with empty label succeeded when it shouldn't")
	}
	if accepted {
		s.Fatal("Wildcard label accepted when AuthFactor should not accept empty label")
	}

	// Verify lock-screen check fails with wrong password or wrong label.
	accepted, err = client.CheckVault(ctx, wrongKeyLabel, hwsec.NewPassAuthConfig(userName, userPassword))
	if err == nil {
		s.Fatal("Key check with wrong label succeeded when it shouldn't")
	}
	if accepted {
		s.Fatal("Key check with wrong label succeeded despite an error")
	}
	accepted, err = client.CheckVault(ctx, legacyKeyLabel, hwsec.NewPassAuthConfig(userName, wrongPassword))
	if err == nil {
		s.Fatal("Key check with wrong password succeeded when it shouldn't")
	}
	if accepted {
		s.Fatal("Key check with wrong password succeeded despite an error")
	}
}
