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
		Func:    Ti50ECReset,
		Desc:    "Test workaround for EC double reset",
		Timeout: 30 * time.Second,
		Contacts: []string{
			"chromeos-faft@google.com", // CrOS Firmware Developers
			"jbk@chromium.org",         // Test Author
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr:         []string{"group:gsc", "gsc_dt_ab", "gsc_dt_shield", "gsc_image_ti50", "gsc_nightly"},
		Fixture:      fixture.GSCOpenCCD,
		Params: []testing.Param{{
			Name: "gsc_reset_gpio",
			Val:  verifyEcRestOnGscReset,
		}, {
			Name: "gsc_reset_tpmv",
			Val:  verifyEcResetOnTpmvRebootCmd,
		}, {
			Name: "gsc_reset_console",
			Val:  verifyEcResetOnConsoleRebootCmd,
		}},
	})
}

func Ti50ECReset(ctx context.Context, s *testing.State) {
	subTest := s.Param().(func(context.Context, *testing.State, utils.DevboardHelper, *ti50.CrOSImage, utils.FirmwareTestingHelper))
	b := utils.NewDevboardHelper(s)
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}
	i := ti50.MustOpenCrOSImage(ctx, b, s)
	defer i.Close(ctx)

	b.Reset(ctx)
	th.MustSucceed(i.WaitUntilBooted(ctx), "Ti50 revives after reboot")

	subTest(ctx, s, b, i, th)
}

func verifyEcRestOnGscReset(ctx context.Context, s *testing.State, b utils.DevboardHelper, i *ti50.CrOSImage, th utils.FirmwareTestingHelper) {
	s.Log("Verify EC reset on GSC_RST_L toggle")
	// Hold GSC in reset before start GPIO monitoring
	b.GpioSet(ctx, ti50.GpioTi50ResetL, false)

	s.Log("Start gpio monitoring")
	gpioMonitor := b.GpioMonitorStart(ctx, ti50.GpioTi50ResetL, ti50.GpioTi50EcRstL, ti50.GpioTi50EcRstFet)

	s.Log("Booting ti50")
	b.GpioSet(ctx, ti50.GpioTi50ResetL, true)
	th.MustSucceed(i.WaitUntilBooted(ctx), "Ti50 revives after reboot")

	// Give a little more time for gpio monitoring to catch EC_RST edges after GSC boots
	testing.Sleep(ctx, time.Second) // GoBigSleepLint: No good way to poll for EC_RST

	events := b.GpioMonitorFinish(ctx, gpioMonitor)
	s.Log("Stop gpio monitoring: ", events)

	resetReleased := events.FindFirst(ti50.GpioTi50ResetL, utils.GpioEdgeRising)
	if resetReleased == nil {
		s.Error("GSC did not come out of reset")
		// Must return so we don't dereference null below
		return
	}
	firstFetAfterRelease := events.FindFirstAfter(*resetReleased, ti50.GpioTi50EcRstFet)
	if firstFetAfterRelease == nil {
		s.Errorf("%s did have an edge after release GSC from reset", ti50.GpioTi50EcRstFet)
		// Must return so we don't dereference null below
		return
	}

	firstEcAfterFet := events.FindFirstAfter(*firstFetAfterRelease, ti50.GpioTi50EcRstL)
	if firstEcAfterFet == nil || firstEcAfterFet.Edge != utils.GpioEdgeRising {
		s.Errorf("%s edge right after FET release is not rising edge", ti50.GpioTi50EcRstL)
		// Must return so we don't dereference null below
		return
	}

	s.Logf("EC released from Reset %dms after GSC released", (firstEcAfterFet.TimestampUS-resetReleased.TimestampUS)/1000)
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
		s.Error("EC not put in reset with GSC reboot TPMV command")
		// Must return so we don't dereference null below
		return
	}
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
		s.Error("EC not put in reset with GSC reboot console command")
		// Must return so we don't dereference null below
		return
	}
	ecResetReleased := events.FindFirstAfter(*ecReset, ti50.GpioTi50EcRstL)
	if ecResetReleased == nil {
		s.Error("EC not released from reset after GSC reboot console command")
	}
}
