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
	// clamshellGscResetHoldDelay is how long GSC reset keys must be held to trigger GSC reset
	clamshellGscResetHoldDelay = 10 * time.Second
	// clamshellMinEcResetPulse is how long EC reset must be asserted.
	clamshellMinEcResetPulse = 10 * time.Millisecond
	// deepSleepDelay the maximum amount of time we should wait in a test for deep sleep
	clamshellMaxDeepSleepDelay = time.Minute
)

func init() {
	testing.AddTest(&testing.Test{
		Func:    Ti50RboxClamshell,
		Desc:    "Verify keyboard combination for clamshell form factor",
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

func Ti50RboxClamshell(ctx context.Context, s *testing.State) {
	b := utils.NewDevboardHelper(s)
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}
	i := ti50.MustOpenCrOSImage(ctx, b, s)
	defer i.Close(ctx)

	s.Log("Restarting ti50 with clamshell straps and SuzyQ connected")
	b.ResetWithStraps(ctx, ti50.FfClamshell, ti50.CcdSuzyQ)
	th.MustSucceed(i.WaitUntilBooted(ctx), "Ti50 revives after reboot")

	s.Log("Verifying KSO is passed through when power button not pressed")
	b.GpioSet(ctx, ti50.GpioTi50PowerBtnL, true)
	b.GpioSet(ctx, ti50.GpioTi50EcKso2Inv, true)
	if b.GpioGet(ctx, ti50.GpioTi50Kso2) != false {
		s.Error("GSC should forward asserted GpioTi50EcKso2Inv")
	}

	b.GpioSet(ctx, ti50.GpioTi50EcKso2Inv, false)
	if b.GpioGet(ctx, ti50.GpioTi50Kso2) != true {
		s.Error("GSC should forward de-asserted GpioTi50EcKso2Inv")
	}

	s.Log("Verifying KSO is always asserted when power button pressed")
	b.GpioSet(ctx, ti50.GpioTi50PowerBtnL, false)
	if b.GpioGet(ctx, ti50.GpioTi50Kso2) != false {
		s.Error("GSC should be asserting KSO low when power button is pressed")
	}
	b.GpioSet(ctx, ti50.GpioTi50PowerBtnL, true)

	verifyEcResetWithKeysInOrder(ctx, s, b, ti50.GpioTi50PowerBtnL, ti50.GpioTi50KsiRefresh)
	verifyEcResetWithKeysInOrder(ctx, s, b, ti50.GpioTi50KsiRefresh, ti50.GpioTi50PowerBtnL)

	s.Log("Disconnecting CCD to allow deep sleep")
	b.GpioApplyStrap(ctx, ti50.CcdDisconnected)

	s.Log("Waiting for deep sleep")
	if err := i.WaitUntilDeepSleep(ctx, clamshellMaxDeepSleepDelay); err != nil {
		s.Fatal("GSC dut not enter deep sleep as precondition for next test: ", err)
	}
	s.Log("GSC in deep sleep")
	verifyEcResetWithKeysInOrder(ctx, s, b, ti50.GpioTi50KsiRefresh, ti50.GpioTi50PowerBtnL)

	s.Log("Waiting for deep sleep")
	if err := i.WaitUntilDeepSleep(ctx, clamshellMaxDeepSleepDelay); err != nil {
		s.Fatal("GSC dut not enter deep sleep as precondition for next test: ", err)
	}
	s.Log("GSC in deep sleep")
	verifyEcResetWithKeysInOrder(ctx, s, b, ti50.GpioTi50PowerBtnL, ti50.GpioTi50KsiRefresh)

	s.Log("Reconnecting SuzyQ to prevent deep sleep")
	b.GpioApplyStrap(ctx, ti50.CcdSuzyQ)
	th.MustSucceed(i.WaitUntilBooted(ctx), "Ti50 is awake")

	s.Log("Pushing GSC reset keys")
	// Ensure that all GSC reset key combo keys are not being pushed yet
	b.GpioSet(ctx, ti50.GpioTi50PowerBtnL, true)
	b.GpioSet(ctx, ti50.GpioTi50KsiRefresh, true)
	b.GpioSet(ctx, ti50.GpioTi50KsiBack, true)

	// Push all GSC reset keys
	b.GpioSet(ctx, ti50.GpioTi50PowerBtnL, false)
	b.GpioSet(ctx, ti50.GpioTi50KsiRefresh, false)
	b.GpioSet(ctx, ti50.GpioTi50KsiBack, false)

	s.Log("Waiting for GSC to reset")
	beforeReset := time.Now()
	if err := i.WaitUntilRoBoot(ctx, clamshellGscResetHoldDelay+time.Second*5); err != nil {
		s.Error("GSC did not reset with reset key combo after 10 seconds")
	} else {
		timeForReset := time.Now().Sub(beforeReset)
		// Allow 1% measurement error.
		if timeForReset.Milliseconds() < int64(float64(clamshellGscResetHoldDelay.Milliseconds())*0.99) {
			s.Error("GSC reset before 10s minimum hold time: ", timeForReset)
		} else {
			s.Log("GSC reset after ", timeForReset)
		}
	}

	// Release all GSC reset keys
	b.GpioSet(ctx, ti50.GpioTi50PowerBtnL, true)
	b.GpioSet(ctx, ti50.GpioTi50KsiRefresh, true)
	b.GpioSet(ctx, ti50.GpioTi50KsiBack, true)

	// TODO(b/262618201) finish the rest of test for other form factors
	// TODO(b/262618201) ensure that AP RO bypass key combo works
	// TODO(b/262618201) ensure that battery disconnect key combo works
	// TODO(b/262618201) ensure that RMA triggered keycombo works
}

// verifyEcResetWithKeysInOrder verifies that pushing the first gpio then the second for 500ms
// causes EC_RST_L to assert
func verifyEcResetWithKeysInOrder(ctx context.Context, s *testing.State, b utils.DevboardHelper, first, second ti50.GpioName) {
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
			if assertTime < uint64(float64(clamshellMinEcResetPulse.Microseconds())*0.99) {
				s.Errorf("EC_RST_L did stay asserted long enough: %dus", assertTime)
			} else {
				s.Logf("EC_RST_L asserted for %dus", assertTime)
			}
		}
	}
}
