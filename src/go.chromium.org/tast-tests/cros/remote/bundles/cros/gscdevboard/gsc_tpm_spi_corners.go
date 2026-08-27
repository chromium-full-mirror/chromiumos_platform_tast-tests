// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/utils"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:    GSCTPMSPICorners,
		Desc:    "Test TPM SPI corner cases",
		Timeout: 5 * time.Minute,
		Contacts: []string{
			"cros-hwsec@google.com", // CrOS GSC Developers
			"jbk@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Attr: []string{
			"group:gsc",
			"gsc_dt_ab", "gsc_dt_shield", "gsc_h1_shield", "gsc_ot_shield", "gsc_ot_fpga_cw310",
			"gsc_image_ti50",
			"gsc_nightly"},
		Fixture: fixture.GSCOpenCCD,
	})
}

func verifyReadyPulses(events utils.GpioEvents, s *testing.State) {
	for i, event := range events.Sorted {
		if !event.IsFallOf(ti50.GpioTi50ApIntL) {
			continue
		}
		// Leading edge of a ready pulse, verify that this happened
		// in connection with chip select being released.
		if i > 0 {
			prevEvent := events.Sorted[i-1]
			if prevEvent.IsRiseOf(ti50.Ti50SpiTpmCs) {
				// Accept ready pulse immediately following release of chip
				// select.
				latency := event.TimestampUS - prevEvent.TimestampUS
				s.Logf("Ready latency: %d us", latency)
				continue
			} else if prevEvent.IsRiseOf(ti50.Ti50SpiTpmMiso) {
				// The SPI device will stop driving its data output when chip
				// select is released, so we may see a rising edge of the data out
				// between chip select deassertion and ready pulse, which is also
				// accepted.
				prev2Event := events.Sorted[i-2]
				if prev2Event.IsRiseOf(ti50.Ti50SpiTpmCs) {
					latency := event.TimestampUS - prev2Event.TimestampUS
					s.Logf("Ready latency: %d us", latency)
					continue
				}
			}
		}
		if i+1 < len(events.Sorted) {
			nextEvent := events.Sorted[i+1]
			if nextEvent.IsRiseOf(ti50.Ti50SpiTpmCs) {
				// On write transactions, the SPI device may issue ready pulse as
				// soon as it has received all the expected data bytes, which
				// should be followed by chip select being deasserted.  Accept as
				// long as there are no further clock edges or other activity
				// between ready pulse and CS deassertion.
				latency := event.TimestampUS - nextEvent.TimestampUS
				s.Logf("Ready latency: %d us", latency)
				continue
			}
		}
		str := ""
		for j := i - 3; j <= i+3 && j < len(events.Sorted); j++ {
			cur := float64(events.Sorted[j].TimestampUS) / 1000
			str = str + fmt.Sprintf("\n\t%-15s\t%s\t%.3fms", events.Sorted[j].Name, events.Sorted[j].Edge, cur)
		}
		s.Errorf("Unexpected ready pulse%s", str)
	}
}

func GSCTPMSPICorners(ctx context.Context, s *testing.State) {
	b := utils.NewDevboardHelper(s, s.FixtValue().(*fixture.Value))
	i := ti50.MustOpenCrOSImage(ctx, b, s, b.TestbedType)
	defer i.Close(ctx)

	tpmHandle := b.ResetAndTpmStartupForBus(ctx, i, ti50.TpmBusSpi, ti50.CCDModeOff, ti50.FfClamshell)

	// Record everything that is transmitted by CLK/CS/MISO/MOSI lines, for manual inspection later.
	gpioMonitor := b.GpioMonitorStart(
		ctx,
		ti50.GpioTi50ApIntL,
		ti50.Ti50SpiTpmCs,
		ti50.Ti50SpiTpmSck,
		ti50.Ti50SpiTpmMosi,
		ti50.Ti50SpiTpmMiso)
	// Store transcript of CLK/CS/MISO/MOSI events in .vcd format, to be reviewed in e.g. Pulseview.
	defer func() {
		events := b.GpioMonitorFinish(ctx, gpioMonitor)
		gpioMonitor.Save(ctx, events, "tpm_spi_corners.vcd")
		verifyReadyPulses(events, s)
	}()

	// Perform irregular SPI TPM transaction, ask for content of status register, but never
	// read the bytes.
	_, err := b.OpenTitanToolCommand(ctx, "spi", "--bus", "TPM", "raw-write", "--hexdata", "C3D4001800")
	if err != nil {
		s.Fatal("spi error: ", err)
	}

	// Now read DIDVID register again.  This should cause previously enqueued status register
	// data to be discarded from the Dauntless SPI fifo.
	didVid := tpmHandle.ReadRegister(ti50.TpmRegDidVid)
	expectedDidVidValue := b.GscProperties().ExpectedDidVidValue()
	if !bytes.Equal(didVid, expectedDidVidValue) {
		s.Error("Unexpected TPM DID_VID after partial SPI transaction: ", didVid)
	}

	// Perform irregular SPI TPM transaction, truncated.
	_, err = b.OpenTitanToolCommand(ctx, "spi", "--bus", "TPM", "raw-write", "--hexdata", "C3D4")
	if err != nil {
		s.Fatal("spi error: ", err)
	}

	// Perform irregular SPI TPM transaction, read more bytes than the size of the register.
	_, err = b.OpenTitanToolCommand(ctx, "spi", "--bus", "TPM", "raw-write-read", "--hexdata", "C3D4001800", "--length", "16")
	if err != nil {
		s.Fatal("spi error: ", err)
	}

	// Check one final time that that GSC still respondes to DIDVID register, to make sure it
	// has not crashed or got the SPI driver into a funny state.
	didVid = tpmHandle.ReadRegister(ti50.TpmRegDidVid)
	if !bytes.Equal(didVid, expectedDidVidValue) {
		s.Error("Unexpected TPM DID_VID: ", didVid)
	}

}
