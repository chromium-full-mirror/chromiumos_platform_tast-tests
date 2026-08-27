// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
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
		Func:    GSCUARTForward,
		Desc:    "Tests forwarding between GSC UARTs and USB",
		Timeout: 90 * time.Second,
		Contacts: []string{
			"cros-hwsec@google.com", // CrOS GSC Developers
			"jbk@chromium.org",      // Test Author
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
	b := utils.NewDevboardHelper(s, s.FixtValue().(*fixture.Value))
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}
	i := ti50.MustOpenCrOSImage(ctx, b, s, b.TestbedType)
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
	b.ResetWithStraps(ctx, ti50.CCDModeOff, ti50.ServoMicroDisconnected)
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC revives after reboot")

	if b.GpioGet(ctx, ti50.GpioTi50UartDbgTxEcRx) {
		s.Error("GSC driving EC UART high before CCD connection")
	}

	b.GpioApplyStrap(ctx, ti50.CCDModeOn)
	// Wait until CCD USB shows up and CCD UART TX is enabled.
	b.WaitUntilCCDConnectedAndUARTTXEnabled(ctx)

	// Test forwarding on each of three ports.
	s.Log("AP off, no uServo")
	testForwardingUARTs(ctx, s, b, r, true, true, false, "AP off, no uServo")

	// Simulate the AP processor being turned on, in order to enable AP forwarding.
	b.GpioSet(ctx, ti50.GpioTi50PltRstL, true)

	// Test forwarding on each of three ports.
	s.Log("AP on, no uServo")
	testForwardingUARTs(ctx, s, b, r, true, true, true, "AP on, no uServo")

	b.GpioApplyStrap(ctx, ti50.CCDModeOff)

	startTime := time.Now()
	for b.GpioGet(ctx, ti50.GpioTi50UartDbgTxEcRx) {
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
	b.GpioApplyStrap(ctx, ti50.CCDModeOn)
	b.WaitUntilCCDConnectedAndUARTTXEnabled(ctx)

	// Test forwarding on each of three ports.
	s.Log("AP off, with uServo")
	testForwardingUARTs(ctx, s, b, r, true, false, false, "AP off, with uServo")

	// Simulate the AP processor being turned on, in order to enable AP forwarding.
	b.GpioSet(ctx, ti50.GpioTi50PltRstL, true)

	// Test forwarding on each of three ports.
	s.Log("AP on, with uServo")
	testForwardingUARTs(ctx, s, b, r, true, false, true, "AP on, with uServo")
}
func testForwardingUARTs(ctx context.Context, s *testing.State, b utils.DevboardHelper, r *rand.Rand, expectUartToUsb, expectUsbToUart, apOn bool, caseStr string) {
	gscProps := b.GscProperties()

	if err := b.TestUARTForwarding(ctx, r, ti50.UartEC, expectUartToUsb, expectUsbToUart, caseStr); err != nil {
		s.Errorf("EC UART failed: %s", err)
	}
	apExpectUartToUsb := expectUartToUsb && apOn
	apExpectUsbToUart := expectUsbToUart && apOn
	if err := b.TestUARTForwarding(ctx, r, ti50.UartAP, apExpectUartToUsb, apExpectUsbToUart, caseStr); err != nil {
		s.Errorf("AP UART failed: %s", err)
	}

	if !gscProps.HasFpmcuUart() {
		return
	}
	if err := b.TestUARTForwarding(ctx, r, ti50.UartFPMCU, expectUartToUsb, expectUsbToUart, caseStr); err != nil {
		s.Errorf("FPMCU UART failed: %s", err)
	}
}
