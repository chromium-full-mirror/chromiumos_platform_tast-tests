// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cryptohome

import (
	"context"

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
		Func: AuthenticateFingerprintFactor,
		Desc: "Checks that cryptohome fingerprint enrollment process succeeds with fake auth stack",
		Contacts: []string{
			"cryptohome-core@google.com",
			"lziest@google.com",
		},
		BugComponent: "b:1088399", // ChromeOS > Security > Cryptohome
		Attr:         []string{"group:mainline", "informational", "group:cryptohome"},
		SoftwareDeps: []string{"pinweaver"},
		Fixture:      "fakeBiometricsFixture",
	})
}

type authFingerprintTestCase struct {
	Name              string
	AuthScanResults   []cryptohome.AuthScanResult
	RecordID          string
	ExpectedErrorCode uda.CryptohomeErrorCode
}

func AuthenticateFingerprintFactor(ctx context.Context, s *testing.State) {
	const (
		userName      = "foo@bar.baz"
		userPassword  = "secret"
		passwordLabel = "online-password"
		fpLabel1      = "fp-1"
		fpLabel2      = "fp-2"
	)
	fpLabels := []string{fpLabel1, fpLabel2}
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

		// Add two fingerprint auth factors.
		createCredSuccess := biod.CreateCredentialReply_SUCCESS
		for _, label := range fpLabels {
			fasm.SetCreateCredStatus(&createCredSuccess)
			fasm.SetEnrollmentProgresses([]cryptohome.EnrollmentProgress{
				{
					ScanResult: biod.ScanResult_SCAN_RESULT_SUCCESS,
					Percentage: 100,
				},
			})
			fasm.SetRecordID(label)
			if err := client.PrepareThenAddFpAuthFactor(ctx, authSessionID, label); err != nil {
				return errors.Wrap(err, "failed to add a fp authfactor")
			}
		}
		// Check two fingerprint and one password auth factors are included in available auth factors.
		expectedConfiguredFactors := []*uda.AuthFactorWithStatus{
			{
				AuthFactor: &uda.AuthFactor{
					Type:  uda.AuthFactorType_AUTH_FACTOR_TYPE_PASSWORD,
					Label: passwordLabel,
				},
			},
			{
				AuthFactor: &uda.AuthFactor{
					Type:  uda.AuthFactorType_AUTH_FACTOR_TYPE_FINGERPRINT,
					Label: fpLabel1,
				},
			},
			{
				AuthFactor: &uda.AuthFactor{
					Type:  uda.AuthFactorType_AUTH_FACTOR_TYPE_FINGERPRINT,
					Label: fpLabel2,
				},
			},
		}
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
		s.Fatal("Failed to setup a persistent user: ", err)
	}

	for _, tc := range []authFingerprintTestCase{
		{
			Name:     "fp1",
			RecordID: fpLabel1,
			AuthScanResults: []cryptohome.AuthScanResult{
				{
					AuthCredStatus: biod.AuthenticateCredentialReply_SUCCESS,
					ScanResult:     biod.ScanResult_SCAN_RESULT_SUCCESS,
				},
			},
			ExpectedErrorCode: uda.CryptohomeErrorCode_CRYPTOHOME_ERROR_NOT_SET,
		},
		{
			Name:     "fp2",
			RecordID: fpLabel2,
			AuthScanResults: []cryptohome.AuthScanResult{
				{
					AuthCredStatus: biod.AuthenticateCredentialReply_SUCCESS,
					ScanResult:     biod.ScanResult_SCAN_RESULT_SUCCESS,
				},
			},
			ExpectedErrorCode: uda.CryptohomeErrorCode_CRYPTOHOME_ERROR_NOT_SET,
		},
		{
			Name:     "bad record id",
			RecordID: "bad id",
			AuthScanResults: []cryptohome.AuthScanResult{
				{
					AuthCredStatus: biod.AuthenticateCredentialReply_SUCCESS,
					ScanResult:     biod.ScanResult_SCAN_RESULT_SUCCESS,
				},
			},
			ExpectedErrorCode: uda.CryptohomeErrorCode_CRYPTOHOME_ERROR_KEY_NOT_FOUND,
		},
		{
			Name:     "fp-1 multi-trials",
			RecordID: fpLabel1,
			AuthScanResults: []cryptohome.AuthScanResult{
				{
					AuthCredStatus: biod.AuthenticateCredentialReply_SUCCESS,
					ScanResult:     biod.ScanResult_SCAN_RESULT_PARTIAL,
				},
				{
					AuthCredStatus: biod.AuthenticateCredentialReply_SUCCESS,
					ScanResult:     biod.ScanResult_SCAN_RESULT_TOO_FAST,
				},
				{
					AuthCredStatus: biod.AuthenticateCredentialReply_SUCCESS,
					ScanResult:     biod.ScanResult_SCAN_RESULT_TOO_SLOW,
				},
				{
					AuthCredStatus: biod.AuthenticateCredentialReply_SUCCESS,
					ScanResult:     biod.ScanResult_SCAN_RESULT_SUCCESS,
				},
			},
			ExpectedErrorCode: uda.CryptohomeErrorCode_CRYPTOHOME_ERROR_NOT_SET,
		},
		{
			Name:     "locks out",
			RecordID: "",
			AuthScanResults: []cryptohome.AuthScanResult{
				{
					AuthCredStatus: biod.AuthenticateCredentialReply_SUCCESS,
					ScanResult:     biod.ScanResult_SCAN_RESULT_PARTIAL,
				},
				{
					AuthCredStatus: biod.AuthenticateCredentialReply_SUCCESS,
					ScanResult:     biod.ScanResult_SCAN_RESULT_TOO_FAST,
				},
				{
					AuthCredStatus: biod.AuthenticateCredentialReply_SUCCESS,
					ScanResult:     biod.ScanResult_SCAN_RESULT_TOO_SLOW,
				},
				{
					AuthCredStatus: biod.AuthenticateCredentialReply_SUCCESS,
					ScanResult:     biod.ScanResult_SCAN_RESULT_PARTIAL,
				},
				{
					AuthCredStatus: biod.AuthenticateCredentialReply_SUCCESS,
					ScanResult:     biod.ScanResult_SCAN_RESULT_TOO_FAST,
				},
			},
			ExpectedErrorCode: uda.CryptohomeErrorCode_CRYPTOHOME_ERROR_FINGERPRINT_DENIED,
		},
	} {
		ok := s.Run(ctx, tc.Name, func(ctx context.Context, s *testing.State) {
			// Set up FakeAuthStackManager's behaviors.
			fasm.SetAuthScanResults(tc.AuthScanResults)
			fasm.SetRecordID(tc.RecordID)

			if err := client.WithAuthSession(ctx, userName, false /*ephemeral*/, uda.AuthIntent_AUTH_INTENT_VERIFY_ONLY, func(authSessionID string) error {
				err := client.PrepareThenAuthFpAuthFactor(ctx, authSessionID, fpLabels)
				return cryptohome.CheckCliErrorCode(err, int(tc.ExpectedErrorCode))
			}); err != nil {
				s.Fatal("Failed a fingerprint test case: ", err)
			}

			// Check authScanResults, and recordID have been consumed.
			if len(fasm.GetAuthScanResults()) != 0 || fasm.GetRecordID() != "" {
				s.Fatal("FakeAuthStackManager did not execute all predefined code paths")
			}

		})
		// Skip upcoming subtests and exit, as subtests are sharing states.
		if !ok {
			return
		}
	}
	// New fingerprint authentication is denied.
	// Set the auth results to reflect a fingerprint touch.
	fasm.SetAuthScanResults([]cryptohome.AuthScanResult{
		{
			AuthCredStatus: biod.AuthenticateCredentialReply_SUCCESS,
			ScanResult:     biod.ScanResult_SCAN_RESULT_SUCCESS,
		},
	})

	if err := client.WithAuthSession(ctx, userName, false /*ephemeral*/, uda.AuthIntent_AUTH_INTENT_VERIFY_ONLY, func(authSessionID string) error {
		err := client.PrepareThenAuthFpAuthFactor(ctx, authSessionID, fpLabels)
		return cryptohome.CheckCliErrorCode(err, int(uda.CryptohomeErrorCode_CRYPTOHOME_ERROR_TPM_DEFEND_LOCK))
	}); err != nil {
		s.Fatal("Failed the fingerprint re-authenticate after locked-out: ", err)
	}

	// Reset the fingerprint state with a password authentication.
	if err := client.WithAuthSession(ctx, userName, false /*ephemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT, func(authSessionID string) error {
		if _, err := client.AuthenticateAuthFactor(ctx, authSessionID, passwordLabel, userPassword); err != nil {
			return errors.Wrap(err, "failed to authenticate with auth session")
		}
		return nil
	}); err != nil {
		s.Fatal("Failed to authenticate with password: ", err)
	}

	// Fingerprint auth after reset succeeds.
	fasm.SetAuthScanResults([]cryptohome.AuthScanResult{
		{
			AuthCredStatus: biod.AuthenticateCredentialReply_SUCCESS,
			ScanResult:     biod.ScanResult_SCAN_RESULT_SUCCESS,
		},
	})
	fasm.SetRecordID(fpLabel1)

	if err := client.WithAuthSession(ctx, userName, false /*ephemeral*/, uda.AuthIntent_AUTH_INTENT_VERIFY_ONLY, func(authSessionID string) error {
		err := client.PrepareThenAuthFpAuthFactor(ctx, authSessionID, fpLabels)
		return err
	}); err != nil {
		s.Fatal("Failed the fingerprint re-authenticate after locked-out: ", err)
	}
	// Check authScanResults, and recordID have been consumed.
	if len(fasm.GetAuthScanResults()) != 0 || fasm.GetRecordID() != "" {
		s.Fatal("FakeAuthStackManager did not execute all predefined code paths")
	}
}
