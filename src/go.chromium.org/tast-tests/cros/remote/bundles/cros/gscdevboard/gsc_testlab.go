// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"context"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/gscdevboard/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"

	"go.chromium.org/tast/core/testing"
)

const (
	// It takes 5 power button presses to enable or disable testlab mode.
	testlabPresses = 5
)

func init() {
	testing.AddTest(&testing.Test{
		Func:    GSCTestlab,
		Desc:    "Verify testlab mode can be enabled and disabled with physical presence",
		Timeout: 5 * time.Minute,
		Contacts: []string{
			"cros-hwsec@google.com", // CrOS GSC Developers
			"mruthven@chromium.org", // Test Author
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr: []string{"group:gsc",
			"gsc_dt_ab", "gsc_dt_shield", "gsc_h1_shield", "gsc_ot_shield",
			"gsc_image_ti50",
			"gsc_nightly"},
		Fixture: fixture.GSCOpenCCD,
	})
}

func runPowerButtonPresses(ctx context.Context, s *testing.State, b utils.DevboardHelper, i *ti50.CrOSImage, numPress int) {
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}
	for press := 1; press <= numPress; press++ {
		th.MustSucceed(i.WaitForPowerButtonPrompt(ctx, time.Second*2), "Power button prompt %d did not happen: %s", press)
		// Ti50 requires 100ms delay between short presses.
		testing.Sleep(ctx, 100*time.Millisecond) // GoBigSleepLint: Simulating button press
		b.GpioSet(ctx, ti50.GpioTi50PowerBtnL, false)
		b.GpioSet(ctx, ti50.GpioTi50PowerBtnL, true)
	}
}

var accessDeniedRE = regexp.MustCompile(`(?i)access denied`)

func verifyTestlabBlocked(ctx context.Context, s *testing.State, i *ti50.CrOSImage, cmd string) {
	// Verify Testlab mode can't be changed while ccd is locked
	output, err := i.Command(ctx, cmd)
	if err != nil {
		s.Fatalf("Failed to execute cmd %s: %s", cmd, err)
	}
	if !accessDeniedRE.MatchString(output) {
		s.Errorf("Did not get access denied running %s", cmd)
	}
}

func GSCTestlab(ctx context.Context, s *testing.State) {
	b := utils.NewDevboardHelper(s)
	i := ti50.MustOpenCrOSImage(ctx, b, s)
	defer i.Close(ctx)

	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}

	s.Log("(Re)starting GSC")
	b.ResetAndTpmStartup(ctx, i, ti50.CcdSuzyQ, ti50.FfClamshell)

	th.MustSucceed(i.CCDLock(ctx), "Lock CCD")
	// Verify testlab mode can't be changed when ccd is locked
	verifyTestlabBlocked(ctx, s, i, "ccd testlab disable")
	verifyTestlabBlocked(ctx, s, i, "ccd testlab enable")

	th.MustSucceed(i.CCDOpen(ctx), "Open CCD")
	// Verify disabling testlab mode first. The GSCOpenCCD fixture enables it.
	// Use WriteSerial here so we can WaitUntilMatch(pushButton) below. i.Command doesn't work
	// because the pushButton message comes before the console prompt.
	th.MustSucceed(i.WriteSerial(ctx, []byte("ccd testlab disable\r")), "Testlab disable")
	runPowerButtonPresses(ctx, s, b, i, testlabPresses)
	th.MustSucceed(i.WaitForTestlabDisable(ctx, time.Second*2), "Testlab disabled")

	th.MustSucceed(i.StartTestlabEnable(ctx), "Testlab enable")
	runPowerButtonPresses(ctx, s, b, i, testlabPresses)
	th.MustSucceed(i.WaitForTestlabEnable(ctx, time.Second*2), "Testlab enabled")
}
