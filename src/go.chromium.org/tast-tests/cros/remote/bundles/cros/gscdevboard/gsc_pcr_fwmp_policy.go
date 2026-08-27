// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"context"
	"strings"
	"time"

	"github.com/google/go-tpm/tpm2"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/utils"
	"go.chromium.org/tast/core/testing"
)

type testGSCPCRFWMPPolicy struct {
	extend     string
	digest     string
	testDefine bool
	testWrite  bool
	writeable  bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:    GSCPCRFWMPPolicy,
		Desc:    "Test FWMP can be updated in the right PCR0 states",
		Timeout: 10 * time.Minute,
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
			Name: "normal_allows_define",
			Val: testGSCPCRFWMPPolicy{
				extend:     ti50.ExtendNormalBoot,
				digest:     ti50.DigestNormalBoot,
				testDefine: true,
				writeable:  true,
			},
		}, {
			Name: "normal_allows_write",
			Val: testGSCPCRFWMPPolicy{
				extend:    ti50.ExtendNormalBoot,
				digest:    ti50.DigestNormalBoot,
				testWrite: true,
				writeable: true,
			},
		}, {
			Name: "rec_allows_define",
			Val: testGSCPCRFWMPPolicy{
				extend:     ti50.ExtendRecBoot,
				digest:     ti50.DigestRecBoot,
				testDefine: true,
				writeable:  true,
			},
		}, {
			Name: "rec_allows_write",
			Val: testGSCPCRFWMPPolicy{
				extend:    ti50.ExtendRecBoot,
				digest:    ti50.DigestRecBoot,
				testWrite: true,
				writeable: true,
			},
		}, {
			Name: "dev_allows_define",
			Val: testGSCPCRFWMPPolicy{
				extend:     ti50.ExtendDevBoot,
				digest:     ti50.DigestDevBoot,
				testDefine: true,
				writeable:  true,
			},
		}, {
			Name: "dev_allows_write",
			Val: testGSCPCRFWMPPolicy{
				extend:    ti50.ExtendDevBoot,
				digest:    ti50.DigestDevBoot,
				testWrite: true,
				writeable: true,
			},
		}, {
			Name: "rec_dev_blocks_define",
			Val: testGSCPCRFWMPPolicy{
				extend:     ti50.ExtendRecDevBoot,
				digest:     ti50.DigestRecDevBoot,
				testDefine: true,
				writeable:  false,
			},
		}, {
			Name: "rec_dev_blocks_write",
			Val: testGSCPCRFWMPPolicy{
				extend:    ti50.ExtendRecDevBoot,
				digest:    ti50.DigestRecDevBoot,
				testWrite: true,
				writeable: false,
			},
		}, {
			Name: "unknown_blocks_define",
			Val: testGSCPCRFWMPPolicy{
				extend:     ti50.ExtendUnknownBoot,
				digest:     ti50.DigestUnknownBoot,
				testDefine: true,
				writeable:  false,
			},
		}, {
			Name: "unknown_blocks_write",
			Val: testGSCPCRFWMPPolicy{
				extend:    ti50.ExtendUnknownBoot,
				digest:    ti50.DigestUnknownBoot,
				testWrite: true,
				writeable: false,
			},
		}, {
			Name: "zeroes",
			Val: testGSCPCRFWMPPolicy{
				extend:    "",
				digest:    ti50.ZeroPCR,
				testWrite: true,
				writeable: true,
			},
		}},
	})
}

func GSCPCRFWMPPolicy(ctx context.Context, s *testing.State) {
	testParams := s.Param().(testGSCPCRFWMPPolicy)
	b := utils.NewDevboardHelper(s, s.FixtValue().(*fixture.Value))
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}
	i := ti50.MustOpenCrOSImage(ctx, b, s, b.TestbedType)
	defer i.Close(ctx)

	attr := ti50.FwmpAttr()
	nvName, err := tpm2.NVName(&attr)
	if err != nil {
		s.Fatal("Failed to get NV name: ", err)
	}
	nvHandle := tpm2.NamedHandle{
		Handle: attr.NVIndex,
		Name:   *nvName,
	}
	// Define space in NV storage and clean up afterwards
	def := tpm2.NVDefineSpace{
		AuthHandle: tpm2.TPMRHPlatform,
		Auth:       ti50.EmptyPassword(),
		PublicInfo: tpm2.New2B(attr),
	}
	// Do not block CCD open in the FWMP since the test uses CCD open to
	// wipe the TPM.
	fwmpFile := utils.MakeFWMPFile(1)

	s.Log("Restarting GSC with appropriate straps")
	tpm := b.ResetAndTpmStartup(ctx, i, ti50.CCDModeOff, ti50.FfClamshell)

	// Define the space before extending PCR0, since the test is verifying write.
	if testParams.testWrite {
		_, err = def.Execute(tpm)
		th.MustSucceed(err, "failed to define FWMP before PCR0 is extended")
		defer tpm.NvUndefineSpace(attr)
	}

	if testParams.extend != "" {
		gpioMonitor := b.GpioMonitorStart(ctx, ti50.GpioTi50EcRstL)
		// Read from the gpio monitor to clear existing events.
		events := b.GpioMonitorRead(ctx, gpioMonitor)
		testing.ContextLog(ctx, "Cleared Events: ", events)

		tpm.PCRExtendCheckDigest(0, testParams.extend, testParams.digest)

		// Read new events from the GPIO monitor
		events = b.GpioMonitorFinish(ctx, gpioMonitor)
		testing.ContextLog(ctx, "EC_RST_L Events: ", events)

		// GSC should not pulse EC_RST_L with an erased board id
		if len(events.Sorted) != 0 {
			s.Errorf("GSC pulsed EC_RST_L extending %s", testParams.extend)
		}

	}

	ccdstate, err := i.Command(ctx, "ccdstate")
	th.MustSucceed(err, "failed to get ccdstate")
	s.Log("ccdstate:", ccdstate)
	if !strings.Contains(strings.ToLower(ccdstate), strings.ToLower(testParams.digest)) {
		s.Errorf("Digest not found in ccdstate output: %s", ccdstate)
	}

	// Try to define the space after PCR0 is extended to see if it works.
	if testParams.testDefine {
		_, err := def.Execute(tpm)
		if !testParams.writeable {
			if err == nil {
				s.Fatal("Defined the FWMP when it should be unwriteable")
			}
			s.Log("Blocked defining the FWMP after PCR0 extend")
			return
		}
		th.MustSucceed(err, "failed to define FWMP after PCR0 is extended")
		defer tpm.NvUndefineSpace(attr)
		s.Log("Defined the FWMP after PCR0 extend")
	}

	s.Log("Create a FWMP with non-zero flags")
	// Write the fwmp file data to new space.
	write := tpm2.NVWrite{
		AuthHandle: tpm2.TPMRHPlatform,
		NVIndex:    nvHandle,
		Data: tpm2.TPM2BMaxNVBuffer{
			Buffer: fwmpFile,
		},
		Offset: 0,
	}
	_, err = write.Execute(tpm)
	if testParams.writeable {
		if err != nil {
			s.Fatal("NVWrite failed: ", err)
		}
		s.Log("Wrote the FWMP")
	} else if err == nil {
		s.Fatalf("FWMP should not be writeable in %s", s.TestName())
	} else {
		s.Log("Blocked writing the FWMP after PCR0 extend")
	}
}
