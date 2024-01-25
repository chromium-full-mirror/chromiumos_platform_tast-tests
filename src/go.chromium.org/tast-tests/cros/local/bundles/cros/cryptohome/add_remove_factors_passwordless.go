// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cryptohome

import (
	"context"

	uda "go.chromium.org/chromiumos/system_api/user_data_auth_proto"

	cryptohomecommon "go.chromium.org/tast-tests/cros/common/cryptohome"
	"go.chromium.org/tast-tests/cros/common/hwsec"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	hwseclocal "go.chromium.org/tast-tests/cros/local/hwsec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type addRemoveFactorsPasswordlessParam struct {
	// If set, the test should use passwords before and after the PIN.
	usePassword bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func: AddRemoveFactorsPasswordless,
		Desc: "Test adding, removing, and listing auth factors without a password",
		Contacts: []string{
			"cryptohome-core@google.com",
			"jadmanski@chromium.org",
		},
		BugComponent: "b:1088399", // ChromeOS > Security > Cryptohome
		Attr:         []string{"group:mainline", "group:cryptohome", "informational"},
		SoftwareDeps: []string{"pinweaver"},
		Fixture:      "ussAuthSessionFixture",
		Params: []testing.Param{{
			Name: "pin_only",
			Val: addRemoveFactorsPasswordlessParam{
				usePassword: false,
			},
		}, {
			Name: "password_first",
			Val: addRemoveFactorsPasswordlessParam{
				usePassword: true,
			},
		}},
	})
}

func AddRemoveFactorsPasswordless(ctx context.Context, s *testing.State) {
	const (
		userPassword  = "secret"
		passwordLabel = "online-password"
		userPin       = "12345"
		pinLabel      = "luggage-pin"
	)

	fixture := s.FixtValue().(*cryptohome.AuthSessionFixture)
	userName := fixture.TestUserName
	userParam := s.Param().(addRemoveFactorsPasswordlessParam)

	cmdRunner := hwseclocal.NewCmdRunner()
	client := hwsec.NewCryptohomeClient(cmdRunner)

	// Create and mount the persistent user.
	_, authSessionID, err := client.StartAuthSession(ctx, userName, false /*ephemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT)
	if err != nil {
		s.Fatal("Failed to start auth session: ", err)
	}
	if err := client.CreatePersistentUser(ctx, authSessionID); err != nil {
		s.Fatal("Failed to create persistent user: ", err)
	}
	if _, err := client.PreparePersistentVault(ctx, authSessionID, false /*ecryptfs*/); err != nil {
		s.Fatal("Failed to prepare new persistent vault: ", err)
	}

	// Helper function that will call ListAuthFactors and check that:
	//   - the configured factors exactly match expectedConfigured
	//   - the supported factors always contain password and PIN
	checkListAuthFactors := func(expectedConfigured []*uda.AuthFactorWithStatus) error {
		listFactorsReply, err := client.ListAuthFactors(ctx, userName)
		if err != nil {
			return errors.Wrap(err, "failed to list auth factors")
		}
		if err := cryptohomecommon.ExpectAuthFactorsWithTypeAndLabel(
			listFactorsReply.ConfiguredAuthFactorsWithStatus, expectedConfigured); err != nil {
			return errors.Wrap(err, "mismatch in configured auth factors (-got, +want)")
		}
		if err := cryptohomecommon.ExpectContainsAuthFactorTypes(
			listFactorsReply.SupportedAuthFactors,
			[]uda.AuthFactorType{uda.AuthFactorType_AUTH_FACTOR_TYPE_PASSWORD, uda.AuthFactorType_AUTH_FACTOR_TYPE_PIN},
		); err != nil {
			return errors.Wrap(err, "mismatch in supported auth factors")
		}
		return nil
	}

	// List the auth factors before we've added any factors.
	if err := checkListAuthFactors(nil); err != nil {
		s.Fatal("Failed list auth factor checks before adding any factors: ", err)
	}

	if userParam.usePassword {
		// Add a password auth factor to the user.
		if err := client.AddAuthFactor(ctx, authSessionID, passwordLabel, userPassword); err != nil {
			s.Fatal("Failed to add password auth factor: ", err)
		}
		if err := checkListAuthFactors([]*uda.AuthFactorWithStatus{
			{AuthFactor: &uda.AuthFactor{
				Type:  uda.AuthFactorType_AUTH_FACTOR_TYPE_PASSWORD,
				Label: passwordLabel,
			}},
		}); err != nil {
			s.Fatal("Failed list auth factor checks after adding password: ", err)
		}

		// Add a PIN auth factor to the user.
		if err := client.AddPinAuthFactor(ctx, authSessionID, pinLabel, userPin); err != nil {
			s.Fatal("Failed to add PIN auth factor: ", err)
		}
		if err := checkListAuthFactors([]*uda.AuthFactorWithStatus{
			{AuthFactor: &uda.AuthFactor{
				Type:  uda.AuthFactorType_AUTH_FACTOR_TYPE_PASSWORD,
				Label: passwordLabel,
			}},
			{AuthFactor: &uda.AuthFactor{
				Type:  uda.AuthFactorType_AUTH_FACTOR_TYPE_PIN,
				Label: pinLabel,
			}},
		}); err != nil {
			s.Fatal("Failed list auth factor checks after adding PIN: ", err)
		}

		// Remove the password auth factor, leaving only PIN.
		if err := client.RemoveAuthFactor(ctx, authSessionID, passwordLabel); err != nil {
			s.Fatal("Failed to remove password factor: ", err)
		}
		if err := checkListAuthFactors([]*uda.AuthFactorWithStatus{
			{AuthFactor: &uda.AuthFactor{
				Type:  uda.AuthFactorType_AUTH_FACTOR_TYPE_PIN,
				Label: pinLabel,
			}},
		}); err != nil {
			s.Fatal("Failed list auth factor checks after removing password: ", err)
		}
	} else {
		// Add a PIN auth factor to the user.
		if err := client.AddPinAuthFactor(ctx, authSessionID, pinLabel, userPin); err != nil {
			s.Fatal("Failed to add PIN auth factor: ", err)
		}
		if err := checkListAuthFactors([]*uda.AuthFactorWithStatus{
			{AuthFactor: &uda.AuthFactor{
				Type:  uda.AuthFactorType_AUTH_FACTOR_TYPE_PIN,
				Label: pinLabel,
			}},
		}); err != nil {
			s.Fatal("Failed list auth factor checks after adding PIN: ", err)
		}
	}
}
