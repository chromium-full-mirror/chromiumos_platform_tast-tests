// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"bytes"
	"context"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/gscdevboard/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:    GSCTPMI2CCorners,
		Desc:    "Test TPM I2C corner cases",
		Timeout: 5 * time.Minute,
		Contacts: []string{
			"ti50-core@google.com",
			"jbk@google.com",
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr: []string{"group:gsc",
			"gsc_h1_shield", "gsc_dt_ab", "gsc_dt_shield", "gsc_ot_shield",
			"gsc_image_ti50",
			"gsc_nightly"},
		Fixture: fixture.GSCOpenCCD,
	})
}

func GSCTPMI2CCorners(ctx context.Context, s *testing.State) {
	b := utils.NewDevboardHelper(s)
	i := ti50.MustOpenCrOSImage(ctx, b, s)
	defer i.Close(ctx)

	// Record everything that goes on on SDA/SCL lines, for manual inspection later.
	gpioMonitor := b.GpioMonitorStart(ctx, ti50.GpioTi50DeviceI2cSda, ti50.GpioTi50DeviceI2cScl)

	tpmHandle := b.ResetAndTpmStartupForBus(ctx, i, ti50.TpmBusI2c, ti50.CcdDisconnected, ti50.FfClamshell)

	// Below the low level OpenTitanToolCommand() is used to send I2C transactions in various
	// ways, which do not form valid TPM commands.  If we find that other tests need to
	// perform similar actions, then we should consider how to create proper helper methods,
	// to avoid having to explicitly mention I2C busses on each invocation.

	// Perform irregular I2C transaction, ask for content of status register, but never read
	// the bytes.
	_, err := b.OpenTitanToolCommand(ctx, "i2c", "--bus", "0", "--addr", "80", "raw-write", "--hexdata", "01")
	if err != nil {
		s.Fatal("i2c error: ", err)
	}

	// Now read DIDVID register again.  This should cause previously enqueued status register
	// data to be discarded from the Dauntless I2C fifo.
	didVid := tpmHandle.ReadRegister(ti50.TpmRegDidVid)
	expectedDidVidValue := b.GscProperties().ExpectedDidVidValue()
	if !bytes.Equal(didVid, expectedDidVidValue) {
		s.Error("Unexpected TPM DID_VID after partial I2C transaction: ", didVid)
	}

	// Perform irregular I2C transaction, ask for other I2C addresses.
	for addr := 0; addr <= 127; addr++ {
		if addr == 0x50 {
			continue
		}
		_, err = b.OpenTitanToolCommand(ctx, "i2c", "--bus", "0", "--addr", strconv.Itoa(addr), "raw-write", "--hexdata", "01")
		if err == nil {
			// No error from opentitantool means that GSC ack'ed the address.
			s.Error("GSC responded to other address: ", addr)
		}
	}

	// Perform irregular I2C transaction, read more bytes than the size of the register.
	_, err = b.OpenTitanToolCommand(ctx, "i2c", "--bus", "0", "--addr", "80", "raw-write-read", "--hexdata", "01", "--length", "16")
	if err != nil {
		s.Fatal("i2c error: ", err)
	}

	testWedgedAddrAck(ctx, b, tpmHandle, s)
	testWedgedData(ctx, b, tpmHandle, s)
	testWedgedDataAck(ctx, b, tpmHandle, s)

	// Check one final time that that GSC still responds to DIDVID register, to make sure it
	// has not crashed or got the I2C driver into a funny state.
	didVid = tpmHandle.ReadRegister(ti50.TpmRegDidVid)
	if !bytes.Equal(didVid, expectedDidVidValue) {
		s.Error("Unexpected TPM DID_VID: ", didVid)
	}

	// Store transcript of SDA/SCL events in .vcd format, to be reviewed in e.g. Pulseview.
	events := b.GpioMonitorFinish(ctx, gpioMonitor)
	gpioMonitor.Save(ctx, events, "i2c.vcd")
}

