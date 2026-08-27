// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/utils"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:    GSCECReset,
		Desc:    "Test workaround for EC double reset",
		Timeout: 30 * time.Second,
		Contacts: []string{
			"cros-hwsec@google.com", // CrOS GSC Developers
			"jbk@chromium.org",      // Test Author
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr: []string{"group:gsc",
			"gsc_dt_ab", "gsc_dt_shield", "gsc_h1_shield", "gsc_ot_shield", "gsc_ot_fpga_cw310",
			"gsc_image_ti50",
			"gsc_nightly"},
		Fixture: fixture.GSCOpenCCD,
		Params: []testing.Param{{
			Name: "gsc_reset_gpio",
			Val:  verifyEcResetOnGscReset,
		}, {
			Name: "gsc_reset_tpmv",
			Val:  verifyEcResetOnTpmvRebootCmd,
		}, {
			Name: "gsc_reset_console",
			Val:  verifyEcResetOnConsoleRebootCmd,
		}},
	})
}

func GSCECReset(ctx context.Context, s *testing.State) {
	subTest := s.Param().(func(context.Context, *testing.State, utils.DevboardHelper, *ti50.CrOSImage, utils.FirmwareTestingHelper))
	b := utils.NewDevboardHelper(s, s.FixtValue().(*fixture.Value))
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}
	i := ti50.MustOpenCrOSImage(ctx, b, s, b.TestbedType)
	defer i.Close(ctx)

	b.Reset(ctx)
	th.MustSucceed(i.WaitUntilBooted(ctx), "Ti50 revives after reboot")

	subTest(ctx, s, b, i, th)
}

func verifyEcResetOnGscReset(ctx context.Context, s *testing.State, b utils.DevboardHelper, i *ti50.CrOSImage, th utils.FirmwareTestingHelper) {
	hasEcReset := b.GscProperties().HasEcRstFet()
	s.Log("Verify EC reset on GSC_RST_L toggle")
	// Hold GSC in reset before start GPIO monitoring
	b.GpioSet(ctx, ti50.GpioTi50ResetL, false)

	s.Log("Start gpio monitoring")
	gpios := []ti50.GpioName{ti50.GpioTi50ResetL, ti50.GpioTi50EcRstL}
	if hasEcReset {
		gpios = append(gpios, ti50.GpioTi50EcRstFet)
	}
	gpioMonitor := b.GpioMonitorStart(ctx, gpios...)

	s.Log("Booting ti50")
	b.GpioSet(ctx, ti50.GpioTi50ResetL, true)
	th.MustSucceed(i.WaitUntilBooted(ctx), "Ti50 revives after reboot")

	// Give a little more time for gpio monitoring to catch EC_RST edges after GSC boots
	testing.Sleep(ctx, time.Second) // GoBigSleepLint: No good way to poll for EC_RST

	events := b.GpioMonitorFinish(ctx, gpioMonitor)
	s.Log("Stop gpio monitoring: ", events)

	resetReleased := events.FindFirst(ti50.GpioTi50ResetL, utils.GpioEdgeRising)
	if resetReleased == nil {
		// Must be fatal so we don't dereference nil below
		s.Fatal("GSC did not come out of reset")
	}

	// Depending on if GSC has a secondary EC RESET FET pin, we need to find
	// the real EC RESET edge after it or not
	var ecResetAfter *utils.GpioEvent
	if hasEcReset {
		firstFetAfterRelease := events.FindFirstAfter(*resetReleased, ti50.GpioTi50EcRstFet)
		if firstFetAfterRelease == nil {
			// Must be fatal so we don't dereference nil below
			s.Fatalf("%s did have an edge after release GSC from reset", ti50.GpioTi50EcRstFet)
		}

		ecResetAfter = firstFetAfterRelease
	} else {
		ecResetAfter = resetReleased
	}

	// Find the real EC RESET edge
	realEcRelease := events.FindFirstAfter(*ecResetAfter, ti50.GpioTi50EcRstL)
	if realEcRelease == nil || realEcRelease.Edge != utils.GpioEdgeRising {
		// Must be fatal so we don't dereference nil below
		s.Fatalf("%s edge right after FET release is not rising edge", ti50.GpioTi50EcRstL)
	}

	s.Logf("EC released from Reset %dms after GSC released", (realEcRelease.TimestampUS-resetReleased.TimestampUS)/1000)
}

func verifyEcResetOnTpmvRebootCmd(ctx context.Context, s *testing.State, b utils.DevboardHelper, i *ti50.CrOSImage, th utils.FirmwareTestingHelper) {
	s.Log("Verify EC reset on GSC reboot TPMV command")

	tpm := b.ResetAndTpmStartup(ctx, i, ti50.FfClamshell)

	s.Log("Start gpio monitoring")
	gpioMonitor := b.GpioMonitorStart(ctx, ti50.GpioTi50EcRstL)

	err := tpm.TpmvReboot(1000)
	th.MustSucceed(err, "Error sending TPMV reboot")

	th.MustSucceed(i.WaitUntilBooted(ctx), "Ti50 revives after reboot")

	// Give a little more time for gpio monitoring to catch EC_RST edges after GSC boots
	testing.Sleep(ctx, 3*time.Second) // GoBigSleepLint: No good way to poll for EC_RST

	events := b.GpioMonitorFinish(ctx, gpioMonitor)
	s.Log("Stop gpio monitoring: ", events)

	ecReset := events.FindFirst(ti50.GpioTi50EcRstL, utils.GpioEdgeFalling)
	if ecReset == nil {
		// Must be fatal so we don't dereference nil below
		s.Fatal("EC not put in reset with GSC reboot TPMV command")
	}
	//lint:ignore SA5011 ecReset is actually checked right above
	ecResetReleased := events.FindFirstAfter(*ecReset, ti50.GpioTi50EcRstL)
	if ecResetReleased == nil {
		s.Error("EC not released from reset after GSC reboot TPMV command")
	}
}

func verifyEcResetOnConsoleRebootCmd(ctx context.Context, s *testing.State, b utils.DevboardHelper, i *ti50.CrOSImage, th utils.FirmwareTestingHelper) {
	s.Log("Verify EC reset on GSC reboot console command")

	s.Log("Start gpio monitoring")
	gpioMonitor := b.GpioMonitorStart(ctx, ti50.GpioTi50EcRstL)

	th.MustSucceed(i.SendConsoleRebootCmd(ctx), "Error calling `reboot` gsctool console command")
	th.MustSucceed(i.WaitUntilBooted(ctx), "Ti50 revives after reboot")

	// Give a little more time for gpio monitoring to catch EC_RST edges after GSC boots
	testing.Sleep(ctx, time.Second) // GoBigSleepLint: No good way to poll for EC_RST

	events := b.GpioMonitorFinish(ctx, gpioMonitor)
	s.Log("Stop gpio monitoring: ", events)

	ecReset := events.FindFirst(ti50.GpioTi50EcRstL, utils.GpioEdgeFalling)
	if ecReset == nil {
		// Must be fatal so we don't dereference nil below
		s.Fatal("EC not put in reset with GSC reboot console command")
	}
	//lint:ignore SA5011 ecReset is actually checked right above
	ecResetReleased := events.FindFirstAfter(*ecReset, ti50.GpioTi50EcRstL)
	if ecResetReleased == nil {
		s.Error("EC not released from reset after GSC reboot console command")
	}
}
