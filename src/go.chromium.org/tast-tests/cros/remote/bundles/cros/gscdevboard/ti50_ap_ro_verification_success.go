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
	f := s.FixtValue().(*fixture.Value)
	b := utils.NewDevboardHelper(f, s)
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}
	i := ti50.MustOpenNewCrOSImage(ctx, b, s)

	// First, provision the GSC and AP flash so it can run AP RO verification correctly.

	s.Log("Best-effort provisioning AP SPI settings")
	// This causes any board ID in the GSCVD to be accepted.
	// This is a write-once field, so this is a best-effort provisioning.
	// If this fails, then the test will fail since the GSCVD won't be accepted.

	utils.NewResult(i.Command(ctx, "bid ZZCR 0")).MustSucceed(s, "Set BID")

	// Use 4 byte addressing since this is a 32 MiB chip.
	utils.NewResult(i.Command(ctx, "ap_ro_verify addrmode 4byte")).MustSucceed(s, "Set addrmode")
	// Found with the `src/third_party/ap_wpsr` tool with
	// `./ap_wpsr --name W25Q256JV_M --start 0 --length 0x00100000`
	utils.NewResult(i.Command(ctx, "ap_ro_verify wpsr d4 fc 0 41")).MustSucceed(s, "Set wpsr")

	th.MustSucceed(b.WithApFlashAccess(ctx, ti50.DoNotHoldInReset, func(flash ti50.ApFlash) {
		// Enable HW WP on the AP SPI chip so the status registers are as expected.
		flash.EnableApWriteProtect(ctx, 0, 0x00100000)

		// Write the fresh AP flash image
		s.Log("Flashing new AP image")
		flash.WriteApFlash(ctx, flashImageContents(s))
	}), "Access AP flash")

	// `WithApFlashAccess` has reset the GSC, and we've waited until boot.
	// AP RO verification has thus been run again.
	// Does `gsctool` say it's passed?
	stdout, err := b.GSCToolCommand(ctx, "", "-D", "-B")
	th.MustSucceed(err, "Get AP RO verification status")

	if !gsctoolApRoPassed.Match(stdout) {
		s.Errorf("AP RO verification did not pass, got %s", string(stdout))
	}
}
