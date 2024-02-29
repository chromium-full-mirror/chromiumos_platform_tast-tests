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

const (
	// minEcResetPulse is how long EC reset must be asserted.
	minEcResetPulse = 10 * time.Millisecond
	// battDisconnectMinimum is how long AC must be removed before battery
	// disconnect is asserted
	battDisconnectMinimum = 5 * time.Second
)

const (
	// deepSleepDelay the maximum amount of time we should wait in a test for deep sleep
	boxMaxDeepSleepDelay = time.Minute
	// boxEcResetGpioDelay is the amount of time the EC reset key combo should be
	// held for box form factor.
	boxEcResetGpioDelay = 200 * time.Millisecond
)

const (
	// tabletEcResetHoldDelay is how long EC reset keys must be held to trigger EC reset
	tabletEcResetHoldDelay = 10 * time.Second
	// tabletGscResetHoldDelay is how long GSC reset keys must be held to trigger GSC reset
	tabletGscResetHoldDelay = 20 * time.Second
	// tabletEcResetGpioDelay is the amount of time the EC reset key combo should
	// be held for tablet form factor.
	tabletEcResetGpioDelay = tabletEcResetHoldDelay + 200*time.Millisecond
)

const (
	// clamshellGscResetHoldDelay is how long GSC reset keys must be held to trigger GSC reset
	clamshellGscResetHoldDelay = 10 * time.Second
	// deepSleepDelay the maximum amount of time we should wait in a test for deep sleep
	clamshellMaxDeepSleepDelay = time.Minute
	// clamshellEcResetGpioDelay is the amount of time the EC reset key combo
	// should be  held for clamshell form factor.
	clamshellEcResetGpioDelay = 200 * time.Millisecond
)

type ti50ValidRBOXParam struct {
	formFactor   ti50.GpioStrap
	mainFunction func(ctx context.Context, s *testing.State, b utils.DevboardHelper, i *ti50.CrOSImage)
}

func init() {
	testing.AddTest(&testing.Test{
		Func:    Ti50RBOX,
		Desc:    "Verify keyboard combination for rbox on all form factors",
		Timeout: 5 * time.Minute,
		Contacts: []string{
			"chromeos-faft@google.com",
			"ti50-core@google.com",
			"jettrink@google.com",
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr: []string{
			"group:gsc",
			"gsc_dt_ab", "gsc_dt_shield", "gsc_ot_shield",
			"gsc_image_ti50",
			"gsc_nightly"},
		Fixture: fixture.GSCOpenCCD,
		Vars:    []string{"bypass_sleep_check"},
		Params: []testing.Param{{
			Name: "clamshell",
			Val: ti50ValidRBOXParam{
				formFactor:   ti50.FfClamshell,
				mainFunction: ti50RBOXClamshell,
			},
		}, {
			Name: "tablet",
			Val: ti50ValidRBOXParam{
				formFactor:   ti50.FfTablet,
				mainFunction: ti50RBOXTablet,
			},
		}, {
			Name: "box",
			Val: ti50ValidRBOXParam{
				formFactor:   ti50.FfBox,
				mainFunction: ti50RBOXBox,
			},
		}},
	})
}

func Ti50RBOX(ctx context.Context, s *testing.State) {
	params := s.Param().(ti50ValidRBOXParam)
	b := utils.NewDevboardHelper(s)
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}
	i := ti50.MustOpenCrOSImage(ctx, b, s)
	defer i.Close(ctx)

	s.Logf("Restarting ti50 with %s straps and SuzyQ connected", string(params.formFactor))
	b.ResetWithStraps(ctx, params.formFactor, ti50.CcdSuzyQ)
	th.MustSucceed(i.WaitUntilBooted(ctx), "Ti50 revives after reboot")

	params.mainFunction(ctx, s, b, i)
}

