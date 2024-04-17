// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"bytes"
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/gscdevboard/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"

	"go.chromium.org/tast/core/testing"
)

type cCDCapabilitiesFlashEC struct {
	capState                         ti50.CCDCapState
	expectInterfaceOpenWhenCCDLocked bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:    CCDCapabilitiesFlashEC,
		Desc:    "Test the FlashEC CCD capability controls flashing the EC with I2C and SPI",
		Timeout: 30 * time.Second,
		Contacts: []string{
			"gsc-sheriff@google.com", // CrOS GSC Developers
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr:         []string{"group:gsc", "gsc_dt_ab", "gsc_dt_shield", "gsc_image_ti50", "gsc_nightly"},
		Fixture:      fixture.GSCOpenCCD,
		Params: []testing.Param{{
			Name: "cap_default",
			Val: cCDCapabilitiesFlashEC{
				capState:                         ti50.CapDefault,
				expectInterfaceOpenWhenCCDLocked: false,
			},
		}, {
			Name: "cap_always",
			Val: cCDCapabilitiesFlashEC{
				capState:                         ti50.CapAlways,
				expectInterfaceOpenWhenCCDLocked: true,
			},
		}, {
			Name: "cap_if_opened",
			Val: cCDCapabilitiesFlashEC{
				capState:                         ti50.CapIfOpened,
				expectInterfaceOpenWhenCCDLocked: false,
			},
		}},
		// TODO(b:240149552): Add `cap_unless_locked` test for Cr50 only
	})
}

func CCDCapabilitiesFlashEC(ctx context.Context, s *testing.State) {
	userParams := s.Param().(cCDCapabilitiesFlashEC)
	b := utils.NewDevboardHelper(s)
	i := ti50.MustOpenCrOSImage(ctx, b, s)
	defer i.Close(ctx)

	b.GpioApplyStrap(ctx, ti50.CcdSuzyQ)
	_ = b.ResetAndTpmStartup(ctx, i, ti50.CcdSuzyQ, ti50.FfClamshell)
	b.WaitUntilCCDConnected(ctx)

	// Set capabilities into their correct states
	if err := i.CCDOpen(ctx); err != nil {
		s.Fatal("Failed to open CCD: ", err)
	}
	ccdStates := map[ti50.CCDCap]ti50.CCDCapState{
		ti50.FlashAP: ti50.CapAlways,
		ti50.FlashEC: userParams.capState,
	}
	if err := i.SetCCDCapabilities(ctx, ccdStates); err != nil {
		s.Fatal("Failed to set CCD open related capabilities: ", err)
	}

	// Make sure we disable the SPI bridge at the end of the test
	defer b.GscUsbSpiBridge(ctx, ti50.DisableSpiBridge)

	// Perform a read, right before the ITE waveform, and ensure that response
	// size of ITE waveform request is correct (See b/322166906)
	verifyI2CRead(ctx, s, &b)
	verifyIteSyncCommand(ctx, s, &b, true)
	verifyEcSpiBridgeEnable(ctx, s, &b, true)
	verifySpiBridgeState(ctx, s, &b, true)
	b.GscUsbSpiBridge(ctx, ti50.DisableSpiBridge)

	if err := i.CCDLock(ctx); err != nil {
		s.Fatal("Failed to lock CCD: ", err)
	}

	verifyIteSyncCommand(ctx, s, &b, userParams.expectInterfaceOpenWhenCCDLocked)
	verifyEcSpiBridgeEnable(ctx, s, &b, userParams.expectInterfaceOpenWhenCCDLocked)
	verifySpiBridgeState(ctx, s, &b, userParams.expectInterfaceOpenWhenCCDLocked)

	// TODO(b:240149552): Test the EC UART bridge?
}

func verifyIteSyncCommand(ctx context.Context, s *testing.State, b *utils.DevboardHelper, expectSuccess bool) {
	// Special ITE sync command
	requestData := []byte{0, 0x78, 1, 0, 0}
	response, err := b.GscUsbI2cInterfaceTransaction(ctx, requestData)
	if err != nil {
		s.Fatalf("Got error: %s", err)
	}
	// ITE sync responses
	expectedResponse := []byte{0, 0, 0, 0}
	if !expectSuccess {
		expectedResponse = []byte{6, 0, 0, 0}
	}
	if !bytes.Equal(expectedResponse, response) {
		s.Errorf("Expected ITE I2C interface response %v, but got %v", expectedResponse, response)
	}
}

func verifyI2CRead(ctx context.Context, s *testing.State, b *utils.DevboardHelper) {
	// Read address 0x60 for 2 bytes. There is nothing at 0x60, so we should
	// get a timeout error.
	requestData := []byte{0, 0x60, 0, 2}
	response, err := b.GscUsbI2cInterfaceTransaction(ctx, requestData)
	if err != nil {
		s.Fatalf("Got error: %s", err)
	}
	// Expected Timeout failure response
	expectedResponse := []byte{1, 0, 0, 0, 0, 0}
	if !bytes.Equal(expectedResponse, response) {
		s.Errorf("Expected I2C interface response %v, but got %v", expectedResponse, response)
	}
}

func verifyEcSpiBridgeEnable(ctx context.Context, s *testing.State, b *utils.DevboardHelper, expectSuccess bool) {
	err := b.GscUsbSpiBridge(ctx, ti50.EnableEcSpiBridge)
	if expectSuccess {
		if err != nil {
			s.Error("Failed to enable the EC SPI bridge when we expected success: ", err)
		}
	} else {
		if err == nil {
			s.Error("Enabled the EC SPI bridge when we expected failure")
		}
	}
}

func verifySpiBridgeState(ctx context.Context, s *testing.State, b *utils.DevboardHelper, expectSuccess bool) {
	// SPI bridge get configuration packet request
	requestData := []byte{0, 0} // Get configuration packet request
	response, err := b.GscUsbSpiInterfaceTransaction(ctx, requestData)
	if expectSuccess {
		// Expect an 8 byte response on success, we don't care about the content.
		if err != nil {
			s.Error("Failed to get the USB SPI configuration response: ", err)
		} else if len(response) != 8 {
			s.Error("Expected SPI bridge response of 8B, but got ", response)
		}
	} else {
		// We get a USB error if the bridge is disabled
		if err == nil {
			s.Error("Expected SPI interface to be disabled, but was able to successfully transact")
		}
	}
}
