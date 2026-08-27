// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/gscdevboard/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"
	"go.chromium.org/tast/core/testing"
)

type ccdCapabilitiesOpenFromUSB struct {
	capState             ti50.CCDCapState
	expectCCDCanBeOpened bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:    CCDCapabilitiesOpenFromUSB,
		Desc:    "Test the OpenFromUSB CCD capability using the console and `gsctool`",
		Timeout: 30 * time.Second,
		Contacts: []string{
			"cros-hwsec@google.com", // CrOS GSC Developers
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr: []string{
			"group:gsc",
			"gsc_dt_ab", "gsc_dt_shield", "gsc_h1_shield", "gsc_ot_shield", "gsc_ot_fpga_cw310",
			"gsc_image_ti50",
			"gsc_nightly"},
		Fixture: fixture.GSCOpenCCD,
		Params: []testing.Param{{
			Name: "cap_always",
			Val: ccdCapabilitiesOpenFromUSB{
				capState:             ti50.CapAlways,
				expectCCDCanBeOpened: true,
			},
		}, {
			Name: "cap_if_opened",
			Val: ccdCapabilitiesOpenFromUSB{
				capState:             ti50.CapIfOpened,
				expectCCDCanBeOpened: false,
			},
		}},
		// TODO(b/240149504): Add `cap_unless_locked` test for Cr50 only
	})
}

func CCDCapabilitiesOpenFromUSB(ctx context.Context, s *testing.State) {
	userParams := s.Param().(ccdCapabilitiesOpenFromUSB)
	b := utils.NewDevboardHelper(s, s.FixtValue().(*fixture.Value))
	i := ti50.MustOpenCrOSImage(ctx, b, s, b.TestbedType)
	defer i.Close(ctx)
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}

	s.Log("Resetting GSC and starting up")
	b.GpioSet(ctx, ti50.GpioTi50ChassisOpen, true)
	_ = b.ResetAndTpmStartup(ctx, i, ti50.CCDModeOn, ti50.FfClamshell)
	b.WaitUntilCCDConnected(ctx)

	// Start with CCD open
	th.MustSucceed(i.CCDOpen(ctx), "Failed to open CCD")

	// Make sure we set the other CCD open related capabilities to always so CCD
	// opens immediately when testing.
	ccdStates := map[ti50.CCDCap]ti50.CCDCapState{
		ti50.OpenFromUSB:     userParams.capState,
		ti50.OpenNoDevMode:   ti50.CapAlways,
		ti50.OpenNoLongPP:    ti50.CapAlways,
		ti50.UnlockNoShortPP: ti50.CapAlways,
	}
	th.MustSucceed(i.SetCCDCapabilities(ctx, ccdStates), "Failed to set CCD open related capabilities")

	b.GpioSet(ctx, ti50.GpioTi50ChassisOpen, false)
	th.MustSucceed(i.CCDLock(ctx), "Lock CCD")

	// Try to reopen CCD from the console
	_ = i.CCDOpen(ctx)
	ccdIsOpen, err := i.IsCCDOpen(ctx)
	th.MustSucceed(err, "Failed to determine if CCD is open")
	checkCCDOpenExpectation(s, userParams.expectCCDCanBeOpened, ccdIsOpen, "the console")

	// Lock CCD again since it might be open
	open, err := i.IsCCDOpen(ctx)
	th.MustSucceed(err, "Get CCD open state")
	if open {
		th.MustSucceed(i.CCDLock(ctx), "Lock CCD")
	}

	// Try to reopen using `gsctool`, tpmv command over USB, ignore error and
	// output
	// DT uses the -D arg. H1 does not.
	_, _ = b.GSCToolCommand(ctx, "", "--ccd_open")
	ccdIsOpen, err = i.IsCCDOpen(ctx)
	th.MustSucceed(err, "Failed to determine if CCD is open")
	checkCCDOpenExpectation(s, userParams.expectCCDCanBeOpened, ccdIsOpen, "gsctool")
}

func checkCCDOpenExpectation(s *testing.State, expectedOpen, isOpen bool, source string) {
	if expectedOpen != isOpen {
		can := "can"
		if !expectedOpen {
			can = "cannot"
		}
		got := "open"
		if !isOpen {
			got = "locked"
		}
		s.Error("Expected CCD " + can + " be opened from " + source + ", but found CCD to be " + got + " after attempting to open")
	}
}
