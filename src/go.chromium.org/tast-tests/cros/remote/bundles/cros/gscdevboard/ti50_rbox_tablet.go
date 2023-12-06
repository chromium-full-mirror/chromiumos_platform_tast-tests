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

const (
	// tabletEcResetHoldDelay is how long EC reset keys must be held to trigger EC reset
	tabletEcResetHoldDelay = 10 * time.Second
	// tabletGscResetHoldDelay is how long GSC reset keys must be held to trigger GSC reset
	tabletGscResetHoldDelay = 20 * time.Second
	// tabletMinEcResetPulse is how long EC reset must be asserted
	tabletMinEcResetPulse = 10 * time.Millisecond
)

func init() {
	testing.AddTest(&testing.Test{
		Func:    Ti50RboxTablet,
		Desc:    "Verify keyboard combination for tablet form factor",
		Timeout: 5 * time.Minute,
		Contacts: []string{
			"chromeos-faft@google.com",
			"ti50-core@google.com",
			"jettrink@google.com",
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr:         []string{"group:gsc", "gsc_dt_ab", "gsc_dt_shield", "gsc_image_ti50", "gsc_nightly"},
		Fixture:      fixture.GSCOpenCCD,
	})
}

func Ti50RboxTablet(ctx context.Context, s *testing.State) {
	b := utils.NewDevboardHelper(s)
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}
	i := ti50.MustOpenCrOSImage(ctx, b, s)
	defer i.Close(ctx)

	s.Log("Restarting ti50 with tablet straps and SuzyQ connected")
	b.ResetWithStraps(ctx, ti50.FfTablet, ti50.CcdSuzyQ)
	th.MustSucceed(i.WaitUntilBooted(ctx), "Ti50 revives after reboot")

	s.Log("Verifying VolDown and VolUp are passed through when power button not pressed")
	b.GpioSet(ctx, ti50.GpioTi50PowerBtnL, true)
	verifyPassthrough(ctx, s, b, ti50.GpioTi50VolDownIn, ti50.GpioTi50VolDownOut)
	verifyPassthrough(ctx, s, b, ti50.GpioTi50VolUpIn, ti50.GpioTi50VolUpOut)

	s.Log("Verifying VolDown and VolUp are passed through when power button pressed")
	b.GpioSet(ctx, ti50.GpioTi50PowerBtnL, false)
	verifyPassthrough(ctx, s, b, ti50.GpioTi50VolDownIn, ti50.GpioTi50VolDownOut)
	verifyPassthrough(ctx, s, b, ti50.GpioTi50VolUpIn, ti50.GpioTi50VolUpOut)

	// Ensure both keys for combo are not pushed
	b.GpioSet(ctx, ti50.GpioTi50PowerBtnL, true)
	b.GpioSet(ctx, ti50.GpioTi50VolUpIn, false)

	s.Log("Start gpio monitoring")
	gpioMonitor := b.GpioMonitorStart(ctx, ti50.GpioTi50EcRstL)
	if gpioMonitor.InitialValues[ti50.GpioTi50EcRstL] != true {
		s.Error("EC_RST_L not de-asserted before pressing EC Refresh combo")
	}

	s.Log("Pushing Power and VolDown then wait for reset to occur")
	b.GpioSet(ctx, ti50.GpioTi50PowerBtnL, false)
	b.GpioSet(ctx, ti50.GpioTi50VolDownIn, true)
	testing.Sleep(ctx, time.Second*11) // GoBigSleepLint: Simulating button press
	b.GpioSet(ctx, ti50.GpioTi50PowerBtnL, true)
	b.GpioSet(ctx, ti50.GpioTi50VolUpIn, false)

	events := b.GpioMonitorFinish(ctx, gpioMonitor)
	s.Log("Stop gpio monitoring: ", events)

	assertReset := events.FindFirst(ti50.GpioTi50EcRstL, utils.GpioEdgeFalling)
	if assertReset == nil {
		s.Error("EC_RST_L did not assert with after 10 seconds")
	} else {
		deassertReset := events.FindFirstAfter(*assertReset, ti50.GpioTi50EcRstL)
		if deassertReset == nil {
			s.Error("EC_RST_L did not de-assert after key combo released")
		} else {
			assertTime := deassertReset.TimestampUS - assertReset.TimestampUS
			// Allow 1% measurement error.
			if assertTime < uint64(float64(tabletMinEcResetPulse.Microseconds())*0.99) {
				s.Errorf("EC_RST_L did stay asserted long enough: %dus", assertTime)
			} else {
				s.Logf("EC_RST_L asserted for %dus", assertTime)
			}

			resetDelayMs := assertReset.TimestampUS / 1000
			// Allow 2% measurement error (b/311438894).
			if resetDelayMs < uint64(float64(tabletEcResetHoldDelay.Milliseconds())*0.98) {
				s.Errorf("EC_RST_L asserted before 10s minimum hold time: %dms", resetDelayMs)
			} else {
				s.Logf("EC_RST_L delayed by %dms", resetDelayMs)
			}
		}
	}

	s.Log("Pushing GSC reset keys")
	// Ensure that all GSC reset key combo keys are not being pushed
	b.GpioSet(ctx, ti50.GpioTi50PowerBtnL, true)
	b.GpioSet(ctx, ti50.GpioTi50VolDownIn, false)
	// Volume Up is required not to be pushed during the 20 seconds
	b.GpioSet(ctx, ti50.GpioTi50VolUpIn, false)

	// Push all GSC reset keys
	b.GpioSet(ctx, ti50.GpioTi50PowerBtnL, false)
	b.GpioSet(ctx, ti50.GpioTi50VolDownIn, true)

	s.Log("Waiting for GSC to reset")
	beforeReset := time.Now()
	if err := i.WaitUntilRoBoot(ctx, tabletGscResetHoldDelay+time.Second*5); err != nil {
		s.Error("GSC did not reset with reset key combo after 25 seconds")
	} else {
		timeForReset := time.Now().Sub(beforeReset)
		// Allow 1% measurement error.
		if timeForReset.Milliseconds() < int64(float64(tabletGscResetHoldDelay.Milliseconds())*0.99) {
			s.Error("GSC reset before 20s minimum hold time: ", timeForReset)
		} else {
			s.Log("GSC reset after ", timeForReset)
		}
	}

	// Release all GSC reset keys
	b.GpioSet(ctx, ti50.GpioTi50PowerBtnL, true)
	b.GpioSet(ctx, ti50.GpioTi50VolDownIn, false)

	s.Log("Start verifying GSC reset does not trigger with Volume Up Pressed")
	// Push all GSC reset keys but with extra Volume Up, which should prevent GSC reset
	b.GpioSet(ctx, ti50.GpioTi50PowerBtnL, false)
	b.GpioSet(ctx, ti50.GpioTi50VolDownIn, true)
	b.GpioSet(ctx, ti50.GpioTi50VolUpIn, true)

	s.Log("Waiting for GSC to reset")
	beforeReset = time.Now()
	if err := i.WaitUntilRoBoot(ctx, tabletGscResetHoldDelay+time.Second*5); err != nil {
		s.Log("GSC did not reset (which is correct) after 25 seconds with Volume Up also pushed")
	} else {
		timeForReset := time.Now().Sub(beforeReset)
		s.Error("GSC reset incorrectly after ", timeForReset)
	}

	// Release all GSC reset keys
	b.GpioSet(ctx, ti50.GpioTi50PowerBtnL, true)
	b.GpioSet(ctx, ti50.GpioTi50VolDownIn, false)
	b.GpioSet(ctx, ti50.GpioTi50VolUpIn, false)

	// TODO(b/262618201) ensure that AP RO bypass key combo works
	// TODO(b/262618201) ensure that battery disconnect key combo works
	// TODO(b/262618201) ensure that RMA triggered keycombo works
}

// verifyPassthrough verifies that the specified from gpio is matches on the to gpio for both
// level high and low
func verifyPassthrough(ctx context.Context, s *testing.State, b utils.DevboardHelper, from, to ti50.GpioName) {
	b.GpioSet(ctx, from, true)
	if b.GpioGet(ctx, to) != true {
		s.Errorf("GSC should forward high from %s to %s", from, to)
	}
	b.GpioSet(ctx, from, false)
	if b.GpioGet(ctx, to) != false {
		s.Errorf("GSC should forward low from %s to %s", from, to)
	}
}