func ti50RBOXBox(ctx context.Context, s *testing.State, b utils.DevboardHelper, i *ti50.CrOSImage) {
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

	verifyEcResetWithKeysInOrder(ctx, s, b, ti50.GpioTi50PowerBtnL, ti50.GpioTi50RecoveryIn, nil)
	verifyEcResetWithKeysInOrder(ctx, s, b, ti50.GpioTi50RecoveryIn, ti50.GpioTi50PowerBtnL, nil)

	verifyRMAKeySequence(ctx, s, b, i, ti50.GpioTi50RecoveryIn, boxEcResetGpioDelay)

	// Finish test if bypass_sleep_check var is present (even if value is "false")
	if _, noSleepCheck := s.Var("bypass_sleep_check"); noSleepCheck {
		s.Log("Bypassing all rbox sleep tests")
		return
	}

	s.Log("Disconnecting CCD to allow deep sleep")
	b.GpioApplyStrap(ctx, ti50.CcdDisconnected)

	s.Log("Waiting for deep sleep")
	if err := i.WaitUntilDeepSleep(ctx, boxMaxDeepSleepDelay); err != nil {
		s.Fatal("GSC dut not enter deep sleep as precondition for next test: ", err)
	}
	s.Log("GSC in deep sleep")
	verifyEcResetWithKeysInOrder(ctx, s, b, ti50.GpioTi50RecoveryIn, ti50.GpioTi50PowerBtnL, nil)

	s.Log("Waiting for deep sleep")
	if err := i.WaitUntilDeepSleep(ctx, boxMaxDeepSleepDelay); err != nil {
		s.Fatal("GSC dut not enter deep sleep as precondition for next test: ", err)
	}
	s.Log("GSC in deep sleep")
	verifyEcResetWithKeysInOrder(ctx, s, b, ti50.GpioTi50PowerBtnL, ti50.GpioTi50RecoveryIn, nil)
}

func ti50RBOXClamshell(ctx context.Context, s *testing.State, b utils.DevboardHelper, i *ti50.CrOSImage) {
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}
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

	verifyEcResetWithKeysInOrder(ctx, s, b, ti50.GpioTi50PowerBtnL, ti50.GpioTi50KsiRefresh, nil)
	verifyEcResetWithKeysInOrder(ctx, s, b, ti50.GpioTi50KsiRefresh, ti50.GpioTi50PowerBtnL, nil)

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

	verifyRMAKeySequence(ctx, s, b, i, ti50.GpioTi50KsiRefresh, clamshellEcResetGpioDelay)
	verifyBatteryDisconnectCancelled(ctx, s, b, ti50.GpioTi50KsiRefresh)
	verifyBatteryDisconnect(ctx, s, b, ti50.GpioTi50KsiRefresh)

	// Finish test if bypass_sleep_check var is present (even if value is "false")
	if _, noSleepCheck := s.Var("bypass_sleep_check"); noSleepCheck {
		s.Log("Bypassing all rbox sleep tests")
		return
	}

	s.Log("Disconnecting CCD to allow deep sleep")
	b.GpioApplyStrap(ctx, ti50.CcdDisconnected)

	s.Log("Waiting for deep sleep")
	if err := i.WaitUntilDeepSleep(ctx, clamshellMaxDeepSleepDelay); err != nil {
		s.Fatal("GSC dut not enter deep sleep as precondition for next test: ", err)
	}
	s.Log("GSC in deep sleep")
	verifyEcResetWithKeysInOrder(ctx, s, b, ti50.GpioTi50KsiRefresh, ti50.GpioTi50PowerBtnL, nil)

	s.Log("Waiting for deep sleep")
	if err := i.WaitUntilDeepSleep(ctx, clamshellMaxDeepSleepDelay); err != nil {
		s.Fatal("GSC dut not enter deep sleep as precondition for next test: ", err)
	}
	s.Log("GSC in deep sleep")
	verifyEcResetWithKeysInOrder(ctx, s, b, ti50.GpioTi50PowerBtnL, ti50.GpioTi50KsiRefresh, nil)

	s.Log("Reconnecting SuzyQ to prevent deep sleep")
	b.GpioApplyStrap(ctx, ti50.CcdSuzyQ)
	th.MustSucceed(i.WaitUntilBooted(ctx), "Ti50 is awake")
}

