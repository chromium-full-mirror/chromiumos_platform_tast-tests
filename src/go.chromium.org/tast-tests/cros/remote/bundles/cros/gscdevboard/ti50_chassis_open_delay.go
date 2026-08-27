// Copyright 2024 The ChromiumOS Authors
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
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

var (
	// BID type 'DUYG' is one that adds 5 minute delay to chassis_open.
	testChassisOpenBIDType = ti50.BIDField(0x44555947)
	// These flags should work with all test image flags (0x10, 0x10000, and 0x20000)
	testChassisOpenBIDFlags = ti50.BIDField(0x37f7f)
)

func init() {
	testing.AddTest(&testing.Test{
		Func:    Ti50ChassisOpenDelay,
		Desc:    "Verify chassis open delayed 5 minutes for some board ID",
		Timeout: 20 * time.Minute,
		Contacts: []string{
			"cros-hwsec@google.com",
			"ecgh@chromium.org",
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr: []string{"group:gsc",
			"gsc_dt_shield", "gsc_ot_shield", "gsc_ot_fpga_cw310",
			"gsc_image_ti50",
			"gsc_nightly"},
		Fixture: fixture.GSCInitialFactory,
		Params: []testing.Param{{
			Name: "no_delay",
			Val:  false,
		}, {
			Name: "delay",
			Val:  true,
		}},
	})
}

func matchCommandOutput(ctx context.Context, i *ti50.CrOSImage, cmd string, resp *regexp.Regexp) error {
	output, err := i.Command(ctx, cmd)
	if err != nil {
		return errors.Wrap(err, "failed to execute `"+cmd+"`")
	}
	if regexp.MustCompile(`(?i)access denied`).MatchString(output) {
		return errors.Wrap(err, "got access denied when trying to run `"+cmd+"`")
	}
	if resp.FindStringSubmatch(output) == nil {
		return errors.Errorf("unexpected output to "+cmd+": wanted %v got %s", resp, output)
	}
	return nil
}

// verifyOpenTimeout checks open fails due to no button push.
func verifyOpenTimeout(ctx context.Context, i *ti50.CrOSImage, th utils.FirmwareTestingHelper) {
	th.MustSucceed(matchCommandOutput(ctx, i, "ccd open", regexp.MustCompile(`Press the physical button now!`)), "ccd open")
	_, err := i.WaitUntilMatch(ctx, regexp.MustCompile(`Timeout waiting for power button!`), time.Second*20)
	th.MustSucceed(err, "failed to timeout")
}

// Ti50ChassisOpenDelay verifies chassis open delayed 5 minutes for some board ID.
func Ti50ChassisOpenDelay(ctx context.Context, s *testing.State) {
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}
	b := utils.NewDevboardHelper(s, s.FixtValue().(*fixture.Value))
	i := ti50.MustOpenCrOSImage(ctx, b, s, b.TestbedType)
	defer i.Close(ctx)
	addDelay := s.Param().(bool)

	b.GpioSet(ctx, ti50.GpioTi50ChassisOpen, false)
	tpm := b.ResetAndTpmStartup(ctx, i, ti50.FfClamshell)
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC failed to boot")

	bid, err := i.ChipBID(ctx)
	th.MustSucceed(err, "failed to get board id")
	s.Logf("BID: %+v", bid)

	if addDelay {
		err = tpm.TpmvSetBoardID(testChassisOpenBIDType, testChassisOpenBIDFlags)
		th.MustSucceed(err, "failed to set board id type")

		bid, err = i.ChipBID(ctx)
		th.MustSucceed(err, "failed to get board id")
		s.Logf("BID: %+v", bid)

		if bid.Type != testChassisOpenBIDType {
			s.Fatalf("Unexpected BID type: Expected %x Got %x", testChassisOpenBIDType, bid.Type)
		}
	}

	// Configure CCD settings so that CCD open will require physical presence.
	// EnsureTestLabEnabled also does EnsureFWMPDisabled.
	fixture.EnsureTestLabEnabled(ctx, s, b.DUTControlAndreiboard, i)
	th.MustSucceed(i.TestlabOpen(ctx), "testlab open")
	th.MustSucceed(i.CCDReset(ctx), "ccd reset")
	// For MP images we must also ensure that we can open from USB and without
	// devmode (this test is only interested in the CCD open delay)
	err = i.SetCCDCapability(ctx, ti50.OpenFromUSB, ti50.CapAlways)
	th.MustSucceed(err, "set OpenFromUSB to always")
	err = i.SetCCDCapability(ctx, ti50.OpenNoDevMode, ti50.CapAlways)
	th.MustSucceed(err, "set OpenNoDevMode to always")
	// Capture caps in log.
	i.Command(ctx, "ccd")
	// Lock so we can test open.
	th.MustSucceed(i.CCDLock(ctx), "Lock CCD")

	s.Log("Timeout with no chassis_open or power button push")
	verifyOpenTimeout(ctx, i, th)

	if addDelay {
		// Testing with BID that requires 5 minute delay.
		s.Log("Timeout with 5 second chassis_open")
		b.GpioSet(ctx, ti50.GpioTi50ChassisOpen, true)
		testing.Sleep(ctx, 5*time.Second) // GoBigSleepLint: Needed for delay testing
		verifyOpenTimeout(ctx, i, th)
		i.Command(ctx, "gpioget")
		b.GpioSet(ctx, ti50.GpioTi50ChassisOpen, false)

		s.Log("Success with 5 minute chassis_open")
		b.GpioSet(ctx, ti50.GpioTi50ChassisOpen, true)
		testing.Sleep(ctx, (5*60+20)*time.Second) // GoBigSleepLint: Needed for delay testing
		i.CCDOpen(ctx)
		b.GpioSet(ctx, ti50.GpioTi50ChassisOpen, false)
	} else {
		// Testing with blank BID that does not require any delay.
		s.Log("Success with instant chassis_open")
		b.GpioSet(ctx, ti50.GpioTi50ChassisOpen, true)
		th.MustSucceed(i.CCDOpen(ctx), "Open CCD")
		b.GpioSet(ctx, ti50.GpioTi50ChassisOpen, false)
	}

}
