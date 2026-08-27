// Copyright 2025 The ChromiumOS Authors
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

func init() {
	testing.AddTest(&testing.Test{
		Func:    GSCI2CBridgeCorners,
		Desc:    "Test abilty of the GSC to tunnel I2C requests through USB CCD",
		Timeout: 30 * time.Second,
		Contacts: []string{
			"cros-hwsec@google.com", // CrOS GSC Developers
			"jbk@google.com",
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr: []string{"group:gsc",
			"gsc_h1_shield", "gsc_dt_shield", "gsc_ot_shield", "gsc_ot_fpga_cw310",
			"gsc_image_ti50",
			"gsc_nightly"},
		Fixture: fixture.GSCOpenCCD,
	})
}

func runI2CCornerTransaction(ctx context.Context, ccdIndex byte, bus ti50.I2cBusName, address byte, expectJammedBus bool, r *rand.Rand, b utils.DevboardHelper, s *testing.State) {
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

	// Set up HyperDebug to be ready to respond to I2C transactions.
	if !expectJammedBus {
		b.I2CDeviceMode(ctx, bus, address)
		b.I2CDevicePrepareRead(ctx, bus, readData)
	}

	// Instruct GSC to perform a write-then-read transaction on I2C bus.
	requestHeader := []byte{ccdIndex, address, byte(len(writeData)), byte(len(readData))}
	response, err := b.GscUsbI2cInterfaceTransaction(ctx, append(requestHeader, writeData...))
	if err != nil {
		s.Fatalf("Got error: %s", err)
	}
	// Read response from GSC: two byte status, two byte fill, followed by data.
	var expectedResponse []byte
	if expectJammedBus {
		// Expect timeout error, with all zero data.
		expectedResponse = append([]byte{0, 128, 0, 0}, make([]byte, len(readData))...)
	} else {
		// Expect success, with data.
		expectedResponse = append([]byte{0, 0, 0, 0}, readData...)
	}
	if !bytes.Equal(expectedResponse, response) {
		s.Errorf("Expected I2C interface response %v, but got %v", expectedResponse, response)
	}

	if expectJammedBus {
		return
	}

	transcript := b.I2CDeviceGetStatus(ctx, bus)

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

func GSCI2CBridgeCorners(ctx context.Context, s *testing.State) {
	b := utils.NewDevboardHelper(s, s.FixtValue().(*fixture.Value))
	r := rand.New(rand.NewSource(42))

	i2cBusses := b.GscProperties().GscHostI2cBusses()

	b.ResetWithStraps(ctx, ti50.CCDModeOn)
	b.WaitUntilCCDConnected(ctx)

	// Try communicating with all possible I2C addresses
	for index, i2cBus := range i2cBusses {
		for addr := 8; addr < 112; addr++ {
			if addr == 52 {
				// OpenTitan Teacup board has an LED driver I2C device on the bus,
				// using address 52.
				continue
			}
			runI2CCornerTransaction(ctx, index, i2cBus.BusName, byte(addr), false, r, b, s)
		}
	}

	addr := 88

	// It seems that Cr50 suffers from a flaw, that if asked to perform a I2C operation while
	// the bus is unpowered, it gets stuck in a state of pulling SCL low indefinitely, even
	// after power to the pullup resistors is restored.  For now, skip this part of testing on
	// Cr50.
	if b.TestbedType == ti50.GscH1Shield {
		return
	}

	s.Log("Simulating unpowered/jammed bus")
	// Drive either or both bus to ground.  This simulates a bus with unpopulated pullups, or
	// some other malfunction.  We do not care that much what error Ti50 will return, but want
	// to verify that if the jamming is removed, Ti50 is able to communicate afterwards.
	for index, i2cBus := range i2cBusses {
		for jam := 0; jam <= 2; jam++ {
			b.GpioMultiSet(ctx, i2cBus.ClockPin, jam&0x01 != 0, utils.GpioModeOpenDrain, utils.GpioPullUp)
			b.GpioMultiSet(ctx, i2cBus.DataPin, jam&0x02 != 0, utils.GpioModeOpenDrain, utils.GpioPullUp)
			runI2CCornerTransaction(ctx, index, i2cBus.BusName, byte(addr), true, r, b, s)
			b.GpioMultiSet(ctx, i2cBus.DataPin, true, utils.GpioModeOpenDrain, utils.GpioPullUp)
			b.GpioMultiSet(ctx, i2cBus.ClockPin, true, utils.GpioModeOpenDrain, utils.GpioPullUp)
			if !b.GpioGet(ctx, i2cBus.DataPin) {
				s.Fatalf("GSC keeps SDA low on bus %s", i2cBus.BusName)
			}
			if !b.GpioGet(ctx, i2cBus.ClockPin) {
				s.Fatalf("GSC keeps SCL low on bus %s", i2cBus.BusName)
			}

			b.GpioMultiSet(ctx, i2cBus.DataPin, true, utils.GpioModeAlternate, utils.GpioPullUp)
			b.GpioMultiSet(ctx, i2cBus.ClockPin, true, utils.GpioModeAlternate, utils.GpioPullUp)
		}
	}

	s.Log("Testing again with power restored")
	for index, i2cBus := range i2cBusses {
		runI2CCornerTransaction(ctx, index, i2cBus.BusName, byte(addr), false, r, b, s)
	}
}
