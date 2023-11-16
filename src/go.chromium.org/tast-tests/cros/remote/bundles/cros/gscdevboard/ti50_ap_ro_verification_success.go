// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"context"
	"os"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/gscdevboard/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"
	"go.chromium.org/tast/core/testing"
)

var (
	gsctoolApRoPassed = regexp.MustCompile(`apro result\s*\(20\)\s*:\s*pass`)
)

func init() {
	testing.AddTest(&testing.Test{
		Func:    Ti50ApRoVerificationSuccess,
		Desc:    "Verify Ti50 verifies a valid AP RO with valid settings",
		Timeout: 5 * time.Minute,
		Contacts: []string{
			"chromeos-faft@google.com",
			"ti50-core@google.com",
			"kupiakos@google.com",
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr:         []string{"group:gsc", "gsc_dt_shield"},
		Fixture:      fixture.Ti50CcdOpen,
		Data:         []string{"valid-32M_20231101.bin"},
	})
}

func flashImageContents(s *testing.State) []byte {
	contents, err := os.ReadFile(s.DataPath("valid-32M_20231101.bin"))
	if err != nil {
		s.Fatalf("Could not read test image %q", err)
	}
	return contents
}

// Ti50ApRoVerificationSuccess tests AP RO verification succeeds against a production image.
func Ti50ApRoVerificationSuccess(ctx context.Context, s *testing.State) {
	b := utils.NewDevboardHelper(s)
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}
	i := ti50.MustOpenCrOSImage(ctx, b, s)
	defer i.Close(ctx)

	// First, provision the GSC and AP flash so it can run AP RO verification correctly.

	s.Log("Best-effort provisioning AP SPI settings")
	// This causes any board ID in the GSCVD to be accepted.
	// This is a write-once field, so this is a best-effort provisioning.
	// If this fails, then the test will fail since the GSCVD won't be accepted.

	_, err := i.Command(ctx, "bid ZZCR 0x7fffffff")
	th.MustSucceed(err, "Set BID")

	// Use 4 byte addressing since this is a 32 MiB chip.
	_, err = i.Command(ctx, "ap_ro_verify addrmode 4byte")
	th.MustSucceed(err, "Set addrmode")
	// Found with the `src/third_party/ap_wpsr` tool with
	// `./ap_wpsr --name W25Q256JV_M --start 0 --length 0x00100000`
	_, err = i.Command(ctx, "ap_ro_verify wpsr d4 fc 0 41")
	th.MustSucceed(err, "Set wpsr")

	b.WithApFlashAccess(ctx, i, ti50.DoNotHoldInReset, func(flash ti50.ApFlash) {
		// Enable SW WP on the AP SPI chip so the status registers are as expected.
		// This range represents the RO section of the AP flash.
		// We are able to modify SW WP because HW WP is disabled due to some previous "ccd reset factory".
		flash.EnableApWriteProtect(ctx, 0, 0x00100000)

		// Write the fresh AP flash image. We only care about the RO section for verification but
		// this will write the whole 32M image. The SW WP is ignored because HW WP is disabled.
		s.Log("Flashing new AP image")
		flash.WriteApFlash(ctx, flashImageContents(s))
	})

	// `WithApFlashAccess` has reset the GSC, and we've waited until boot.
	// AP RO verification has thus been run again.
	// Does `gsctool` say it's passed?
	stdout, err := b.GSCToolCommand(ctx, "", "-D", "-B")
	th.MustSucceed(err, "Get AP RO verification status")

	// HW WP is still disabled, but that doesn't prevent a verification pass.
	if !gsctoolApRoPassed.Match(stdout) {
		s.Errorf("AP RO verification did not pass, got %s", string(stdout))
	}
}
