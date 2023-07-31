// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hwsec

import (
	"context"
	"strings"
	"time"

	"github.com/google/go-attestation/attest"

	"go.chromium.org/tast-tests/cros/common/hwsec"
	"go.chromium.org/tast-tests/cros/common/testexec"
	hwseclocal "go.chromium.org/tast-tests/cros/local/hwsec"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: VTPMEKCert,
		Desc: "Check the vTPM EK cert is readable",
		Contacts: []string{
			"cros-hwsec@google.com",
			"yich@google.com", // Test author.
		},
		// ChromeOS > Platform > System > Hardware Security > HwSec AP
		BugComponent: "b:1188704",
		SoftwareDeps: []string{"tpm2", "endorsement", "no_tpm_dynamic"},
		Attr:         []string{"group:hwsec", "group:vtpm", "hwsec_nightly"},
	})
}

func VTPMEKCert(ctx context.Context, s *testing.State) {
	r := hwseclocal.NewCmdRunner()
	helper, err := hwseclocal.NewFullHelper(ctx, r)
	if err != nil {
		s.Fatal("Helper creation error: ", err)
	}

	if err := helper.EnsureTPMIsReady(ctx, hwsec.DefaultTakingOwnershipTimeout); err != nil {
		s.Fatal("Failed to ensure tpm readiness: ", err)
	}

	ali := hwseclocal.NewAttestationLocalInfra(helper.DaemonController())
	if err := ali.Enable(ctx); err != nil {
		s.Fatal("Failed to enable local test infra feature: ", err)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second) // Reserve 10 seconds for clean up tasks.
	defer cancel()
	defer func(ctx context.Context) {
		if err := ali.Disable(ctx); err != nil {
			s.Error("Failed to disable local test infra feature: ", err)
		}
	}(cleanupCtx)

	s.Log("TPM is ensured to be ready")
	if err := helper.EnsureIsPreparedForEnrollment(ctx, hwsec.DefaultPreparationForEnrolmentTimeout); err != nil {
		s.Fatal("Failed to prepare for enrollment: ", err)
	}

	nvdata, err := testexec.CommandContext(ctx, "trunks_client", "--index_data", "--index=0x1C0000a", "--vtpm").Output()
	if err != nil {
		s.Fatal("Failed to read nvdata: ", err)
	}

	res := strings.TrimSpace(strings.TrimPrefix(string(nvdata), "NV Index data: "))

	ekCert, err := hwsec.HexDecode([]byte(res))

	if err != nil {
		s.Fatal("Failed to hex-decode cert: ", err)
	}

	if _, err := attest.ParseEKCertificate(ekCert); err != nil {
		s.Fatal("Failed to parse EK cert: ", err)
	}
}
