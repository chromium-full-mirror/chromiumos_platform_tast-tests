// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"bytes"
	"context"
	"fmt"
	"math/rand"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/gscdevboard/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:    GSCUARTForward,
		Desc:    "Tests forwarding between GSC UARTs and USB",
		Timeout: 90 * time.Second,
		Contacts: []string{
			"chromeos-faft@google.com", // CrOS Firmware Developers
			"jbk@chromium.org",         // Test Author
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr: []string{"group:gsc",
			"gsc_dt_ab", "gsc_dt_shield", "gsc_h1_shield", "gsc_ot_fpga_cw310", "gsc_ot_shield",
			"gsc_image_ti50",
			"gsc_nightly"},
		Fixture: fixture.GSCOpenCCD,
	})
}

func GSCUARTForward(ctx context.Context, s *testing.State) {
	b := utils.NewDevboardHelper(s)
	gscProps := b.GscProperties()
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}
	i := ti50.MustOpenCrOSImage(ctx, b, s)
	defer i.Close(ctx)

	seed := time.Now().UnixNano()
	s.Logf("Random seed: %d", seed)
	r := rand.New(rand.NewSource(seed))

	//
	// Fixture has already done "ccd open", now boot GSC simulating CCD without uServo,
	// verify that forwarding works both ways.
	//
	// Simulate the AP processor being off initially.
	s.Log("(Re)starting ti50")
	b.GpioSet(ctx, ti50.GpioTi50PltRstL, false)
	b.ResetWithStraps(ctx, ti50.CcdDisconnected, ti50.ServoMicroDisconnected)
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")

	if b.GpioGet(ctx, ti50.GpioTi50UartDbgTxEcRx) != false {
		s.Error("GSC driving EC UART high before CCD connection")
	}

	b.GpioApplyStrap(ctx, ti50.CcdSuzyQ)
	b.WaitUntilCCDConnected(ctx)

	// Test forwarding on each of three ports.
	s.Log("AP off, no uServo")
	testForwarding(ctx, s, b, th, r, ti50.UartEC, true, true, "AP off, no uServo")
	testForwarding(ctx, s, b, th, r, ti50.UartAP, false, false, "AP off, no uServo")
	if gscProps.HasFpmcuUart() {
		testForwarding(ctx, s, b, th, r, ti50.UartFPMCU, true, true, "AP off, no uServo")
	}

	// Simulate the AP processor being turned on, in order to enable AP forwarding.
	b.GpioSet(ctx, ti50.GpioTi50PltRstL, true)

	// Test forwarding on each of three ports.
	s.Log("AP on, no uServo")
	testForwarding(ctx, s, b, th, r, ti50.UartEC, true, true, "AP on, no uServo")
	testForwarding(ctx, s, b, th, r, ti50.UartAP, true, true, "AP on, no uServo")
	if gscProps.HasFpmcuUart() {
		testForwarding(ctx, s, b, th, r, ti50.UartFPMCU, true, true, "AP on, no uServo")
	}

	b.GpioApplyStrap(ctx, ti50.CcdDisconnected)

	startTime := time.Now()
	for b.GpioGet(ctx, ti50.GpioTi50UartDbgTxEcRx) == true {
		if time.Since(startTime) > 5*time.Second {
			s.Error("GSC driving EC UART high after CCD disconnection")
			break
		}
	}

	//
	// Boot GSC simulating a uServo being connected simultaneously with CCD.  Verify that
	// data goes from UART to USB, but that USB data is not forwarded to UART (would conflict
	// with uServo).
	//
	// Simulate the AP processor being off initially.
	b.GpioSet(ctx, ti50.GpioTi50PltRstL, false)
	b.ResetWithStraps(ctx, ti50.ServoMicroConnected)
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")
	b.GpioApplyStrap(ctx, ti50.CcdSuzyQ)
	b.WaitUntilCCDConnected(ctx)

	// Test forwarding on each of three ports.
	s.Log("AP off, with uServo")
	testForwarding(ctx, s, b, th, r, ti50.UartEC, true, false, "AP off, with uServo")
	testForwarding(ctx, s, b, th, r, ti50.UartAP, false, false, "AP off, with uServo")
	if gscProps.HasFpmcuUart() {
		testForwarding(ctx, s, b, th, r, ti50.UartFPMCU, true, false, "AP off, with uServo")
	}

	// Simulate the AP processor being turned on, in order to enable AP forwarding.
	b.GpioSet(ctx, ti50.GpioTi50PltRstL, true)

	// Test forwarding on each of three ports.
	s.Log("AP on, with uServo")
	testForwarding(ctx, s, b, th, r, ti50.UartEC, true, false, "AP on, with uServo")
	testForwarding(ctx, s, b, th, r, ti50.UartAP, true, false, "AP on, with uServo")
	if gscProps.HasFpmcuUart() {
		testForwarding(ctx, s, b, th, r, ti50.UartFPMCU, true, false, "AP on, with uServo")
	}
}

