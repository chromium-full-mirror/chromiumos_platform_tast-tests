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

func init() {
	testing.AddTest(&testing.Test{
		Func:    Ti50UartThroughput,
		Desc:    "Tests forwarding between GSC UARTs and USB at sustained maximum throughput",
		Timeout: 5 * time.Minute,
		Contacts: []string{
			"chromeos-faft@google.com", // CrOS Firmware Developers
			"jbk@chromium.org",         // Test Author
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr:         []string{"group:gsc", "gsc_dt_ab", "gsc_dt_shield", "gsc_ot_fpga_cw310", "gsc_image_ti50"},
		Fixture:      fixture.Ti50CcdOpen,
	})
}

const (
	uartThroughputBlockSize            int = 64
	uartThroughputNumWarmupBlocks      int = 16
	uartThroughputNumMeasurementBlocks int = 32768

	// uartBitsPerByte represents how many bit times it takes to transmit a byte on UART.
	uartBitsPerByte int = 10

	uartThroughputNominalBps   float64 = 115200.0
	uartThroughputBpsTolerance float64 = 400.0
)

func Ti50UartThroughput(ctx context.Context, s *testing.State) {
	const crLf = "\r\n"
	const ecMagic byte = 0xEC
	const apMagic byte = 0xA5
	const fpmcuMagic byte = 0xF5

	f := s.FixtValue().(*fixture.Value)
	b := utils.NewDevboardHelper(f, s)
	i := ti50.NewCrOSImage(b)

	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}

	s.Log("(Re)starting ti50")
	b.GpioApplyStrap(ctx, ti50.CcdSuzyQ, ti50.ServoMicroDisconnected)

	// Simulate the AP processor being turned on, in order to enable AP forwarding.
	th.MustSucceed(b.Reset(ctx), "Reset board")
	th.MustSucceed(i.WaitUntilBooted(ctx), "Ti50 revives after reboot")
	b.GpioSet(ctx, ti50.GpioTi50PltRstL, true)

	ecUart := f.DevBoard().PhysicalUart(ti50.UartEC, time.Second)
	ecCcd := f.DevBoard().CcdSerialInterface(ti50.UartEC, time.Second)
	th.MustSucceed(ecCcd.Open(ctx), "Failed to open EC ccd")
	th.MustSucceed(ecUart.Open(ctx), "Failed to open EC uart")
	apUart := f.DevBoard().PhysicalUart(ti50.UartAP, time.Second)
	apCcd := f.DevBoard().CcdSerialInterface(ti50.UartAP, time.Second)
	th.MustSucceed(apCcd.Open(ctx), "Failed to open AP ccd")
	th.MustSucceed(apUart.Open(ctx), "Failed to open AP uart")
	fpmcuUart := f.DevBoard().PhysicalUart(ti50.UartFPMCU, time.Second)
	fpmcuCcd := f.DevBoard().CcdSerialInterface(ti50.UartFPMCU, time.Second)
	th.MustSucceed(fpmcuCcd.Open(ctx), "Failed to open FPMCU ccd")
	th.MustSucceed(fpmcuUart.Open(ctx), "Failed to open FPMCU uart")

	// Flush out any "DATA LOST" message along with other queued-up data.
	ecUart.WriteSerial(ctx, []byte(crLf))
	th.MustSucceed(ecCcd.ClearInput(ctx), "Error clearing buffer")
	th.MustSucceed(ecUart.ClearInput(ctx), "Error clearing buffer")

	apUart.WriteSerial(ctx, []byte(crLf))
	th.MustSucceed(apCcd.ClearInput(ctx), "Error clearing buffer")
	th.MustSucceed(apUart.ClearInput(ctx), "Error clearing buffer")

	fpmcuUart.WriteSerial(ctx, []byte(crLf))
	th.MustSucceed(fpmcuCcd.ClearInput(ctx), "Error clearing buffer")
	th.MustSucceed(fpmcuUart.ClearInput(ctx), "Error clearing buffer")

	// Start out by sending some initial data on all three UARTS, to fill up the buffers.

	var sendBlockNo = 0
	var recvBlockNo = 0

	for ; sendBlockNo < uartThroughputNumWarmupBlocks; sendBlockNo++ {
		sendIteration(ctx, s, f, th, ecUart, ecMagic, sendBlockNo)
		sendIteration(ctx, s, f, th, apUart, apMagic, sendBlockNo)
		sendIteration(ctx, s, f, th, fpmcuUart, fpmcuMagic, sendBlockNo)
	}

	// Now, read and verify one 64-byte block of data from each of the three CCD endpoints,
	// followed by transmitting yet another block on each UART.  This way, we ensure that the
	// amount of in-transit data is bounded.
	start := time.Now()
	for ; recvBlockNo < uartThroughputNumWarmupBlocks+uartThroughputNumMeasurementBlocks; recvBlockNo, sendBlockNo = recvBlockNo+1, sendBlockNo+1 {
		recvIteration(ctx, s, f, th, ecCcd, ecMagic, recvBlockNo)
		recvIteration(ctx, s, f, th, apCcd, apMagic, recvBlockNo)
		recvIteration(ctx, s, f, th, fpmcuCcd, fpmcuMagic, recvBlockNo)
		sendIteration(ctx, s, f, th, ecUart, ecMagic, sendBlockNo)
		sendIteration(ctx, s, f, th, apUart, apMagic, sendBlockNo)
		sendIteration(ctx, s, f, th, fpmcuUart, fpmcuMagic, sendBlockNo)
	}
	elapsed := time.Since(start)

	var bps = float64(uartThroughputNumMeasurementBlocks*uartThroughputBlockSize*uartBitsPerByte) / elapsed.Seconds()
	s.Logf("Effective transfer speed: %.02f kbps", bps/1000)
	if bps > uartThroughputNominalBps+uartThroughputBpsTolerance {
		s.Error("Transfer speed too fast, something is not right")
	}
	if bps < uartThroughputNominalBps-uartThroughputBpsTolerance {
		s.Error("Transfer speed too slow, HyperDebug may not be performing")
	}

	// Gracefully shut down connections.
	th.MustSucceed(ecCcd.ClearInput(ctx), "Error clearing buffer")
	th.MustSucceed(apCcd.ClearInput(ctx), "Error clearing buffer")
	th.MustSucceed(fpmcuCcd.ClearInput(ctx), "Error clearing buffer")

	th.MustSucceed(ecCcd.Close(ctx), "Failed to close EC ccd")
	th.MustSucceed(ecUart.Close(ctx), "Failed to close EC uart")
	th.MustSucceed(apCcd.Close(ctx), "Failed to close AP ccd")
	th.MustSucceed(apUart.Close(ctx), "Failed to close AP uart")
	th.MustSucceed(fpmcuCcd.Close(ctx), "Failed to close FPMCU ccd")
	th.MustSucceed(fpmcuUart.Close(ctx), "Failed to close FPMCU uart")
}

