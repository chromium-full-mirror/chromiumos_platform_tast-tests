// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cryptohome

import (
	"context"

	biod "go.chromium.org/chromiumos/system_api/biod_messages_proto"
	uda "go.chromium.org/chromiumos/system_api/user_data_auth_proto"

	cryptohomecommon "go.chromium.org/tast-tests/cros/common/cryptohome"
	hwseclocal "go.chromium.org/tast-tests/cros/local/hwsec"

	"go.chromium.org/tast-tests/cros/common/hwsec"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: AddRemoveFingerprintFactor,
		Desc: "Checks that cryptohome fingerprint enrollment process succeeds with fake auth stack",
		Contacts: []string{
			"cryptohome-core@google.com",
			"lziest@google.com",
		},
		BugComponent: "b:1088399", // ChromeOS > Security > Cryptohome
		Attr:         []string{"group:mainline", "informational", "group:cryptohome", "group:hw_agnostic"},
		SoftwareDeps: []string{"pinweaver"},
		Fixture:      "fakeBiometricsFixture",
	})
}

type addFingerprintTestCase struct {
	Label                string
	EnrollmentProgresses []cryptohome.EnrollmentProgress
	CreateCredStatus     biod.CreateCredentialReply_CreateCredentialStatus
	ExpectedErrorCode    uda.CryptohomeErrorCode
}

func AddRemoveFingerprintFactor(ctx context.Context, s *testing.State) {
	const (
		userName      = "foo@bar.baz"
		userPassword  = "secret"
		passwordLabel = "online-password"
		fpLabel1      = "fp-1"
		fpLabel2      = "fp-2"
	)
	f, ok := s.FixtValue().(*cryptohome.BiometricsFixture)
	if !ok {
		s.Fatal("Test fixture is not BiometricsFixture")
	}
	fasm := f.FakeAuthStackManager

	cmdRunner := hwseclocal.NewCmdRunner()
	client := hwsec.NewCryptohomeClient(cmdRunner)

	// Ensure a clean state.
	if err := client.UnmountAndRemoveVault(ctx, userName); err != nil {
		s.Fatal("Failed to remove old vault for preparation: ", err)
	}

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
			Label: fpLabel1,
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
			Label: fpLabel2,
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
		ok := s.Run(ctx, tc.Label, func(ctx context.Context, s *testing.State) {
			// Set up FakeAuthStackManager's behaviors.
			fasm.SetCreateCredStatus(&tc.CreateCredStatus)
			fasm.SetEnrollmentProgresses(tc.EnrollmentProgresses)
			fasm.SetRecordID(tc.Label)

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
					return cryptohome.CheckCliErrorCode(err, int(tc.ExpectedErrorCode))
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
		if !ok {
			// Don't continue to run upcoming subtests, as the test state has already gone wrong.
			return
		}

	}

	// Remove fingerprint auth factors fp-1 and fp-2.
	if err := client.WithAuthSession(ctx, userName, false /*ephemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT, func(authSessionID string) error {
		if _, err := client.AuthenticateAuthFactor(ctx, authSessionID, passwordLabel, userPassword); err != nil {
			return errors.Wrap(err, "failed to authenticate with auth session")
		}
		for _, label := range []string{fpLabel1, fpLabel2} {
			fasm.SetRecordID(label)
			if err := client.RemoveAuthFactor(ctx, authSessionID, label); err != nil {
				return errors.Wrapf(err, "failed to remove auth factor %s", label)
			}
		}
		expectedConfiguredFactors = []*uda.AuthFactorWithStatus{{
			AuthFactor: &uda.AuthFactor{
				Type:  uda.AuthFactorType_AUTH_FACTOR_TYPE_PASSWORD,
				Label: passwordLabel,
			},
		}}
		listFactorsReply, err := client.ListAuthFactors(ctx, userName)
		if err != nil {
			return errors.Wrap(err, "cryptohome ListAuthFactors returns an error")
		}
		if err := cryptohomecommon.ExpectAuthFactorsWithTypeAndLabel(
			listFactorsReply.ConfiguredAuthFactorsWithStatus, expectedConfiguredFactors); err != nil {
			return errors.Wrap(err, "cryptohome ListAuthFactors does not return expected auth factors")
		}
		return nil
	}); err != nil {
		s.Fatal("Failed the fingerprint removal: ", err)
	}
}
