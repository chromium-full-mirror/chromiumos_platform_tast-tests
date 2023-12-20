// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"bytes"
	"context"
	"math/rand"
	"time"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/gscdevboard/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"

	"go.chromium.org/tast/core/testing"
)

type gSCI2CBridgeParam struct {
	capState                         ti50.CCDCapState
	expectInterfaceOpenWhenCCDLocked bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:    GSCI2CBridge,
		Desc:    "Test abilty of the GSC to tunnel I2C requests through USB CCD",
		Timeout: 30 * time.Second,
		Contacts: []string{
			"ti50-core@google.com",
			"jbk@google.com",
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr:         []string{"group:gsc", "gsc_dt_shield", "gsc_image_ti50", "gsc_nightly"},
		Fixture:      fixture.GSCOpenCCD,
		Params: []testing.Param{{
			Name: "cap_default",
			Val: gSCI2CBridgeParam{
				capState:                         ti50.CapDefault,
				expectInterfaceOpenWhenCCDLocked: false,
			},
		}, {
			Name: "cap_always",
			Val: gSCI2CBridgeParam{
				capState:                         ti50.CapAlways,
				expectInterfaceOpenWhenCCDLocked: true,
			},
		}, {
			Name: "cap_if_opened",
			Val: gSCI2CBridgeParam{
				capState:                         ti50.CapIfOpened,
				expectInterfaceOpenWhenCCDLocked: false,
			},
		}},
		// TODO(b:240149552): Add `cap_unless_locked` test for Cr50 only
	})
}

func runI2CTransaction(ctx context.Context, ccdIndex byte, bus ti50.I2cBusName, address byte, expectInterfaceOpen bool, r *rand.Rand, b utils.DevboardHelper, s *testing.State) {
	// I2C write of between 1 and 32 random bytes
	writeData := make([]byte, 1+r.Intn(32))
	if _, err := r.Read(writeData); err != nil {
		s.Fatalf("Error generating random data: %s", err)
	}
	// I2C read of between 1 and 32 random bytes
	readData := make([]byte, 1+r.Intn(32))
	if _, err := r.Read(readData); err != nil {
		s.Fatalf("Error generating random data: %s", err)
	}

	s.Logf("Writing %d bytes, reading %d bytes", len(writeData), len(readData))

	// Set up HyperDebug to be ready to repond to I2C transactions.
	b.I2CDeviceMode(ctx, bus, address)
	b.I2CDevicePrepareRead(ctx, bus, readData)

	// Instruct GSC to perform a write-then-read transaction on I2C bus.
	requestHeader := []byte{ccdIndex, address, byte(len(writeData)), byte(len(readData))}
	response, err := b.GscUsbI2cInterfaceTransaction(ctx, append(requestHeader, writeData...))
	if err != nil {
		s.Fatalf("Got error: %s", err)
	}
	// Read response from GSC: two byte status, two byte fill, followed by data.
	var expectedResponse []byte
	if expectInterfaceOpen {
		// Expect success, with data.
		expectedResponse = append([]byte{0, 0, 0, 0}, readData...)
	} else {
		// Expect permission error.
		expectedResponse = []byte{6, 0, 0, 0}
	}
	if !bytes.Equal(expectedResponse, response) {
		s.Errorf("Expected I2C interface response %v, but got %v", expectedResponse, response)
	}

	transcript := b.I2CDeviceGetStatus(ctx, bus)
	if !expectInterfaceOpen {
		// Verify that nothing happened on the I2C bus.
		if len(transcript.Transfers) != 0 {
			s.Errorf("Unexpected transactions on bus %s: %v", bus, transcript)
		}
		return
	}

	// Verify expected sequence of transfers on the I2C bus.
	if len(transcript.Transfers) != 2 || transcript.Transfers[0].Direction != utils.I2CWrite || transcript.Transfers[1].Direction != utils.I2CRead {
		s.Errorf("Unexpected sequence of transactions on bus %s: %v", bus, transcript)
	} else {
		if !bytes.Equal(transcript.Transfers[0].Data, writeData) {
			s.Errorf("Unexpected data %s sent by GSC on bus %s", transcript.Transfers[0].Data, bus)
		}
		if transcript.Transfers[1].Len != len(readData) {
			s.Errorf("Unexpected read length %d by GSC on bus %s", transcript.Transfers[1].Len, bus)
		}
	}
}

func GSCI2CBridge(ctx context.Context, s *testing.State) {
	userParams := s.Param().(gSCI2CBridgeParam)
	b := utils.NewDevboardHelper(s)
	i := ti50.MustOpenCrOSImage(ctx, b, s)
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}
	r := rand.New(rand.NewSource(42))

	i2cBusses := b.GscProperties().GscHostI2cBusses()

	b.ResetWithStraps(ctx, ti50.CcdSuzyQ)
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")

	// Set capabilities into their correct states
	th.MustSucceed(i.CCDOpen(ctx), "Failed to open CCD")
	ccdStates := map[ti50.CCDCap]ti50.CCDCapState{
		ti50.I2C: userParams.capState,
	}
	th.MustSucceed(i.SetCCDCapabilities(ctx, ccdStates), "Failed to set CCD open related capabilities")

	// With CCD open, I2C tunneling should always be allowed.
	for index, name := range i2cBusses {
		for addr := 8; addr < 112; addr++ {
			runI2CTransaction(ctx, index, name, byte(addr), true, r, b, s)
		}
	}

	th.MustSucceed(i.CCDLock(ctx), "Failed to lock CCD")

	// With CCD closed, I2C tunneling should be allowed only according to capabilities.
	addr := 8 + r.Intn(112)
	for index, name := range i2cBusses {
		runI2CTransaction(ctx, index, name, byte(addr), userParams.expectInterfaceOpenWhenCCDLocked, r, b, s)
	}
}
