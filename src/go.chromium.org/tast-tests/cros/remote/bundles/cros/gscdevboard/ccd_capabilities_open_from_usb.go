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

type cCDCapabilitiesOpenFromUSB struct {
	capState             ti50.CCDCapState
	expectCCDCanBeOpened bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:    CCDCapabilitiesOpenFromUSB,
		Desc:    "Test the OpenFromUSB CCD capability using the console and `gsctool`",
		Timeout: 30 * time.Second,
		Contacts: []string{
			"chromeos-faft@google.com", // CrOS Firmware Developers
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr:         []string{"group:gsc", "gsc_dt_ab", "gsc_dt_shield", "gsc_image_ti50", "gsc_nightly"},
		Fixture:      fixture.Ti50CcdOpen,
		Params: []testing.Param{{
			Name: "cap_always",
			Val: cCDCapabilitiesOpenFromUSB{
				capState:             ti50.CapAlways,
				expectCCDCanBeOpened: true,
			},
		}, {
			Name: "cap_if_opened",
			Val: cCDCapabilitiesOpenFromUSB{
				capState:             ti50.CapIfOpened,
				expectCCDCanBeOpened: false,
			},
		}},
		// TODO(b/240149504): Add `cap_unless_locked` test for Cr50 only
	})
}

func CCDCapabilitiesOpenFromUSB(ctx context.Context, s *testing.State) {
	userParams := s.Param().(cCDCapabilitiesOpenFromUSB)
	b := utils.NewDevboardHelper(s)
	i := ti50.MustOpenCrOSImage(ctx, b, s)
	defer i.Close(ctx)

	// Open CCD + chassis when finished
	defer func() {
		b.GpioSet(ctx, ti50.GpioTi50ChassisOpen, true)
		if err := i.CCDOpen(ctx); err != nil {
			s.Fatal("Failed to open CCD during cleanup")
		}
	}()

	s.Log("Resetting GSC and starting up")
	b.GpioSet(ctx, ti50.GpioTi50ChassisOpen, true)
	_ = b.ResetAndTpmStartup(ctx, i, ti50.TpmBusSpi, ti50.CcdSuzyQ, ti50.FfClamshell)

	if err := i.CCDOpen(ctx); err != nil {
		s.Fatal("Failed to open CCD")
	}

	// Make sure we set the other CCD open related capabilities to always so CCD
	// opens immediately when testing.
	ccdStates := map[ti50.CCDCap]ti50.CCDCapState{
		ti50.OpenFromUSB:     userParams.capState,
		ti50.OpenNoDevMode:   ti50.CapAlways,
		ti50.OpenNoLongPP:    ti50.CapAlways,
		ti50.UnlockNoShortPP: ti50.CapAlways,
	}
	if err := i.SetCCDCapabilities(ctx, ccdStates); err != nil {
		s.Fatal("Failed to set CCD open related capabilities: ", err)
	}

	b.GpioSet(ctx, ti50.GpioTi50ChassisOpen, false)
	if err := i.CCDLock(ctx); err != nil {
		s.Fatal("Failed to lock CCD")
	}

	// Try to reopen CCD from the console
	_ = i.CCDOpen(ctx)
	ccdIsOpen, err := i.IsCCDOpen(ctx)
	if err != nil {
		s.Fatal("Failed to determine if CCD is open")
	}
	checkCCDOpenExpectation(s, userParams.expectCCDCanBeOpened, ccdIsOpen, "the console")

	// Lock CCD again since it might be open
	if err := i.CCDLock(ctx); err != nil {
		s.Fatal("Failed to lock CCD")
	}

	// Try to reopen using `gsctool`, tpmv command over USB, ignore error and
	// output
	// TODO(b/240149504): Support Cr50 here
	_, _ = b.GSCToolCommand(ctx, "", "-D", "--ccd_open")
	ccdIsOpen, err = i.IsCCDOpen(ctx)
	if err != nil {
		s.Fatal("Failed to determine if CCD is open")
	}
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