func ti50RBOXTablet(ctx context.Context, s *testing.State, b utils.DevboardHelper, i *ti50.CrOSImage) {
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}
	s.Log("Verifying VolDown and VolUp are passed through when power button not pressed")
	b.GpioSet(ctx, ti50.GpioTi50PowerBtnL, true)
	verifyPassthrough(ctx, s, b, ti50.GpioTi50VolDownIn, ti50.GpioTi50VolDownOut)
	verifyPassthrough(ctx, s, b, ti50.GpioTi50VolUpIn, ti50.GpioTi50VolUpOut)

	s.Log("Verifying VolDown and VolUp are passed through when power button pressed")
	b.GpioSet(ctx, ti50.GpioTi50PowerBtnL, false)
	verifyPassthrough(ctx, s, b, ti50.GpioTi50VolDownIn, ti50.GpioTi50VolDownOut)
	verifyPassthrough(ctx, s, b, ti50.GpioTi50VolUpIn, ti50.GpioTi50VolUpOut)

	// Ensure keys for combo are not pushed
	b.GpioSet(ctx, ti50.GpioTi50PowerBtnL, true)
	b.GpioSet(ctx, ti50.GpioTi50VolDownIn, true)
	b.GpioSet(ctx, ti50.GpioTi50VolUpIn, true)

	delay := tabletEcResetHoldDelay
	verifyEcResetWithKeysInOrder(ctx, s, b, ti50.GpioTi50PowerBtnL, ti50.GpioTi50VolUpIn, &delay)
	verifyEcResetWithKeysInOrder(ctx, s, b, ti50.GpioTi50VolUpIn, ti50.GpioTi50PowerBtnL, &delay)

	// Ensure EC reset works when Volume Down is also pressed. This is the
	// recovery key combo and needs to reset the EC as well.
	s.Log("Verify EC reset still happens when VolDown is also pressed")
	b.GpioSet(ctx, ti50.GpioTi50VolDownIn, false)
	verifyEcResetWithKeysInOrder(ctx, s, b, ti50.GpioTi50PowerBtnL, ti50.GpioTi50VolUpIn, &delay)
	b.GpioSet(ctx, ti50.GpioTi50VolDownIn, true)

	s.Log("Pushing GSC reset keys")
	// Ensure that all GSC reset key combo keys are not being pushed
	b.GpioSet(ctx, ti50.GpioTi50PowerBtnL, true)
	b.GpioSet(ctx, ti50.GpioTi50VolUpIn, true)
	// Volume Down is required not to be pushed during the 20 seconds
	b.GpioSet(ctx, ti50.GpioTi50VolDownIn, true)

	// Push all GSC reset keys
	b.GpioSet(ctx, ti50.GpioTi50PowerBtnL, false)
	b.GpioSet(ctx, ti50.GpioTi50VolUpIn, false)

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
	b.GpioSet(ctx, ti50.GpioTi50VolUpIn, true)

	s.Log("Start verifying GSC reset does not trigger with Volume Down pressed")
	// Push all GSC reset keys but with extra Volume Down, which should prevent GSC reset
	th.MustSucceed(i.WaitUntilBooted(ctx), "GSC booted")
	b.GpioSet(ctx, ti50.GpioTi50PowerBtnL, false)
	b.GpioSet(ctx, ti50.GpioTi50VolUpIn, false)
	b.GpioSet(ctx, ti50.GpioTi50VolDownIn, false)

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
	b.GpioSet(ctx, ti50.GpioTi50VolUpIn, true)
	b.GpioSet(ctx, ti50.GpioTi50VolDownIn, true)

	verifyRMAKeySequence(ctx, s, b, i, ti50.GpioTi50VolUpIn, tabletEcResetGpioDelay)
	verifyBatteryDisconnectCancelled(ctx, s, b, ti50.GpioTi50VolUpIn)
	verifyBatteryDisconnect(ctx, s, b, ti50.GpioTi50VolUpIn)
}

