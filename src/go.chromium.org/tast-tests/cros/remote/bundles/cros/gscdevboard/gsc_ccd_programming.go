// Copyright 2024 The ChromiumOS Authors
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
		Func:    GSCCCDProgramming,
		Desc:    "Verify GSC's ability to program other ICs over CCD",
		Timeout: 30 * time.Second,
		Contacts: []string{
			"cros-hwsec@google.com", // CrOS GSC Developers
			"jettrink@google.com",   // Test Author
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr: []string{"group:gsc",
			"gsc_h1_shield", "gsc_dt_shield", "gsc_ot_shield", "gsc_ot_fpga_cw310",
			"gsc_image_ti50",
			"gsc_nightly"},
		Fixture: fixture.GSCOpenCCD,
	})
}

func GSCCCDProgramming(ctx context.Context, s *testing.State) {
	b := utils.NewDevboardHelper(s, s.FixtValue().(*fixture.Value))
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}
	i := ti50.MustOpenCrOSImage(ctx, b, s, b.TestbedType)
	defer i.Close(ctx)

	f := s.FixtValue().(*fixture.Value)

	// Enable CCD
	b.ResetWithStraps(ctx, ti50.CCDModeOn)
	th.MustSucceed(i.WaitUntilBooted(ctx), "Ti50 revives after reboot")
	b.WaitUntilCCDConnected(ctx)

	s.Log("Verify UART_EC_TX_GSC_RX functionality")

	// Convert the EC_UART_TX_GSC_RX pin to an input on hyperdebug instead of the
	// normal UART alternate function.
	b.GpioMultiSet(ctx, ti50.GpioTi50UartEcTxDbgRx, true, utils.GpioModeInput, utils.GpioPullUp)
	if !b.GpioGet(ctx, ti50.GpioTi50UartEcTxDbgRx) {
		s.Fatal("UART RX not high be default")
	}

	// H1 doesn't block EC_TX_CR50_RX_OUT toggle at any time
	if f.TestbedProperties.TestbedType == ti50.GscH1Shield {
		s.Log("Skipping verification while EC not in reset for h1")
	} else {
		_, err := i.Command(ctx, "gpioset EC_TX_CR50_RX_OUT 0")
		th.MustSucceed(err, "Call gpioset EC_TX_CR50_RX_OUT 0")
		if !b.GpioGet(ctx, ti50.GpioTi50UartEcTxDbgRx) {
			s.Error("UART RX should not change if ecrst isn't being held")
		}
	}

	th.MustSucceed(i.EcrstOn(ctx), "Hold EC in reset")

	_, err := i.Command(ctx, "gpioset EC_TX_CR50_RX_OUT 0")
	th.MustSucceed(err, "Call gpioset EC_TX_CR50_RX_OUT 0")
	if b.GpioGet(ctx, ti50.GpioTi50UartEcTxDbgRx) {
		s.Error("UART RX not set to low when EC held in reset")
	}

	th.MustSucceed(i.EcrstOff(ctx), "Release EC from reset")

	if b.GpioGet(ctx, ti50.GpioTi50UartEcTxDbgRx) {
		s.Error("UART RX did not maintain low value after EC released")
	}

	_, err = i.Command(ctx, "gpioset EC_TX_CR50_RX_OUT 1")
	th.MustSucceed(err, "Call gpioset EC_TX_CR50_RX_OUT 1")
	if !b.GpioGet(ctx, ti50.GpioTi50UartEcTxDbgRx) {
		s.Error("UART RX not released back to high")
	}

	// The swizzle board does not route through the following signals:
	// B7: GSC_EC_SPI_SEL (tested below)
	// B8: EN_I2C_DBG_PWR_L
	// B11, B12: I2C_GSC_DBG_SDA/SCL
	// C7: Unused by Ti50
	// C9: Unused by Ti50
	if b.TestbedType != ti50.GscOpentitanCw310Fpga {
		s.Log("Verify EC_FLASH_SELECT functionality")

		_, err = i.Command(ctx, "gpioset EC_FLASH_SELECT 0")
		th.MustSucceed(err, "Call gpioset EC_FLASH_SELECT 0")
		if b.GpioGet(ctx, ti50.GpioTi50ECFlashSelect) {
			s.Error("EC flash select gpio did not go low")
		}

		_, err = i.Command(ctx, "gpioset EC_FLASH_SELECT 1")
		th.MustSucceed(err, "Call gpioset EC_FLASH_SELECT 1")
		if !b.GpioGet(ctx, ti50.GpioTi50ECFlashSelect) {
			s.Error("EC flash select gpio did not go high")
		}
	}
}