// testWedgedAddrAck performs an irregular I2C transaction: Addressing the GSC, but then
// simulating that the AP resets and loses its state in the middle of clocking, while the GSC was
// acking the address (keeping the SDA line low, preventing the AP from issuing "start" condition,
// AKA a "wedged bus").
func testWedgedAddrAck(ctx context.Context, b utils.DevboardHelper, tpmHandle *utils.TpmHelper, s *testing.State) {
	prepareBitbanging(ctx, b, tpmHandle, s)
	defer doneBitbanging(ctx, b, s)

	// Start condition
	b.GpioSet(ctx, ti50.GpioTi50DeviceI2cSda, false)
	b.GpioSet(ctx, ti50.GpioTi50DeviceI2cScl, false)

	// 7 bits of address 0x50
	sendI2cAddress(ctx, b, 0x50)

	// Read mode
	b.GpioSet(ctx, ti50.GpioTi50DeviceI2cSda, true)
	b.GpioSet(ctx, ti50.GpioTi50DeviceI2cScl, true)
	b.GpioSet(ctx, ti50.GpioTi50DeviceI2cScl, false)

	// ACK
	verifyUnwedge(ctx, b, s, "Address ack")
}

// testWedgedData performs an irregular I2C transaction: Asking for the content of the status
// register, but reading only some of the bits, then simulating that the AP resets and loses its
// state in the middle of clocking, while the GSC was outputting a zero data bit (keeping the SDA
// line low, preventing the AP from issuing "start" condition, AKA a "wedged bus").
func testWedgedData(ctx context.Context, b utils.DevboardHelper, tpmHandle *utils.TpmHelper, s *testing.State) {
	prepareBitbanging(ctx, b, tpmHandle, s)
	defer doneBitbanging(ctx, b, s)

	// Start condition
	b.GpioSet(ctx, ti50.GpioTi50DeviceI2cSda, false)
	b.GpioSet(ctx, ti50.GpioTi50DeviceI2cScl, false)

	// 7 bits of address 0x50
	sendI2cAddress(ctx, b, 0x50)

	// Read mode
	b.GpioSet(ctx, ti50.GpioTi50DeviceI2cSda, true)
	b.GpioSet(ctx, ti50.GpioTi50DeviceI2cScl, true)
	b.GpioSet(ctx, ti50.GpioTi50DeviceI2cScl, false)

	// ACK
	b.GpioSet(ctx, ti50.GpioTi50DeviceI2cSda, true)
	b.GpioSet(ctx, ti50.GpioTi50DeviceI2cScl, true)
	if b.GpioGet(ctx, ti50.GpioTi50DeviceI2cSda) != false {
		s.Error("No ack")
	}
	b.GpioSet(ctx, ti50.GpioTi50DeviceI2cScl, false)

	// Now look for the first zero bit in the data from the GSC.  (We are reading DID_VID
	// register, and knows that the first byte will contain bits of value zero.)
	b.GpioSet(ctx, ti50.GpioTi50DeviceI2cScl, true)
	for b.GpioGet(ctx, ti50.GpioTi50DeviceI2cSda) {
		b.GpioSet(ctx, ti50.GpioTi50DeviceI2cScl, false)

		b.GpioSet(ctx, ti50.GpioTi50DeviceI2cScl, true)
	}

	verifyUnwedge(ctx, b, s, "Data read")
}

// testWedgedDataAck performs an irregular I2C transaction: Addressing the GSC, and beginning to
// write a byte (register address) then simulating that the AP resets and loses its state in the
// middle of clocking, while the GSC was acking the data byte (keeping the SDA line low,
// preventing the AP from issuing "start" condition, AKA a "wedged bus").
func testWedgedDataAck(ctx context.Context, b utils.DevboardHelper, tpmHandle *utils.TpmHelper, s *testing.State) {
	prepareBitbanging(ctx, b, tpmHandle, s)
	defer doneBitbanging(ctx, b, s)

	// Start condition
	b.GpioSet(ctx, ti50.GpioTi50DeviceI2cSda, false)
	b.GpioSet(ctx, ti50.GpioTi50DeviceI2cScl, false)

	// 7 bits of address 0x50
	sendI2cAddress(ctx, b, 0x50)

	// Write mode
	b.GpioSet(ctx, ti50.GpioTi50DeviceI2cSda, false)
	b.GpioSet(ctx, ti50.GpioTi50DeviceI2cScl, true)
	b.GpioSet(ctx, ti50.GpioTi50DeviceI2cScl, false)

	// Address ACK
	b.GpioSet(ctx, ti50.GpioTi50DeviceI2cSda, true)
	b.GpioSet(ctx, ti50.GpioTi50DeviceI2cScl, true)
	if b.GpioGet(ctx, ti50.GpioTi50DeviceI2cSda) != false {
		s.Error("No ack")
	}
	b.GpioSet(ctx, ti50.GpioTi50DeviceI2cScl, false)

	for i := 0; i < 8; i++ {
		b.GpioSet(ctx, ti50.GpioTi50DeviceI2cScl, true)
		if b.GpioGet(ctx, ti50.GpioTi50DeviceI2cSda) != true {
			s.Error("Unexpected driving of SDA by GSC")
		}
		b.GpioSet(ctx, ti50.GpioTi50DeviceI2cScl, false)
	}

	// Data byte ACK
	verifyUnwedge(ctx, b, s, "Write data ack")
}