func testForwarding(ctx context.Context, s *testing.State, b utils.DevboardHelper, th utils.FirmwareTestingHelper, r *rand.Rand, port ti50.UartName, expectUartToUsb, expectUsbToUart bool, caseStr string) {
	uart := b.PhysicalUart(port)
	ccd := b.CcdSerialInterface(port, time.Second)
	th.MustSucceed(ccd.Open(ctx), "Failed to open %s ccd", port)
	defer ccd.Close(ctx)
	th.MustSucceed(uart.Open(ctx), "Failed to open %s uart", port)
	defer uart.Close(ctx)

	// Flush out any "DATA LOST" message along with other queued-up data.
	if expectUartToUsb {
		uart.WriteSerial(ctx, []byte("AB\r\n"))
		_, _, err := ccd.ReadSerialSubmatch(ctx, regexp.MustCompile(`AB\r\n`))
		th.MustSucceed(err, "Error clearing buffer")
	}

	// Send data to UART, expecting to read it out of the USB interface.
	databuf := []byte(fmt.Sprintf("The quick red fox jumps over the lazy brown dog for the %dth time", r.Intn(1000000000)))

	th.MustSucceed(uart.WriteSerial(ctx, databuf), "Write error")
	byt, err := ccd.ReadSerialBytes(ctx, len(databuf))
	if expectUartToUsb {
		if err != nil {
			s.Errorf("%s: Data sent to %s UART did not come out of USB: %s", caseStr, port, err)
		} else if !bytes.Equal(byt, databuf) {
			s.Errorf("%s: Data sent to %s UART came out of USB corrupted", caseStr, port)
			s.Errorf("Wanted '%+v' got '%+v'", databuf, byt)
		}
	} else {
		if err == nil {
			if !bytes.Equal(byt, databuf) {
				s.Errorf("%s: Data sent to %s UART unexpectedly did come out of USB: corrupted", caseStr, port)
				s.Errorf("Sent '%+v' got '%+v'", databuf, byt)
			} else {
				s.Errorf("%s: Data sent to %s UART unexpectedly did come out of USB", caseStr, port)
			}
		}
	}

	// Send data to USB interface, expecting to read it out of the UART.
	th.MustSucceed(uart.ClearInput(ctx), "Error clearing buffer")

	databuf = []byte(fmt.Sprintf("The quick red fox jumps over the lazy brown dog for the %dth time", r.Intn(1000000000)))

	th.MustSucceed(ccd.WriteSerial(ctx, databuf), "Write error")
	byt, err = uart.ReadSerialBytes(ctx, len(databuf))
	if expectUsbToUart {
		if err != nil {
			s.Errorf("%s: Data sent to %s USB did not come out of UART: %s", caseStr, port, err)
		} else if !bytes.Equal(byt, databuf) {
			s.Errorf("%s: Data sent to %s USB came out of UART corrupted", caseStr, port)
			s.Errorf("Wanted '%+v' got '%+v'", databuf, byt)
		}
	} else {
		if err == nil {
			if !bytes.Equal(byt, databuf) {
				s.Errorf("%s: Data sent to %s USB unexpectedly did come out of UART: corrupted", caseStr, port)
				s.Errorf("Sent '%+v' got '%+v'", databuf, byt)
			} else {
				s.Errorf("%s: Data sent to %s USB unexpectedly did come out of UART", caseStr, port)
			}
		}
	}
}