// verifyEcResetWithKeysInOrder verifies that pushing the first gpio then the
// second for the minimum + 500ms causes EC_RST_L to assert
func verifyEcResetWithKeysInOrder(ctx context.Context, s *testing.State, b utils.DevboardHelper, first, second ti50.GpioName, minHold *time.Duration) {
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

	hold := time.Millisecond * 500
	if minHold != nil {
		hold += *minHold
	}
	s.Logf("Pressing %s for %s", second, hold)
	b.GpioSet(ctx, second, false)
	testing.Sleep(ctx, hold) // GoBigSleepLint: Simulating press push
	b.GpioSet(ctx, second, true)

	s.Log("Releasing ", first)
	b.GpioSet(ctx, first, true)

	events := b.GpioMonitorFinish(ctx, gpioMonitor)
	s.Log("Stop gpio monitoring: ", events)

	assertReset := events.FindFirst(ti50.GpioTi50EcRstL, utils.GpioEdgeFalling)
	if assertReset == nil {
		s.Errorf("EC_RST_L did not assert with key combo %s then %s", first, second)
		return
	}
	deassertReset := events.FindFirstAfter(*assertReset, ti50.GpioTi50EcRstL)
	if deassertReset == nil {
		s.Errorf("EC_RST_L did not de-assert after key combo released %s then %s", first, second)
		return
	}
	assertTime := deassertReset.TimestampUS - assertReset.TimestampUS
	// Allow 1% measurement error.
	if assertTime < uint64(float64(minEcResetPulse.Microseconds())*0.99) {
		s.Errorf("EC_RST_L did stay asserted long enough: %dus", assertTime)
		return
	}
	s.Logf("EC_RST_L asserted for %dus", assertTime)

	if minHold != nil {
		resetDelayMs := assertReset.TimestampUS / 1000
		// Allow 2% measurement error (b/311438894).
		if resetDelayMs < uint64(float64(minHold.Milliseconds())*0.98) {
			s.Errorf("EC_RST_L asserted before minimum hold time: %dms", resetDelayMs)
			return
		}
		s.Logf("EC_RST_L delayed by %dms", resetDelayMs)
	}
}

// verifyBatteryDisconnect verifies the battery disconnect sequence
func verifyBatteryDisconnect(ctx context.Context, s *testing.State, b utils.DevboardHelper, ecResetPin ti50.GpioName) {
	s.Log("Start gpio monitoring for battery disconnect ")

	// Start with AC connected
	b.GpioSet(ctx, ti50.GpioTi50ACPresent, true)

	gpioMonitor := b.GpioMonitorStart(ctx, ti50.GpioTi50BattDisableL, ti50.GpioTi50ACPresent)
	if gpioMonitor.InitialValues[ti50.GpioTi50BattDisableL] != true {
		s.Errorf("%s not de-asserted before battery disconnect combo", ti50.GpioTi50BattDisableL)
	}

	// Press EC Reset key combo
	b.GpioSet(ctx, ti50.GpioTi50PowerBtnL, false)
	b.GpioSet(ctx, ecResetPin, false)

	// Remove power
	b.GpioSet(ctx, ti50.GpioTi50ACPresent, false)

	s.Log("Hold battery disconnect key combo for a few seconds")
	// GoBigSleepLint: Hold key combo for 5+ seconds
	testing.Sleep(ctx, battDisconnectMinimum+time.Second)

	// Release EC Reset key combo
	b.GpioSet(ctx, ti50.GpioTi50PowerBtnL, true)
	b.GpioSet(ctx, ecResetPin, true)

	events := b.GpioMonitorFinish(ctx, gpioMonitor)
	s.Log("Stop gpio monitoring: ", events)

	assertBattDisable := events.FindFirst(ti50.GpioTi50BattDisableL, utils.GpioEdgeFalling)
	if assertBattDisable == nil {
		s.Errorf("%s did not assert with key combo", ti50.GpioTi50BattDisableL)
	} else {
		acRemovedEdge := events.FindFirst(ti50.GpioTi50ACPresent, utils.GpioEdgeFalling)
		if acRemovedEdge == nil {
			s.Fatal("Could not find AC_PRESENT falling edge")
		}
		delayTime := assertBattDisable.TimestampUS - acRemovedEdge.TimestampUS
		// Allow 1% measurement error.
		if delayTime < uint64(float64(battDisconnectMinimum.Microseconds())*0.99) {
			s.Errorf("Asserted %s too soon after removing AC power: %dus", ti50.GpioTi50BattDisableL, delayTime)
		} else if delayTime > uint64(float64(battDisconnectMinimum.Microseconds())*1.01) {
			s.Errorf("Asserted %s too later after removing AC power: %dus", ti50.GpioTi50BattDisableL, delayTime)
		} else {
			s.Logf("%s asserted after %dus", ti50.GpioTi50BattDisableL, delayTime)
		}
	}
}

