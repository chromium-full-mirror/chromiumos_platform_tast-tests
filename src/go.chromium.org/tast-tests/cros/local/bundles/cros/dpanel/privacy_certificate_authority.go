// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dpanel

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/hwsec"
	hwseclocal "go.chromium.org/tast-tests/cros/local/hwsec"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PrivacyCertificateAuthority,
		Desc:         "Test PrivacyCertificateAuthority",
		Attr:         []string{},
		Contacts:     []string{"dpanel-eng@google.com"},
		BugComponent: "b:157583",
		SoftwareDeps: []string{"tpm", "endorsement", "no_tpm_dynamic"},
		Params: []testing.Param{
			{
				Name: "prod",
				Val:  hwsec.DefaultPCA,
			},
			{
				Name: "qa",
				Val:  hwsec.TestPCA,
			},
		},
		Timeout: 10 * time.Minute,
	})
}

// PrivacyCertificateAuthority enrolls the device and requests a certificate for binding software keys.
func PrivacyCertificateAuthority(ctx context.Context, s *testing.State) {
	pca := s.Param().(hwsec.PCAType)
	r := hwseclocal.NewCmdRunner()
	helper, err := hwseclocal.NewFullHelper(ctx, r)
	if err != nil {
		s.Fatal("Helper creation error: ", err)
	}

	attestation := helper.AttestationClient()

	// Ensure device is ready for the test.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if err := helper.EnsureTPMAndSystemStateAreReset(ctx); err != nil {
			return err
		}
		if err := helper.EnsureTPMIsReady(ctx, hwsec.DefaultTakingOwnershipTimeout); err != nil {
			return err
		}
		if err := helper.EnsureIsPreparedForEnrollment(ctx, hwsec.DefaultPreparationForEnrolmentTimeout); err != nil {
			return err
		}
		return nil
	}, &testing.PollOptions{Interval: 10 * time.Second}); err != nil {
		s.Fatal("Failed to enroll device: ", err)
	}

	// Execute the actual test, enroll the device and try to get a certificate.
	at := hwsec.NewAttestationTest(attestation, pca)

	if err := at.Enroll(ctx); err != nil {
		s.Fatal("Failed to enroll device: ", err)
	}

	// Clean up the device after the test.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 1*time.Minute)
	defer cancel()
	defer func(ctx context.Context) {
		if err := helper.EnsureTPMAndSystemStateAreReset(ctx); err != nil {
			s.Fatal("Failed to reset TPM and state: ", err)
		}
	}(cleanupCtx)

	if err := at.GetCertificateWithProfile(ctx, hwsec.SoftBindCertProfile, "", ""); err != nil {
		s.Fatal("Failed to get certificate: ", err)
	}
}
