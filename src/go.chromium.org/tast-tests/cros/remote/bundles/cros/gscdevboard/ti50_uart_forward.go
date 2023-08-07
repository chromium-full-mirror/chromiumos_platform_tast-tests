// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"bytes"
	"context"
	"fmt"
	"math/rand"
	"time"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/gscdevboard/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"

	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:    Ti50UartForward,
		Desc:    "Tests forwarding between GSC UARTs and USB",
		Timeout: 90 * time.Second,
		Contacts: []string{
			"chromeos-faft@google.com", // CrOS Firmware Developers
			"jbk@chromium.org",         // Test Author
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr:         []string{"group:gsc", "gsc_dt_ab", "gsc_dt_shield", "gsc_ot_fpga_cw310", "gsc_image_ti50"},
		Fixture:      fixture.Ti50CcdOpen,
	})
}

func Ti50UartForward(ctx context.Context, s *testing.State) {
	f := s.FixtValue().(*fixture.Value)
	b := utils.NewDevboardHelper(f.DevBoard(), s)
	i := ti50.NewCrOSImage(b)

	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}

	seed := time.Now().UnixNano()
	s.Logf("Random seed: %d", seed)
	r := rand.New(rand.NewSource(seed))

	s.Log("(Re)starting ti50")
	b.GpioApplyStrap(ctx, ti50.CcdSuzyQ)

	//
	// Fixture has already done "ccd open", now boot Ti50 simulating CCD without uServo,
	// verify that forwarding works both ways.
	//
	b.GpioApplyStrap(ctx, ti50.ServoMicroDisconnected)
	// Simulate the AP processor being off initially.
	b.GpioSet(ctx, ti50.GpioTi50PltRstL, false)
	th.MustSucceed(b.Reset(ctx), "Reset board")
	th.MustSucceed(i.WaitUntilBooted(ctx), "Ti50 revives after reboot")

	// Test forwarding on each of three ports.
	testForwarding(ctx, s, f, th, r, ti50.UartEC, true, true)
	testForwarding(ctx, s, f, th, r, ti50.UartAP, false, false)
	testForwarding(ctx, s, f, th, r, ti50.UartFPMCU, true, true)

	// Simulate the AP processor being turned on, in order to enable AP forwarding.
	b.GpioSet(ctx, ti50.GpioTi50PltRstL, true)

	// Test forwarding on each of three ports.
	testForwarding(ctx, s, f, th, r, ti50.UartEC, true, true)
	testForwarding(ctx, s, f, th, r, ti50.UartAP, true, true)
	testForwarding(ctx, s, f, th, r, ti50.UartFPMCU, true, true)

	//
	// Boot Ti50 simulating a uServo being connected simultaneously with CCD.  Verify that
	// data goes from UART to USB, but that USB data is not forwarded to UART (would conflict
	// with uServo).
	//
	b.GpioApplyStrap(ctx, ti50.ServoMicroConnected)
	// Simulate the AP processor being off initially.
	b.GpioSet(ctx, ti50.GpioTi50PltRstL, false)
	th.MustSucceed(b.Reset(ctx), "Reset board")
	th.MustSucceed(i.WaitUntilBooted(ctx), "Ti50 revives after reboot")

	// Test forwarding on each of three ports.
	testForwarding(ctx, s, f, th, r, ti50.UartEC, true, false)
	testForwarding(ctx, s, f, th, r, ti50.UartAP, false, false)
	testForwarding(ctx, s, f, th, r, ti50.UartFPMCU, true, false)

	// Simulate the AP processor being turned on, in order to enable AP forwarding.
	b.GpioSet(ctx, ti50.GpioTi50PltRstL, true)

	// Test forwarding on each of three ports.
	testForwarding(ctx, s, f, th, r, ti50.UartEC, true, false)
	testForwarding(ctx, s, f, th, r, ti50.UartAP, true, false)
	testForwarding(ctx, s, f, th, r, ti50.UartFPMCU, true, false)
}

func testForwarding(ctx context.Context, s *testing.State, f *fixture.Value, th utils.FirmwareTestingHelper, r *rand.Rand, port ti50.UartName, expectUartToUsb, expectUsbToUart bool) {
	uart := f.DevBoard().PhysicalUart(port, time.Second)
	ccd := f.DevBoard().CcdSerialInterface(port, time.Second)
	th.MustSucceed(ccd.Open(ctx), "Failed to open %s ccd", port)
	th.MustSucceed(uart.Open(ctx), "Failed to open %s uart", port)

	// Flush out any "DATA LOST" message along with other queued-up data.
	uart.WriteSerial(ctx, []byte{13, 10})
	th.MustSucceed(ccd.ClearInput(ctx), "Error clearing buffer")

	// Send data to UART, expecting to read it out of the USB interface.
	databuf := []byte(fmt.Sprintf("The quick red fox jumps over the lazy brown dog for the %dth time", r.Intn(1000000000)))

	th.MustSucceed(uart.WriteSerial(ctx, databuf), "Write error")
	byt, err := ccd.ReadSerialBytes(ctx, len(databuf))
	if expectUartToUsb {
		if err != nil {
			s.Errorf("Data sent to %s UART did not come out of USB: %s", port, err)
		} else if !bytes.Equal(byt, databuf) {
			s.Errorf("Data sent to %s UART came out of USB corrupted", port)
			s.Errorf("Wanted '%+v' got '%+v'", databuf, byt)
		}
	} else {
		if err == nil {
			if !bytes.Equal(byt, databuf) {
				s.Errorf("Data sent to %s UART unexpectedly did come out of USB: corrupted", port)
				s.Errorf("Sent '%+v' got '%+v'", databuf, byt)
			} else {
				s.Errorf("Data sent to %s UART unexpectedly did come out of USB", port)
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
			s.Errorf("Data sent to %s USB did not come out of UART: %s", port, err)
		} else if !bytes.Equal(byt, databuf) {
			s.Errorf("Data sent to %s USB came out of UART corrupted", port)
			s.Errorf("Wanted '%+v' got '%+v'", databuf, byt)
		}
	} else {
		if err == nil {
			if !bytes.Equal(byt, databuf) {
				s.Errorf("Data sent to %s USB unexpectedly did come out of UART: corrupted", port)
				s.Errorf("Sent '%+v' got '%+v'", databuf, byt)
			} else {
				s.Errorf("Data sent to %s USB unexpectedly did come out of UART", port)
			}
		}
	}

	th.MustSucceed(ccd.Close(ctx), "Failed to close %s ccd", port)
	th.MustSucceed(uart.Close(ctx), "Failed to close %s uart", port)
}
