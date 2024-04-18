// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fixture

import (
	"context"
	"time"

	"github.com/golang/protobuf/proto"

	apb "go.chromium.org/chromiumos/system_api/attestation_proto"
	"go.chromium.org/tast-tests/cros/common/hwsec"
	hwseclocal "go.chromium.org/tast-tests/cros/local/hwsec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:     "attestationFixture",
		Desc:     "The HWSec attestation test fixture",
		Contacts: []string{"cros-hwsec@google.com", "yich@google.com"},
		// ChromeOS > Platform > baseOS > Hardware Security > HwSec AP
		BugComponent:    "b:1188704",
		Impl:            &attestFixtImpl{},
		SetUpTimeout:    2 * time.Minute,
		ResetTimeout:    5 * time.Second,
		TearDownTimeout: 30 * time.Second,
	})
}

// AttestFixture contains the necessary things for attestation testing.
type AttestFixture struct {
	Username    string
	Attestation *hwsec.AttestationClient
	AttestTest  *hwsec.AttestationTest
	AttestDBus  *hwseclocal.AttestationDBus
}

type attestFixtImpl struct {
	cleanup func(context.Context) error
}

func (f *attestFixtImpl) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	r := hwseclocal.NewCmdRunner()
	helper, err := hwseclocal.NewFullHelper(ctx, r)
	if err != nil {
		s.Fatal("Helper creation error: ", err)
	}
	attestation := helper.AttestationClient()
	cryptohome := helper.CryptohomeClient()
	mountInfo := hwsec.NewCryptohomeMountInfo(r, cryptohome)
	if err := helper.EnsureTPMIsReady(ctx, hwsec.DefaultTakingOwnershipTimeout); err != nil {
		s.Fatal("Failed to ensure tpm readiness: ", err)
	}
	s.Log("TPM is ensured to be ready")
	if err := helper.EnsureIsPreparedForEnrollment(ctx, hwsec.DefaultPreparationForEnrolmentTimeout); err != nil {
		s.Fatal("Failed to prepare for enrollment: ", err)
	}

	at := hwsec.NewAttestationTest(attestation, hwsec.DefaultPCA)

	ac, err := hwseclocal.NewAttestationDBus(ctx)
	if err != nil {
		s.Fatal("Failed to create attestation client: ", err)
	}

	enrollReply, err := ac.Enroll(ctx, &apb.EnrollRequest{Forced: proto.Bool(true)})
	if err != nil {
		s.Fatal("Failed to call Enroll D-Bus API: ", err)
	}
	if *enrollReply.Status != apb.AttestationStatus_STATUS_SUCCESS {
		s.Fatal("Failed to enroll: ", enrollReply.Status.String())
	}

	const username = "test@crashwsec.bigr.name"

	s.Log("Resetting vault in case the cryptohome status is contaminated")
	// Okay to call it even if the vault doesn't exist.
	if err := mountInfo.CleanUpMount(ctx, username); err != nil {
		s.Fatal("Failed to cleanup: ", err)
	}

	if err := cryptohome.MountVault(ctx, "fake_label", hwsec.NewPassAuthConfig(username, "testpass"), true /* create */, hwsec.NewVaultConfig()); err != nil {
		s.Fatal("Failed to create user vault: ", err)
	}

	f.cleanup = func(ctx context.Context) error {
		if err := mountInfo.CleanUpMount(ctx, username); err != nil {
			return errors.Wrap(err, "failed to cleanup")
		}
		return nil
	}
	return &AttestFixture{
		Username:    username,
		Attestation: attestation,
		AttestTest:  at,
		AttestDBus:  ac,
	}
}

func (f *attestFixtImpl) TearDown(ctx context.Context, s *testing.FixtState) {
	if err := f.cleanup(ctx); err != nil {
		s.Fatal("Failed to cleanup: ", err)
	}
}

func (f *attestFixtImpl) PreTest(ctx context.Context, s *testing.FixtTestState) {
}

func (f *attestFixtImpl) PostTest(ctx context.Context, s *testing.FixtTestState) {
}

func (f *attestFixtImpl) Reset(ctx context.Context) error {
	return nil
}
