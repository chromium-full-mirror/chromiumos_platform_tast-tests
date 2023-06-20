// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hwsec

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/hwsec"
	hwseclocal "go.chromium.org/tast-tests/cros/local/hwsec"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         AttestationEnrollOnly,
		Desc:         "Verifies attestation-related functionality",
		Attr:         []string{"group:mainline", "informational", "group:attestation"},
		Contacts:     []string{"cros-hwsec@google.com", "chingkang@google.com"},
		BugComponent: "b:1188704",
		SoftwareDeps: []string{"tpm", "endorsement", "no_tpm_dynamic"},
	})
}

// AttestationEnrollOnly enrolls the device.
// Note that this item it to check if crbug/1070162 can be reproduced.
func AttestationEnrollOnly(ctx context.Context, s *testing.State) {
	r := hwseclocal.NewCmdRunner()
	helper, err := hwseclocal.NewFullHelper(ctx, r)
	if err != nil {
		s.Fatal("Helper creation error: ", err)
	}

	attestation := helper.AttestationClient()

	if err := helper.EnsureTPMIsReady(ctx, hwsec.DefaultTakingOwnershipTimeout); err != nil {
		s.Fatal("Failed to ensure tpm readiness: ", err)
	}
	s.Log("TPM is ensured to be ready")
	if err := helper.EnsureIsPreparedForEnrollment(ctx, hwsec.DefaultPreparationForEnrolmentTimeout); err != nil {
		s.Fatal("Failed to prepare for enrollment: ", err)
	}

	at := hwsec.NewAttestationTest(attestation, hwsec.DefaultPCA)

	if err := at.Enroll(ctx); err != nil {
		s.Fatal("Failed to enroll device: ", err)
	}
}
