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
		Attr:         []string{"group:gsc", "gsc_dt_ab", "gsc_dt_shield", "gsc_image_ti50", "gsc_nightly"},
		Fixture:      fixture.GSCOpenCCD,
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

	verifyEcResetWithKeysInOrder(ctx, s, b, ti50.GpioTi50PowerBtnL, ti50.GpioTi50RecoveryIn)
	verifyEcResetWithKeysInOrder(ctx, s, b, ti50.GpioTi50RecoveryIn, ti50.GpioTi50PowerBtnL)

	s.Log("Disconnecting CCD to allow deep sleep")
	b.GpioApplyStrap(ctx, ti50.CcdDisconnected)

	s.Log("Waiting for deep sleep")
	if err := i.WaitUntilDeepSleep(ctx, boxMaxDeepSleepDelay); err != nil {
		s.Fatal("GSC dut not enter deep sleep as precondition for next test: ", err)
	}
	s.Log("GSC in deep sleep")
	verifyEcResetWithKeysInOrder(ctx, s, b, ti50.GpioTi50RecoveryIn, ti50.GpioTi50PowerBtnL)

	s.Log("Waiting for deep sleep")
	if err := i.WaitUntilDeepSleep(ctx, boxMaxDeepSleepDelay); err != nil {
		s.Fatal("GSC dut not enter deep sleep as precondition for next test: ", err)
	}
	s.Log("GSC in deep sleep")
	verifyEcResetWithKeysInOrder(ctx, s, b, ti50.GpioTi50PowerBtnL, ti50.GpioTi50RecoveryIn)

	verifyRMAKeySequence(ctx, s, b, i, ti50.GpioTi50RecoveryIn, boxEcResetGpioDelay)
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

	verifyRMAKeySequence(ctx, s, b, i, ti50.GpioTi50KsiRefresh, clamshellEcResetGpioDelay)

	// TODO(b/262618201) ensure that battery disconnect key combo works
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
			if assertTime < uint64(float64(minEcResetPulse.Microseconds())*0.99) {
				s.Errorf("EC_RST_L did stay asserted long enough: %dus", assertTime)
			} else {
				s.Logf("EC_RST_L asserted for %dus", assertTime)
			}
		}
	}
}

func ti50RBOXTablet(ctx context.Context, s *testing.State, b utils.DevboardHelper, i *ti50.CrOSImage) {
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

	s.Log("Start gpio monitoring")
	gpioMonitor := b.GpioMonitorStart(ctx, ti50.GpioTi50EcRstL)
	if gpioMonitor.InitialValues[ti50.GpioTi50EcRstL] != true {
		s.Error("EC_RST_L not de-asserted before pressing EC Refresh combo")
	}

	s.Log("Pushing Power and VolUp then wait for reset to occur")
	b.GpioSet(ctx, ti50.GpioTi50PowerBtnL, false)
	b.GpioSet(ctx, ti50.GpioTi50VolUpIn, false)
	// GoBigSleepLint: Simulating button press
	testing.Sleep(ctx, tabletEcResetGpioDelay)
	b.GpioSet(ctx, ti50.GpioTi50VolUpIn, true)
	b.GpioSet(ctx, ti50.GpioTi50PowerBtnL, true)

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
			if assertTime < uint64(float64(minEcResetPulse.Microseconds())*0.99) {
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

	// TODO(b/262618201) ensure that battery disconnect key combo works
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