// prepareBitbanging will ask the GSC for the value of the DID_VID register, without reading the
// value, and then reconfigure the SDA and SCL pins for bit-banging.  This means that any
// subsequent I2C read transaction should expect to receive the DID_VID value.
func prepareBitbanging(ctx context.Context, b utils.DevboardHelper, tpmHandle *utils.TpmHelper, s *testing.State) {
	// Check that that GSC still responds to DIDVID register, to make sure it has not
	// crashed or got the I2C driver into a funny state.
	didVid := tpmHandle.ReadRegister(ti50.TpmRegDidVid)
	expectedDidVidValue := b.GscProperties().ExpectedDidVidValue()
	if !bytes.Equal(didVid, expectedDidVidValue) {
		s.Error("Unexpected TPM DID_VID: ", didVid)
	}

	// Now again write the address of the DIDVID register.  Do not yet attempt to read the
	// value.
	_, err := b.OpenTitanToolCommand(ctx, "i2c", "--bus", "0", "--addr", "80", "raw-write", "--hexdata", "06")
	if err != nil {
		s.Fatal("I2c error: ", err)
	}

	b.GpioMultiSet(ctx, ti50.GpioTi50DeviceI2cSda, true, utils.GpioModeOpenDrain, utils.GpioPullUp)
	b.GpioMultiSet(ctx, ti50.GpioTi50DeviceI2cScl, true, utils.GpioModeOpenDrain, utils.GpioPullUp)
}

func doneBitbanging(ctx context.Context, b utils.DevboardHelper, s *testing.State) {
	b.GpioMultiSet(ctx, ti50.GpioTi50DeviceI2cSda, true, utils.GpioModeAlternate, utils.GpioPullUp)
	b.GpioMultiSet(ctx, ti50.GpioTi50DeviceI2cScl, true, utils.GpioModeAlternate, utils.GpioPullUp)
}

func sendI2cAddress(ctx context.Context, b utils.DevboardHelper, addr int) {
	for i := 7; i > 0; {
		i--

		b.GpioSet(ctx, ti50.GpioTi50DeviceI2cSda, (addr&(1<<i)) != 0)
		b.GpioSet(ctx, ti50.GpioTi50DeviceI2cScl, true)
		b.GpioSet(ctx, ti50.GpioTi50DeviceI2cScl, false)
	}
}

func verifyUnwedge(ctx context.Context, b utils.DevboardHelper, s *testing.State, testcase string) {
	b.GpioSet(ctx, ti50.GpioTi50DeviceI2cSda, true)
	b.GpioSet(ctx, ti50.GpioTi50DeviceI2cScl, true)
	testTime := time.Now()

	if b.GpioGet(ctx, ti50.GpioTi50DeviceI2cSda) != false {
		s.Errorf("%s: I2C device did not pull SDA low", testcase)
	}
	var elapsedTime time.Duration
	for ; elapsedTime < 5*time.Second; elapsedTime = time.Since(testTime) {
		if b.GpioGet(ctx, ti50.GpioTi50DeviceI2cSda) == true {
			s.Logf("%s: Wedge ended after %d ms", testcase, int64(elapsedTime/time.Millisecond))
			return
		}
	}

	s.Error("I2C bus wedged")

	b.GpioSet(ctx, ti50.GpioTi50DeviceI2cScl, false)
	for !b.GpioGet(ctx, ti50.GpioTi50DeviceI2cSda) {
		b.GpioSet(ctx, ti50.GpioTi50DeviceI2cScl, true)
		b.GpioSet(ctx, ti50.GpioTi50DeviceI2cScl, false)
	}

	// Stop condition
	b.GpioSet(ctx, ti50.GpioTi50DeviceI2cSda, false)
	b.GpioSet(ctx, ti50.GpioTi50DeviceI2cScl, true)
	b.GpioSet(ctx, ti50.GpioTi50DeviceI2cSda, true)
	if !b.GpioGet(ctx, ti50.GpioTi50DeviceI2cSda) || !b.GpioGet(ctx, ti50.GpioTi50DeviceI2cScl) {
		s.Fatalf("%s: Unable to get out of wedge", testcase)
	}
}