// recvIteration receives a block of data from one specific UART, verifying that it was as expected.
func recvIteration(ctx context.Context, s *testing.State, f *fixture.Value, th utils.FirmwareTestingHelper, ccd ti50.SerialChannel, magic byte, iteration int) {
	databuf, err := ccd.ReadSerialBytes(ctx, uartThroughputBlockSize)
	th.MustSucceed(err, "Read error")

	// All validation errors reported as "Fatal", in order to avoid thousands of lines of
	// error messages, as all future data would fail validation in case of a dropped sequence.
	if databuf[0] != byte(iteration) ||
		databuf[1] != byte(iteration>>8) ||
		databuf[2] != byte(iteration>>16) ||
		databuf[3] != byte(iteration>>24) {
		s.Fatal("Incorrect sequence number")
	}
	if databuf[4] != magic {
		s.Fatal("Incorrect magic")
	}
	var idx = 5
	for idx < uartThroughputBlockSize {
		if databuf[idx] != byte(idx) {
			s.Fatal("Incorrect data contents")
		}
		idx = idx + 1
	}
}

// sendIteration sends a block of data on one specific UART.
func sendIteration(ctx context.Context, s *testing.State, f *fixture.Value, th utils.FirmwareTestingHelper, uart ti50.SerialChannel, magic byte, iteration int) {
	databuf := make([]byte, uartThroughputBlockSize)
	var idx = 0
	for idx < uartThroughputBlockSize {
		databuf[idx] = byte(idx)
		idx = idx + 1
	}
	databuf[0] = byte(iteration)
	databuf[1] = byte(iteration >> 8)
	databuf[2] = byte(iteration >> 16)
	databuf[3] = byte(iteration >> 24)

	databuf[4] = magic
	th.MustSucceed(uart.WriteSerial(ctx, databuf), "Write error")
}
