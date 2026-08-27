// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"bytes"
	"context"
	"encoding/hex"
	"time"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/gscdevboard/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"
	"go.chromium.org/tast/core/testing"
)

type testGSCPCRDoubleExtend struct {
	pcr          uint8
	firstExtend  string
	firstDigest  string
	secondExtend string
	secondDigest string
}

const (
	// A non-zero value to try to extend PCR0 twice
	nonStandardExtend = "1234000000000000000000000000000000000000000000000000000000000000"
	// PCR0 digest after it's been extended to the non-standard value.
	secondExtendDigest = "468c762da8d7de155c738b4c15dd9430a3cdf58f21b237770a25e673353578a1"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:    GSCPCRDoubleExtend,
		Desc:    "Verify GSC blocks PCR0 double extends",
		Timeout: 30 * time.Second,
		Contacts: []string{
			"cros-hwsec@google.com", // CrOS GSC Developers
			"mruthven@google.com",   // Test Author
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr: []string{
			"group:gsc",
			"gsc_dt_ab", "gsc_dt_shield", "gsc_h1_shield", "gsc_ot_shield", "gsc_ot_fpga_cw310",
			"gsc_image_ti50",
			"gsc_nightly"},
		Fixture: fixture.GSCOpenCCD,
		Params: []testing.Param{{
			// GSC should not modify PCR0 after it's been extended once.
			Name: "pcr0",
			Val: testGSCPCRDoubleExtend{
				pcr:          0,
				firstExtend:  ti50.ExtendNormalBoot,
				firstDigest:  ti50.DigestNormalBoot,
				secondExtend: nonStandardExtend,
				secondDigest: ti50.DigestNormalBoot,
			},
			ExtraAttr: []string{"gsc_smoke"},
		}, {
			// GSC should allow double extending PCR1
			Name: "pcr1",
			Val: testGSCPCRDoubleExtend{
				pcr:          1,
				firstExtend:  ti50.ExtendNormalBoot,
				firstDigest:  ti50.DigestNormalBoot,
				secondExtend: nonStandardExtend,
				secondDigest: secondExtendDigest,
			},
		}, {
			// GSC should allow double extending PCR7
			Name: "pcr7",
			Val: testGSCPCRDoubleExtend{
				pcr:          7,
				firstExtend:  ti50.ExtendNormalBoot,
				firstDigest:  ti50.DigestNormalBoot,
				secondExtend: nonStandardExtend,
				secondDigest: secondExtendDigest,
			},
		}},
	})
}

func GSCPCRDoubleExtend(ctx context.Context, s *testing.State) {
	testParams := s.Param().(testGSCPCRDoubleExtend)
	b := utils.NewDevboardHelper(s, s.FixtValue().(*fixture.Value))
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}
	i := ti50.MustOpenCrOSImage(ctx, b, s, b.TestbedType)
	defer i.Close(ctx)

	s.Log("Restarting GSC with appropriate straps")
	tpm := b.ResetAndTpmStartup(ctx, i, ti50.CCDModeOff, ti50.FfClamshell)

	ccdstate, err := i.Command(ctx, "ccdstate")
	th.MustSucceed(err, "failed to get ccdstate output")
	s.Logf("ccdstate: %s", ccdstate)

	zeroPCR, err := hex.DecodeString(ti50.ZeroPCR)
	th.MustSucceed(err, "failed to decode zeroPCR")

	read, err := tpm.PCRRead(testParams.pcr)
	th.MustSucceed(err, "failed to read pcr")
	if !bytes.Equal(read.PCRValues.Digests[0].Buffer, zeroPCR) {
		s.Fatalf("PCR%d is not after reset", testParams.pcr)
	}

	tpm.PCRExtendCheckDigest(testParams.pcr, testParams.firstExtend, testParams.firstDigest)
	th.MustSucceed(err, "failed to extend first value")

	err = tpm.PCRExtendCheckDigest(testParams.pcr, testParams.secondExtend, testParams.secondDigest)
	th.MustSucceed(err, "unexpected result during second extend")

	s.Log("PCR double extend policy worked")
}
