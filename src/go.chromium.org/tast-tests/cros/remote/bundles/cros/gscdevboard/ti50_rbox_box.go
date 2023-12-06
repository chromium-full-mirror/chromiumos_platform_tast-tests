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
	// boxMinEcResetPulse is how long EC reset must be asserted.
	boxMinEcResetPulse = 10 * time.Millisecond
	// deepSleepDelay the maximum amount of time we should wait in a test for deep sleep
	boxMaxDeepSleepDelay = time.Minute
)

func init() {
	testing.AddTest(&testing.Test{
		Func:    Ti50RboxBox,
		Desc:    "Verify keyboard combination for box form factor",
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

func Ti50RboxBox(ctx context.Context, s *testing.State) {
	b := utils.NewDevboardHelper(s)
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}
	i := ti50.MustOpenCrOSImage(ctx, b, s)
	defer i.Close(ctx)

	s.Log("Restarting ti50 with box straps and SuzyQ connected")
	b.ResetWithStraps(ctx, ti50.FfBox, ti50.CcdSuzyQ)
	th.MustSucceed(i.WaitUntilBooted(ctx), "Ti50 revives after reboot")

	s.Log("Verifying Recovery Button is passed through when power button not pressed")
	b.GpioSet(ctx, ti50.GpioTi50PowerBtnL, true)
	b.GpioSet(ctx, ti50.GpioTi50RecoveryIn, true)
	if b.GpioGet(ctx, ti50.GpioTi50RecoveryOut) != true {
		s.Errorf("GSC should forward high from %s to %s", ti50.GpioTi50RecoveryIn, ti50.GpioTi50RecoveryOut)
	}
	b.GpioSet(ctx, ti50.GpioTi50RecoveryIn, false)
	if b.GpioGet(ctx, ti50.GpioTi50RecoveryOut) != false {
		s.Errorf("GSC should forward low from %s to %s", ti50.GpioTi50RecoveryIn, ti50.GpioTi50RecoveryOut)
	}

	s.Log("Verifying Recovery Button is held high when power button pressed")
	b.GpioSet(ctx, ti50.GpioTi50PowerBtnL, false)
	b.GpioSet(ctx, ti50.GpioTi50RecoveryIn, true)
	if b.GpioGet(ctx, ti50.GpioTi50RecoveryOut) != true {
		s.Error("GSC should keep Recovery Button high if power button pressed")
	}
	b.GpioSet(ctx, ti50.GpioTi50RecoveryIn, false)
	if b.GpioGet(ctx, ti50.GpioTi50RecoveryOut) != true {
		s.Error("GSC should not forward Recovery Button press with Power button pressed")
	}

	boxVerifyEcResetWithKeysInOrder(ctx, s, b, ti50.GpioTi50PowerBtnL, ti50.GpioTi50RecoveryIn)
	boxVerifyEcResetWithKeysInOrder(ctx, s, b, ti50.GpioTi50RecoveryIn, ti50.GpioTi50PowerBtnL)

	s.Log("Disconnecting CCD to allow deep sleep")
	b.GpioApplyStrap(ctx, ti50.CcdDisconnected)

	s.Log("Waiting for deep sleep")
	if err := i.WaitUntilDeepSleep(ctx, boxMaxDeepSleepDelay); err != nil {
		s.Fatal("GSC dut not enter deep sleep as precondition for next test: ", err)
	}
	s.Log("GSC in deep sleep")
	boxVerifyEcResetWithKeysInOrder(ctx, s, b, ti50.GpioTi50RecoveryIn, ti50.GpioTi50PowerBtnL)

	s.Log("Waiting for deep sleep")
	if err := i.WaitUntilDeepSleep(ctx, boxMaxDeepSleepDelay); err != nil {
		s.Fatal("GSC dut not enter deep sleep as precondition for next test: ", err)
	}
	s.Log("GSC in deep sleep")
	boxVerifyEcResetWithKeysInOrder(ctx, s, b, ti50.GpioTi50PowerBtnL, ti50.GpioTi50RecoveryIn)
}

// boxVerifyEcResetWithKeysInOrder verifies that pushing the first gpio then the second for 500ms
// causes EC_RST_L to assert
func boxVerifyEcResetWithKeysInOrder(ctx context.Context, s *testing.State, b utils.DevboardHelper, first, second ti50.GpioName) {
	s.Logf("Verifying EC_RST_L asserted when pushing %s then %s", first, second)
	// Ensure that both keys start not pressed
	b.GpioSet(ctx, first, true)
	b.GpioSet(ctx, second, true)

	s.Log("Start gpio monitoring")
	gpioMonitor := b.GpioMonitorStart(ctx, ti50.GpioTi50EcRstL)
	if gpioMonitor.InitialValues[ti50.GpioTi50EcRstL] != true {
		s.Error("EC_RST_L not de-asserted before pressing EC Refresh combo")
	}

	s.Log("Pushing ", first)
	b.GpioSet(ctx, first, false)

	s.Logf("Tapping %s for 500ms", second)
	b.GpioSet(ctx, second, false)
	testing.Sleep(ctx, time.Millisecond*500) // GoBigSleepLint: Simulating button press
	b.GpioSet(ctx, second, true)

	s.Log("Releasing ", first)
	b.GpioSet(ctx, first, true)

	events := b.GpioMonitorFinish(ctx, gpioMonitor)
	s.Log("Stop gpio monitoring: ", events)

	assertReset := events.FindFirst(ti50.GpioTi50EcRstL, utils.GpioEdgeFalling)
	if assertReset == nil {
		s.Errorf("EC_RST_L did not assert with key combo %s then %s", first, second)
	} else {
		deassertReset := events.FindFirstAfter(*assertReset, ti50.GpioTi50EcRstL)
		if deassertReset == nil {
			s.Errorf("EC_RST_L did not de-assert after key combo released %s then %s", first, second)
		} else {
			assertTime := deassertReset.TimestampUS - assertReset.TimestampUS
			// Allow 1% measurement error.
			if assertTime < uint64(float64(boxMinEcResetPulse.Microseconds())*0.99) {
				s.Errorf("EC_RST_L did stay asserted long enough: %dus", assertTime)
			} else {
				s.Logf("EC_RST_L asserted for %dus", assertTime)
			}
		}
	}
}
