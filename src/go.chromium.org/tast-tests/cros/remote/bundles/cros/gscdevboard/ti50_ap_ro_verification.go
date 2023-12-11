// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"context"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/gscdevboard/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"
	"go.chromium.org/tast/core/testing"
)

type spiImage string

const (
	validSPIImage  spiImage = "valid-32M_20231101.bin"
	badGBBSPIImage spiImage = "bad-gbb-32MB_20231101.bin"

	verificationResultSuccess = 0xfffff000
	verificationResultBadGBB  = 0x11000000
)

var (
	gsctoolApRoPassed  = regexp.MustCompile(`apro result\s*\(20\)\s*:\s*pass`)
	verificationResult = regexp.MustCompile(`AP RO verification result: [^(]+ \(0x(\w+)\)`)
)

func init() {
	testing.AddTest(&testing.Test{
		Func:    Ti50APROVerification,
		Desc:    "Verify AP RO verification feature with valid and invalid settings",
		Timeout: 15 * time.Minute,
		Contacts: []string{
			"chromeos-faft@google.com",
			"ti50-core@google.com",
			"kupiakos@google.com",
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr:         []string{"group:gsc", "gsc_dt_shield"},
		Fixture:      fixture.GSCInitialFactory,
		Data:         []string{string(validSPIImage), string(badGBBSPIImage)},
	})
}

func getSPIImageContents(s *testing.State, image spiImage) []byte {
	contents, err := os.ReadFile(s.DataPath(string(image)))
	if err != nil {
		s.Fatalf("Could not read image %q: %s", string(image), err)
	}
	return contents
}

// Ti50APROVerification tests AP RO verification succeeds against a production image.
func Ti50APROVerification(ctx context.Context, s *testing.State) {
	b := utils.NewDevboardHelper(s)
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}
	i := ti50.MustOpenCrOSImage(ctx, b, s)
	defer i.Close(ctx)

	// First, provision the GSC and AP flash so it can run AP RO verification correctly.
	s.Log("(Re)starting GSC")
	b.Reset(ctx)
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")

	s.Log("Provisioning AP SPI settings")
	bidSet, err := i.Command(ctx, "bid ZZCR 0x7fffffff")
	th.MustSucceed(err, "Set BID")
	if strings.Contains(bidSet, "failed") {
		s.Fatal("Could not set BID: ", bidSet)
	}

	// Use 4 byte addressing since this is a 32 MiB chip.
	modeSet, err := i.Command(ctx, "ap_ro_verify addrmode 4byte")
	th.MustSucceed(err, "Set addrmode")
	if strings.Contains(modeSet, "failed") {
		s.Fatal("Could not set address mode: ", modeSet)
	}

	// Found with the `src/third_party/ap_wpsr` tool with
	// `./ap_wpsr --name W25Q256JV_M --start 0 --length 0x00100000`
	wpsrSet, err := i.Command(ctx, "ap_ro_verify wpsr d4 fc 0 41")
	th.MustSucceed(err, "Set wpsr")
	if strings.Contains(wpsrSet, "failed") {
		s.Fatal("Could not set wpsr: ", wpsrSet)
	}

	s.Log("Reset ccd to clear initial factory mode")
	th.MustSucceed(i.CCDOpen(ctx), "CCD Open")
	th.MustSucceed(i.CCDReset(ctx), "CCD Reset")
	th.MustSucceed(i.CCDResetFactory(ctx), "CCD Factory Reset")

	// Validate success case first to ensure that latch flips
	verifyValidImage(ctx, s, b, i)

	// Now all failed verification should hold system in reset when AllowUnverifiedRO is false
	verifyBadImage(ctx, s, b, i, badGBBSPIImage, verificationResultBadGBB)
}

func verifyValidImage(ctx context.Context, s *testing.State, b utils.DevboardHelper, i *ti50.CrOSImage) {
	flashSPIImage(ctx, s, b, i, validSPIImage)
	verifyVerificationResultOnReboot(ctx, s, b, i, verificationResultSuccess)

	ecResetL := b.GpioGet(ctx, ti50.GpioTi50EcRstL)
	if ecResetL != true {
		s.Error("EC not released for successful AP RO verifcation")
	}
}

func verifyBadImage(ctx context.Context, s *testing.State, b utils.DevboardHelper, i *ti50.CrOSImage, spi spiImage, wantVerificationResult uint32) {
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}

	// Ensure that AllowUnverifiedRo is set to never so EC is held in reset
	s.Log("Set AllowUnverifiedRo to never")
	th.MustSucceed(i.CCDOpen(ctx), "CCD Open failed")
	th.MustSucceed(i.SetCCDCapability(ctx, ti50.AllowUnverifiedRO, ti50.CapDefault), "Set AllowUnverifiedRo to never")

	flashSPIImage(ctx, s, b, i, spi)
	verifyVerificationResultOnReboot(ctx, s, b, i, wantVerificationResult)

	ecResetL := b.GpioGet(ctx, ti50.GpioTi50EcRstL)
	if ecResetL != false {
		s.Error("EC released when AP RO verification failed and AllowUnverifiedRo set to never")
	}

	s.Log("Set AllowUnverifiedRo to always")
	th.MustSucceed(i.CCDOpen(ctx), "CCD Open failed")
	th.MustSucceed(i.SetCCDCapability(ctx, ti50.AllowUnverifiedRO, ti50.CapAlways), "Set AllowUnverifiedRo to always")

	verifyVerificationResultOnReboot(ctx, s, b, i, wantVerificationResult)

	// GoBigSleepLint: It is a true failure if EC isn't released within 0.5 sec
	testing.Sleep(ctx, 500*time.Millisecond)

	ecResetL = b.GpioGet(ctx, ti50.GpioTi50EcRstL)
	if ecResetL != true {
		s.Error("EC held in reset when AP RO verification failed and AllowUnverifiedRo set to always")
	}
}

func flashSPIImage(ctx context.Context, s *testing.State, b utils.DevboardHelper, i *ti50.CrOSImage, spi spiImage) {
	b.WithApFlashAccess(ctx, i, ti50.HoldInReset, func(flash ti50.ApFlash) {
		// Enable SW WP on the AP SPI chip so the status registers are as expected.
		// This range represents the RO section of the AP flash.
		// We are able to modify SW WP because HW WP is disabled due to some previous "ccd reset factory".
		flash.EnableApWriteProtect(ctx, 0, 0x00100000)

		// Write the fresh AP flash image. We only care about the RO section for verification but
		// this will write the whole 32M image. The SW WP is ignored because HW WP is disabled.
		s.Log("Flashing new AP image: ", string(spi))
		flash.WriteApFlash(ctx, getSPIImageContents(s, spi))
	})
}

func verifyVerificationResultOnReboot(ctx context.Context, s *testing.State, b utils.DevboardHelper, i *ti50.CrOSImage, wantVerificationResult uint32) {
	b.Reset(ctx)
	m, err := i.WaitUntilMatch(ctx, verificationResult, 5*time.Second)
	if err != nil {
		s.Fatal("Expected to see Ti50 verification result: ", err)
	}

	verificationResult := mustParseVerificationResult(s, m)
	if verificationResult != wantVerificationResult {
		s.Errorf("Unexpected verification result: got 0x%x, wanted 0x%x", verificationResult, wantVerificationResult)
	}
}

func mustParseVerificationResult(s *testing.State, m [][]byte) uint32 {
	result, err := strconv.ParseUint(string(m[1]), 16, 32)
	if err != nil {
		s.Fatalf("Could not parse verification result of %v: %s", m[1], err)
	}
	return uint32(result)
}
