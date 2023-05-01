// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"context"
	"time"

	"chromiumos/tast/common/firmware/ti50"
	"chromiumos/tast/remote/bundles/cros/gscdevboard/utils"
	"chromiumos/tast/remote/firmware/ti50/fixture"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:    Ti50EcReset,
		Desc:    "Test workaround for EC double reset",
		Timeout: 30 * time.Second,
		Contacts: []string{
			"chromeos-faft@google.com", // CrOS Firmware Developers
			"jbk@chromium.org",         // Test Author
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Attr:         []string{"group:firmware"},
		Fixture:      fixture.Ti50,
	})
}

func Ti50EcReset(ctx context.Context, s *testing.State) {
	f := s.FixtValue().(*fixture.Value)
	b := utils.NewDevboardHelper(f.DevBoard(), s)
	i := ti50.NewCrOSImage(b)
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}

	// Hold GSC in reset before start GPIO monitoring
	b.GpioSet(ctx, ti50.GpioTi50ResetL, false)

	s.Log("Start gpio monitoring")
	gpioMonitor := b.GpioMonitorStart(ctx, ti50.GpioTi50ResetL, ti50.GpioTi50EcRstL, ti50.GpioTi50EcRstFet)

	s.Log("Booting ti50")
	b.GpioSet(ctx, ti50.GpioTi50ResetL, true)
	th.MustSucceed(i.WaitUntilBooted(ctx), "Ti50 revives after reboot")

	// Give a little more time for gpio monitoring to catch EC_RST edges after GSC boots
	testing.Sleep(ctx, time.Second)

	events := b.GpioMonitorFinish(ctx, gpioMonitor)
	s.Log("Stop gpio monitoring: ", events)

	resetReleased := events.FindFirst(ti50.GpioTi50ResetL, utils.GpioEdgeRising)
	if resetReleased == nil {
		// Fatal since we need to deference gpio event later
		s.Fatal("GSC did not come out of reset")
	}
	firstFetAfterRelease := events.FindFirstAfter(*resetReleased, ti50.GpioTi50EcRstFet)
	if firstFetAfterRelease == nil {
		// Fatal since we need to deference gpio event later
		s.Fatalf("%s did have an edge after release GSC from reset", ti50.GpioTi50EcRstFet)
	}

	firstEcAfterFet := events.FindFirstAfter(*firstFetAfterRelease, ti50.GpioTi50EcRstL)
	if firstEcAfterFet == nil || firstEcAfterFet.Edge != utils.GpioEdgeRising {
		// Fatal since we need to deference gpio event later
		s.Fatalf("%s edge right after FET release is not rising edge", ti50.GpioTi50EcRstL)
	}

	s.Logf("EC released from Reset %dms after GSC released", (firstEcAfterFet.TimestampUS-resetReleased.TimestampUS)/1000)
}
