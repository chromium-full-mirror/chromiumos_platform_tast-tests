// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cryptohome

import (
	"context"
	"strconv"
	"strings"

	biod "chromiumos/system_api/biod_messages_proto"
	uda "chromiumos/system_api/user_data_auth_proto"

	cryptohomecommon "go.chromium.org/tast-tests/cros/common/cryptohome"
	hwseclocal "go.chromium.org/tast-tests/cros/local/hwsec"

	"go.chromium.org/tast-tests/cros/common/hwsec"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: AddFingerprintFactor,
		Desc: "Checks that cryptohome fingerprint enrollment process succeeds with fake auth stack",
		Contacts: []string{
			"cryptohome-core@google.com",
			"lziest@google.com",
		},
		BugComponent: "b:1088399", // ChromeOS > Security > Cryptohome
		Attr:         []string{"group:cryptohome"},
		SoftwareDeps: []string{"pinweaver"},
		Fixture:      "ussAuthSessionFakeBiometricsFixture",
	})
}

type addFingerprintTestCase struct {
	Label                string
	EnrollmentProgresses []cryptohome.EnrollmentProgress
	CreateCredStatus     biod.CreateCredentialReply_CreateCredentialStatus
	ExpectedErrorCode    uda.CryptohomeErrorCode
}

func AddFingerprintFactor(ctx context.Context, s *testing.State) {
	const (
		userName      = "foo@bar.baz"
		userPassword  = "secret"
		passwordLabel = "online-password"
	)
	f, ok := s.FixtValue().(*cryptohome.BiometricsFixture)
	if !ok {
		s.Fatal("Test fixture is not BiometricsFixture")
	}
	fasm := f.FakeAuthStackManager
	if err := cryptohome.RemoveVault(ctx, userName); err != nil {
		s.Fatal("Failed to remove old vault for preparation: ", err)
	}

	cmdRunner := hwseclocal.NewCmdRunner()
	client := hwsec.NewCryptohomeClient(cmdRunner)

	// Create and mount the persistent user.
	if err := client.WithAuthSession(ctx, userName, false /*ephemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT, func(authSessionID string) error {
		if err := client.CreatePersistentUser(ctx, authSessionID); err != nil {
			return errors.Wrap(err, "failed to create persistent user")
		}

		if _, err := client.PreparePersistentVault(ctx, authSessionID, false /*ecryptfs*/); err != nil {
			return errors.Wrap(err, "failed to prepare new persistent vault")
		}
		// Add a password auth factor to the user.
		if err := client.AddAuthFactor(ctx, authSessionID, passwordLabel, userPassword); err != nil {
			return errors.Wrap(err, "failed to add a password authfactor")
		}
		return nil
	}); err != nil {
		s.Fatal("Failed to setup a persistent user: ", err)
	}

	expectedConfiguredFactors := []*uda.AuthFactorWithStatus{{
		AuthFactor: &uda.AuthFactor{
			Type:  uda.AuthFactorType_AUTH_FACTOR_TYPE_PASSWORD,
			Label: passwordLabel,
		},
	}}

	for _, tc := range []addFingerprintTestCase{
		{
			Label: "fp-1",
			EnrollmentProgresses: []cryptohome.EnrollmentProgress{
				{
					ScanResult: biod.ScanResult_SCAN_RESULT_SUCCESS,
					Percentage: 100,
				},
			},
			CreateCredStatus:  biod.CreateCredentialReply_SUCCESS,
			ExpectedErrorCode: uda.CryptohomeErrorCode_CRYPTOHOME_ERROR_NOT_SET,
		},
		{
			Label: "fp-2",
			EnrollmentProgresses: []cryptohome.EnrollmentProgress{
				{
					ScanResult: biod.ScanResult_SCAN_RESULT_SUCCESS,
					Percentage: 50,
				},
				{
					ScanResult: biod.ScanResult_SCAN_RESULT_SUCCESS,
					Percentage: 100,
				},
			},
			CreateCredStatus:  biod.CreateCredentialReply_SUCCESS,
			ExpectedErrorCode: uda.CryptohomeErrorCode_CRYPTOHOME_ERROR_NOT_SET,
		},
		{
			Label: "fp-3",
			EnrollmentProgresses: []cryptohome.EnrollmentProgress{
				{
					ScanResult: biod.ScanResult_SCAN_RESULT_SUCCESS,
					Percentage: 100,
				},
			},
			CreateCredStatus:  biod.CreateCredentialReply_CREATE_RECORD_FAILED,
			ExpectedErrorCode: uda.CryptohomeErrorCode_CRYPTOHOME_ADD_CREDENTIALS_FAILED,
		},
	} {
		s.Run(ctx, tc.Label, func(ctx context.Context, s *testing.State) {
			// Set up FakeAuthStackManager's behaviors.
			fasm.SetCreateCredStatus(&tc.CreateCredStatus)
			fasm.SetEnrollmentProgresses(tc.EnrollmentProgresses)

			if err := client.WithAuthSession(ctx, userName, false /*ephemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT, func(authSessionID string) error {
				if _, err := client.AuthenticateAuthFactor(ctx, authSessionID, passwordLabel, userPassword); err != nil {
					return errors.Wrap(err, "failed to authenticate with auth session")
				}
				err := client.PrepareThenAddFpAuthFactor(ctx, authSessionID, tc.Label)
				if tc.ExpectedErrorCode == uda.CryptohomeErrorCode_CRYPTOHOME_ERROR_NOT_SET {
					if err != nil {
						return errors.Wrap(err, "cryptohome PrepareThenAddFpAuthFactor returns an error")
					}
					listFactorsReply, err := client.ListAuthFactors(ctx, userName)
					if err != nil {
						return errors.Wrap(err, "cryptohome ListAuthFactors returns an error")
					}
					expectedConfiguredFactors = append(expectedConfiguredFactors,
						&uda.AuthFactorWithStatus{
							AuthFactor: &uda.AuthFactor{
								Type:  uda.AuthFactorType_AUTH_FACTOR_TYPE_FINGERPRINT,
								Label: tc.Label,
							},
						})
					if err := cryptohomecommon.ExpectAuthFactorsWithTypeAndLabel(
						listFactorsReply.ConfiguredAuthFactorsWithStatus, expectedConfiguredFactors); err != nil {
						return errors.Wrap(err, "cryptohome ListAuthFactors does not return expected auth factors")
					}
				} else {
					if err == nil || !strings.Contains(err.Error(), strconv.Itoa(int(tc.ExpectedErrorCode))) {
						return errors.Wrapf(err, "cryptohome cli does not return expected error code: %d, got: ", int(tc.ExpectedErrorCode))
					}
				}
				return nil
			}); err != nil {
				s.Fatal("Failed a fingerprint test case: ", err)
			}

			// Check enrollment signals and CreateCredStatus has been consumed.
			if fasm.GetCreateCredStatus() != nil || fasm.GetEnrollmentProgresses() != nil {
				s.Fatal("FakeAuthStackManager did not execute all predefined code paths")
			}

		})
	}
}