// verifyBatteryDisconnectCancelled verifies the battery disconnect sequence
// is cancelled if the AC power is re connected within 5 seconds
func verifyBatteryDisconnectCancelled(ctx context.Context, s *testing.State, b utils.DevboardHelper, ecResetPin ti50.GpioName) {
	s.Log("Start gpio monitoring for battery disconnect")

	// Start with AC connected
	b.GpioSet(ctx, ti50.GpioTi50ACPresent, true)
	gpioMonitor := b.GpioMonitorStart(ctx, ti50.GpioTi50BattDisableL, ti50.GpioTi50ACPresent)
	if gpioMonitor.InitialValues[ti50.GpioTi50BattDisableL] != true {
		s.Errorf("%s not de-asserted before battery disconnect combo", ti50.GpioTi50BattDisableL)
	}

	// Press EC Reset key combo
	b.GpioSet(ctx, ti50.GpioTi50PowerBtnL, false)
	b.GpioSet(ctx, ecResetPin, false)

	// Remove power
	b.GpioSet(ctx, ti50.GpioTi50ACPresent, false)

	s.Log("Hold battery disconnect key combo for a few seconds")
	// GoBigSleepLint: Hold key combo for half of the required time
	testing.Sleep(ctx, battDisconnectMinimum/2)

	// Send a pulse on AC Present. This should cancel battery disconnect
	b.GpioSet(ctx, ti50.GpioTi50ACPresent, true)
	b.GpioSet(ctx, ti50.GpioTi50ACPresent, false)

	s.Log("Hold battery disconnect key combo for a few seconds")
	// GoBigSleepLint: Hold key combo for half of the required time
	testing.Sleep(ctx, battDisconnectMinimum/2+time.Second)

	// Release EC Reset key combo
	b.GpioSet(ctx, ti50.GpioTi50PowerBtnL, true)
	b.GpioSet(ctx, ecResetPin, true)

	events := b.GpioMonitorFinish(ctx, gpioMonitor)
	s.Log("Stop gpio monitoring: ", events)

	assertBattDisable := events.FindFirst(ti50.GpioTi50BattDisableL, utils.GpioEdgeFalling)
	if assertBattDisable != nil {
		acRemovedEdge := events.FindFirst(ti50.GpioTi50ACPresent, utils.GpioEdgeFalling)
		if acRemovedEdge == nil {
			s.Fatal("Could not find AC_PRESENT falling edge")
		}
		delayTime := assertBattDisable.TimestampUS - acRemovedEdge.TimestampUS
		s.Errorf("Battery disconnect after %dus when it should have been canceled", delayTime)
	}
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

func verifyRMAKeySequence(ctx context.Context, s *testing.State, b utils.DevboardHelper, i *ti50.CrOSImage, gpio ti50.GpioName, ecResetGpioDelay time.Duration) {
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}
	params := s.Param().(ti50ValidRBOXParam)
	assertVal := false

	// Verify that AP RO result is one of the V2 errors before performing
	// the RMA key sequence. This only applies to Ti50 not Cr50.
	tpm := b.ResetAndTpmStartup(ctx, i, params.formFactor)
	mode, err := tpm.TpmvGetApRoVerificationStatus()
	th.MustSucceed(err, "Get AP RO verification status via TPMV command")
	if !mode.IsV2Code() {
		s.Error("AP RO verification status not V2 code: ", mode)
	}

	// Turn AP off while performing RMA key combo (AP would be shut off with the
	// first refresh/recovery key tap anyway due to EC reset).
	s.Logf("Send RMA request key combo (takes more than %s)", ecResetGpioDelay*3)
	b.GpioSet(ctx, ti50.GpioTi50PltRstL, false)
	b.GpioSet(ctx, ti50.GpioTi50PowerBtnL, false)

	b.GpioSet(ctx, gpio, assertVal)
	// GoBigSleepLint: Simulating button press
	testing.Sleep(ctx, ecResetGpioDelay)
	b.GpioSet(ctx, gpio, !assertVal)

	// GoBigSleepLint: Simulating reasonable time between button press
	testing.Sleep(ctx, time.Millisecond*500)

	b.GpioSet(ctx, gpio, assertVal)
	// GoBigSleepLint: Simulating button press
	testing.Sleep(ctx, ecResetGpioDelay)
	b.GpioSet(ctx, gpio, !assertVal)

	// GoBigSleepLint: Simulating reasonable time between button press
	testing.Sleep(ctx, time.Millisecond*500)

	b.GpioSet(ctx, gpio, assertVal)
	// GoBigSleepLint: Simulating button press
	testing.Sleep(ctx, ecResetGpioDelay)
	b.GpioSet(ctx, gpio, !assertVal)

	b.GpioSet(ctx, ti50.GpioTi50PowerBtnL, true)

	// Turn AP back on
	b.GpioSet(ctx, ti50.GpioTi50PltRstL, true)
	b.WaitForTpm(ctx, tpm)

	// Turn AP back off. This should not reset the RMA request since we are not
	// going to read the RMA request via TPMV command.
	b.GpioSet(ctx, ti50.GpioTi50PltRstL, false)

	// GoBigSleepLint: Simulating AP being off for reasonable amount of time
	testing.Sleep(ctx, time.Millisecond*200)

	// Turn AP back on. Should still have RMA request pending
	b.GpioSet(ctx, ti50.GpioTi50PltRstL, true)
	b.WaitForTpm(ctx, tpm)

	// Ensure that AP RO verification status is a V1 code. This indicates that
	// RMA was requested.
	mode, err = tpm.TpmvGetApRoVerificationStatus()
	th.MustSucceed(err, "Get AP RO verification status via TPMV command")
	if mode.IsV2Code() {
		s.Error("AP RO verification status should be V1 code: ", mode)
	}

	// Ensure that AP RO verification status is still a V1 code. The status should
	// not reset until AP is reset.
	mode, err = tpm.TpmvGetApRoVerificationStatus()
	th.MustSucceed(err, "Get AP RO verification status via TPMV command")
	if mode.IsV2Code() {
		s.Error("AP RO verification status should be V1 code: ", mode)
	}

	// Turn AP back off. This should reset the RMA request since we read the
	// RMA request.
	b.GpioSet(ctx, ti50.GpioTi50PltRstL, false)

	// GoBigSleepLint: Simulating AP being off for reasonable amount of time
	testing.Sleep(ctx, time.Millisecond*200)

	// Turn AP back on. Should still have RMA request pending
	b.GpioSet(ctx, ti50.GpioTi50PltRstL, true)
	b.WaitForTpm(ctx, tpm)

	// Ensure that AP RO verification status has returned to V2 code meaning there
	// is not RMA request pending.
	mode, err = tpm.TpmvGetApRoVerificationStatus()
	th.MustSucceed(err, "Get AP RO verification status via TPMV command")
	if !mode.IsV2Code() {
		s.Error("AP RO verification status should be V2 code: ", mode)
	}
}
