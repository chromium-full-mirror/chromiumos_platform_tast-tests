// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/utils"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

var (
	// Boot param test output if verification passed.
	successRE = regexp.MustCompile(`SUCCESS!`)
)

func init() {
	testing.AddTest(&testing.Test{
		Func:    GSCBootParam,
		Desc:    "Verify boot_params_test succeeds",
		Timeout: 30 * time.Second,
		Contacts: []string{
			"cros-hwsec@google.com", // CrOS GSC Developers
			"aluo@google.com",       // Test Author
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr: []string{"group:gsc",
			"gsc_h1_shield", "gsc_dt_shield", "gsc_ot_shield",
			"gsc_image_ti50", "gsc_nightly"},
		Fixture: fixture.GSCOpenCCD,
	})
}

// GSCBootParam verifies boot_param_test passes
func GSCBootParam(ctx context.Context, s *testing.State) {
	b := utils.NewDevboardHelper(s, s.FixtValue().(*fixture.Value))
	i := ti50.MustOpenCrOSImage(ctx, b, s, b.TestbedType)
	defer i.Close(ctx)

	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}

	// Reserve 15 seconds after tpm startup for test
	tpmCtx, cancel := ctxutil.Shorten(ctx, 15*time.Second)
	defer cancel()

	tpm := b.ResetAndTpmStartupForBus(tpmCtx, i, ti50.TpmBusSpi, ti50.CCDModeOn, ti50.FfClamshell)
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")

	// Startup Clear
	hexdata, err := tpm.OpenTitanToolTpmCommand("execute-command", "--hexdata", "80010000000c000001440000")
	if err != nil {
		s.Fatal("Error startup: ", err)
	}

	// Read Boot param bin size
	hexdata, err = tpm.OpenTitanToolTpmCommand("execute-command", "--hexdata", "80010000000e00000169013fff0a")
	if err != nil {
		s.Fatal("Error read boot param size: ", err)
	}
	// 22 is the offset to bin data size
	binSize := binary.BigEndian.Uint32(hexdata[22 : 22+4])

	// Read Boot param data
	hexdata, err = tpm.OpenTitanToolTpmCommand("execute-command", "--hexdata", "8002000000230000014e4000000c013fff0a0000000940000009000000000004370000")
	if err != nil {
		s.Fatal("Error read boot param data: ", err)
	}
	// 16 is the offset to bin data
	bootParamBin := hexdata[16 : 16+binSize]

	output, dice, testMs, err := b.BootParamTest(ctx, bootParamBin, true, true)

	if err != nil {
		s.Fatal("boot_param_test error: ", err)
	}

	s.Logf("boot_param_test output: %s", output)
	s.Logf("boot_param_test dice: %s", hex.EncodeToString(dice))
	s.Logf("boot_param_test took %d ms", testMs)

	if successRE.FindSubmatch(output) == nil {
		s.Fatal("boot_param_test unsuccessful")
	}
}
