// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"context"
	"reflect"
	"time"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/gscdevboard/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"

	"go.chromium.org/tast/core/testing"
)

type cCDCapabilitiesFlashAP struct {
	capState                        ti50.CCDCapState
	expectSpiAvailableWhenCCDLocked bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:    CCDCapabilitiesFlashAP,
		Desc:    "Test the FlashAP CCD capability using a GSC dev board",
		Timeout: 30 * time.Second,
		Contacts: []string{
			"chromeos-faft@google.com", // CrOS Firmware Developers
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr:         []string{"group:gsc", "gsc_dt_ab", "gsc_dt_shield", "gsc_image_ti50", "gsc_nightly", "gsc_h1_shield"},
		Fixture:      fixture.GSCOpenCCD,
		Params: []testing.Param{{
			Name: "cap_default",
			Val: cCDCapabilitiesFlashAP{
				capState:                        ti50.CapDefault,
				expectSpiAvailableWhenCCDLocked: false,
			},
		}, {
			Name: "cap_always",
			Val: cCDCapabilitiesFlashAP{
				capState:                        ti50.CapAlways,
				expectSpiAvailableWhenCCDLocked: true,
			},
		}, {
			Name: "cap_if_opened",
			Val: cCDCapabilitiesFlashAP{
				capState:                        ti50.CapIfOpened,
				expectSpiAvailableWhenCCDLocked: false,
			},
		}},
		// TODO(b:240148863): Add `cap_unless_locked` test for Cr50 only
	})
}

func CCDCapabilitiesFlashAP(ctx context.Context, s *testing.State) {
	userParams := s.Param().(cCDCapabilitiesFlashAP)
	b := utils.NewDevboardHelper(s)
	i := ti50.MustOpenCrOSImage(ctx, b, s)
	defer i.Close(ctx)

	b.GpioApplyStrap(ctx, ti50.CcdSuzyQ)
	_ = b.ResetAndTpmStartup(ctx, i, ti50.CcdSuzyQ, ti50.FfClamshell)

	// Set capabilities into their correct states
	if err := i.CCDOpen(ctx); err != nil {
		s.Fatal("Failed to open CCD: ", err)
	}
	ccdStates := map[ti50.CCDCap]ti50.CCDCapState{
		ti50.FlashAP: userParams.capState,
		ti50.FlashEC: ti50.CapAlways,
	}
	if err := i.SetCCDCapabilities(ctx, ccdStates); err != nil {
		s.Fatal("Failed to set CCD open related capabilities: ", err)
	}
	if err := i.CCDLock(ctx); err != nil {
		s.Fatal("Failed to lock CCD: ", err)
	}

	// TODO(b:240149552): Improve this test by using `flashrom` directly to
	// attempt to read from the SPI chip.

	// Enable the AP SPI bridge
	defer b.GscUsbSpiBridge(ctx, ti50.DisableSpiBridge)
	err := b.GscUsbSpiBridge(ctx, ti50.EnableApSpiBridge)
	// We always expect our SPI bridge enable call to succeed if on Cr50. On
	// Ti50, this call fails if the SPI bridge is unavailable.
	if userParams.expectSpiAvailableWhenCCDLocked || b.TestbedType == ti50.GscH1Shield {
		if err != nil {
			s.Error("Failed to enable the AP SPI bridge when we expected success: ", err)
		}
	} else {
		if err == nil {
			s.Error("Enabled the AP SPI bridge when we expected failure")
		}
	}

	// SPI bridge get configuration packet request
	requestData := []byte{0, 0} // Get configuration packet request
	response, err := b.GscUsbSpiInterfaceTransaction(ctx, requestData)
	if userParams.expectSpiAvailableWhenCCDLocked {
		// Expect an 8 byte response on success, we don't care about the content.
		if err != nil {
			s.Error("Failed to get the USB SPI configuration response: ", err)
		} else if len(response) != 8 {
			s.Error("Expected SPI bridge response of 8B, but got ", response)
		}
	} else {
		// Handle Cr50 and Ti50 differently
		if b.TestbedType == ti50.GscH1Shield {
			// On Cr50, we get a Response Start Packet containing a SPI bridge disabled error
			expected := []byte{0x05, 0x00, 0x05, 0x00}
			if !reflect.DeepEqual(response, expected) {
				s.Error("Expected a Response Start Packet containing a SPI bridge disabled error, but got ", response)
			}
		} else {
			// On Ti50, we get a USB error if the bridge is disabled
			if err == nil {
				s.Error("Expected SPI interface to be disabled, but was able to successfully transact")
			}
		}

	}
}
