// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"bytes"
	"context"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/gscdevboard/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"
	"go.chromium.org/tast/core/testing"
)

type ti50SleepParam struct {
	tpmCommunication    ti50.TpmBus
	tpmStrapping        ti50.GpioStrap
	servoMicroStrapping ti50.GpioStrap
}

func init() {
	testing.AddTest(&testing.Test{
		Func:    Ti50Sleep,
		Desc:    "Test ti50 deep/normal sleep on Andreiboard connected to devboardsvc host",
		Timeout: 15 * time.Minute,
		Contacts: []string{
			"chromeos-faft@google.com", // CrOS Firmware Developers
			"jbk@chromium.org",         // Test Author
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr:         []string{"group:gsc", "gsc_dt_ab", "gsc_dt_shield", "gsc_image_ti50", "gsc_nightly"},
		Fixture:      fixture.GSCOpenCCD,
		Vars:         []string{"bypass_sleep_check"},
		Params: []testing.Param{{
			Name: "spi_no_uservo",
			Val: ti50SleepParam{
				tpmCommunication:    ti50.TpmBusSpi,
				tpmStrapping:        ti50.TpmSpi,
				servoMicroStrapping: ti50.ServoMicroDisconnected,
			},
		}, {
			Name: "spi_uservo",
			Val: ti50SleepParam{
				tpmCommunication:    ti50.TpmBusSpi,
				tpmStrapping:        ti50.TpmSpi,
				servoMicroStrapping: ti50.ServoMicroConnected,
			},
		}, {
			Name: "i2c_no_uservo",
			Val: ti50SleepParam{
				tpmCommunication:    ti50.TpmBusI2c,
				tpmStrapping:        ti50.TpmI2c,
				servoMicroStrapping: ti50.ServoMicroDisconnected,
			},
		}, {
			Name: "i2c_uservo",
			Val: ti50SleepParam{
				tpmCommunication:    ti50.TpmBusI2c,
				tpmStrapping:        ti50.TpmI2c,
				servoMicroStrapping: ti50.ServoMicroConnected,
			},
		}},
	})
}

var (
	reBoot       *regexp.Regexp = regexp.MustCompile(`Ravn4|([0-9a-fA-F]{8})`)
	reResetType  *regexp.Regexp = regexp.MustCompile(`Reset Type: ([0-9a-zA-Z_]*)[\r\n]`)
	reWakeSource *regexp.Regexp = regexp.MustCompile(`Wake source: 0x([0-9a-fA-F]{8})`)
	reConsole    *regexp.Regexp = regexp.MustCompile(`Console is enabled`)
	// Time to wait to ensure GSC did not go to sleep. This should be longer
	// than to longest sleep delay in hil/pmu.rs
	waitForNoSleep = 70 * time.Second
)

const (
	wakeSourceGpio  = "80000001"
	wakeSourceRbox  = "80000004"
	wakeSourceAdc   = "80000008"
	wakeSourceUart1 = "80000010"
	wakeSourceUart2 = "80000020"
	wakeSourceUart3 = "80000040"
	wakeSourceUart4 = "80000080"
	wakeSourceUart5 = "80000100"
)

func logCurrent(ctx context.Context, s *testing.State, b utils.DevboardHelper, pv *perf.Values, label string) {
	for n := 1; n <= 5; n++ {
		testing.Sleep(ctx, 100*time.Millisecond) // GoBigSleepLint: Space out readings
		c := b.ReadGscTotalMilliAmps(ctx)
		pv.Append(perf.Metric{
			Name:      label,
			Unit:      "milliamps",
			Direction: perf.SmallerIsBetter,
			Multiple:  true,
		}, float64(c))
		s.Logf("%s: %.2f mA", label, c)
	}
}

func verifyDeepSleep(ctx context.Context, s *testing.State, i *ti50.CrOSImage, th utils.FirmwareTestingHelper) {
	s.Log("Waiting for sleep with AP off")
	th.MustSucceed(i.WaitUntilDeepSleep(ctx, time.Minute), "Ti50 did not sleep when AP off")
	_, err := i.WaitUntilMatch(ctx, reBoot, time.Second*3)
	if err != nil {
		// We expected to NOT see boot a message.
		return
	}
	s.Error("Ti50 did not stay asleep")
}

// verifyNormalWakeup verifies that Ti50 emits console output indicating wakeup from normal sleep.
// Aside from reporting any unexpected behavior through `s.Error`, the return value will be true
// only if Ti50 did wake up, which is useful for the top-level test script to know which state
// Ti50 is in, in case it wants to continue testing other aspects.
func verifyNormalWakeup(ctx context.Context, s *testing.State, i *ti50.CrOSImage, b utils.DevboardHelper, gpioMonitor utils.GpioMonitorSession, expected, trigger string) bool {
	wakeMatch, err := b.ReadSerialSubmatch(ctx, reWakeSource)
	if err != nil {
		s.Errorf("Waking on %s: Unable to recognize wake source", trigger)
		return false
	}
	if string(wakeMatch[1]) != expected {
		s.Errorf("Waking on %s: Unexpected wake mask: got %s, expected %s", trigger, wakeMatch[1], expected)
	}
	if err := i.WaitUntilBooted(ctx); err != nil {
		s.Error("Ti50 did not wake up by ", trigger)
	}
	events := b.GpioMonitorRead(ctx, gpioMonitor)
	if len(events.Sorted) != 0 {
		s.Errorf("Waking on %s: Unexpected gpio events: %s", trigger, events)
	}
	return true
}

// verifyDeepWakeup verifies that Ti50 emits console output indicating wakeup from deep sleep.
// Aside from reporting any unexpected behavior through `s.Error`, the return value will be true
// only if Ti50 did wake up, which is useful for the top-level test script to know which state
// Ti50 is in, in case it wants to continue testing other aspects.
func verifyDeepWakeup(ctx context.Context, s *testing.State, i *ti50.CrOSImage, b utils.DevboardHelper, gpioMonitor utils.GpioMonitorSession, expected, trigger string) bool {
	_, err := b.ReadSerialSubmatch(ctx, reBoot)
	if err != nil {
		s.Error("Ti50 did not wake up by ", trigger)
		return false
	}
	resetMatch, err := b.ReadSerialSubmatch(ctx, reResetType)
	if err != nil {
		s.Errorf("Waking on %s: did not recognize reset type", trigger)
		return true
	}
	if string(resetMatch[1]) == "Wake" {
		wakeMatch, err := b.ReadSerialSubmatch(ctx, reWakeSource)
		if err != nil {
			s.Errorf("Waking on %s: Unable to recognize wake source", trigger)
			return true
		}
		if string(wakeMatch[1]) != expected {
			s.Errorf("Waking on %s: Unexpected wake mask: got %s, expected %s", trigger, wakeMatch[1], expected)
		}
	} else {
		s.Errorf("Waking on %s: Unexpected reset type: got %q, expected %q", trigger, string(resetMatch[1]), "Wake")
	}
	_, err = b.ReadSerialSubmatch(ctx, reConsole)
	if err != nil {
		s.Error("Console not enabled after wake up by ", trigger)
		return true
	}
	if err := i.WaitUntilBooted(ctx); err != nil {
		s.Error("Ti50 did not wake up by ", trigger)
	}
	events := b.GpioMonitorRead(ctx, gpioMonitor)
	if len(events.Sorted) != 0 {
		s.Errorf("Waking on %s: Unexpected gpio events: %s", trigger, events)
	}
	return true
}

func Ti50Sleep(ctx context.Context, s *testing.State) {
	// Check if runtime bypass_sleep_check var is present, if so (even if value is
	// "false") set the 70 seconds wait to 1 second the rest of the test
	if _, fastSleep := s.Var("bypass_sleep_check"); fastSleep {
		s.Log("All sleep verification waits reduced from 70 seconds to 1 second")
		waitForNoSleep = 1 * time.Second
	}

	testParams := s.Param().(ti50SleepParam)
	b := utils.NewDevboardHelper(s)
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}
	i := ti50.MustOpenCrOSImage(ctx, b, s)
	defer i.Close(ctx)
	pv := perf.NewValues()

	s.Log("Restarting ti50 with appropriate straps")
	b.GpioSet(ctx, ti50.GpioTi50PltRstL, false)
	b.GpioSet(ctx, ti50.GpioTi50LidOpen, false)
	b.GpioSet(ctx, ti50.GpioTi50EcRstL, true)
	b.GpioSet(ctx, ti50.GpioTi50EcPacketMode, false)
	b.GpioSet(ctx, ti50.GpioTi50CcdModeL, true)
	b.ResetWithStraps(ctx, testParams.servoMicroStrapping, ti50.CcdDisconnected, testParams.tpmStrapping)
	th.MustSucceed(i.WaitUntilBooted(ctx), "Ti50 revives after reboot")
	gpioMonitor := b.GpioMonitorStart(ctx, ti50.GpioTi50EcRstL, ti50.GpioTi50EcRstFet)

	logCurrent(ctx, s, b, pv, "Awake")

	verifyDeepSleep(ctx, s, i, th)
	logCurrent(ctx, s, b, pv, "DeepSleep")

	s.Log("Simulating power button press")
	b.GpioSet(ctx, ti50.GpioTi50PowerBtnL, false)
	b.GpioSet(ctx, ti50.GpioTi50PowerBtnL, true)
	if verifyDeepWakeup(ctx, s, i, b, gpioMonitor, wakeSourceRbox, "Power Button") {
		verifyDeepSleep(ctx, s, i, th)
	}

	s.Log("Simulating lid low-to-high event")
	b.GpioSet(ctx, ti50.GpioTi50LidOpen, true)
	if verifyDeepWakeup(ctx, s, i, b, gpioMonitor, wakeSourceGpio, "Lid low-to-high event") {
		verifyDeepSleep(ctx, s, i, th)
	}

	s.Logf("Simulating EC packet mode, wait %s", waitForNoSleep)
	b.GpioSet(ctx, ti50.GpioTi50EcPacketMode, true)
	if verifyDeepWakeup(ctx, s, i, b, gpioMonitor, wakeSourceGpio, "EC packet mode") {
		if err := i.WaitUntilAnySleep(ctx, waitForNoSleep); err == nil {
			s.Error("Ti50 went to sleep while EC packet mode asserted")
		}
		b.GpioSet(ctx, ti50.GpioTi50EcPacketMode, false)
		verifyDeepSleep(ctx, s, i, th)
	}

	s.Logf("Simulating CCD_MODE asserted, wait %s", waitForNoSleep)
	b.GpioSet(ctx, ti50.GpioTi50CcdModeL, false)
	if verifyDeepWakeup(ctx, s, i, b, gpioMonitor, wakeSourceGpio, "CCD_MODE asserted") {
		if err := i.WaitUntilAnySleep(ctx, waitForNoSleep); err == nil {
			s.Error("Ti50 went to sleep while CCD_MODE asserted")
		}
		b.GpioSet(ctx, ti50.GpioTi50CcdModeL, true)
		verifyDeepSleep(ctx, s, i, th)
	}

	s.Logf("Simulating SuzyQ inserted, wait %s", waitForNoSleep)
	b.GpioApplyStrap(ctx, ti50.CcdSuzyQ)
	if verifyDeepWakeup(ctx, s, i, b, gpioMonitor, wakeSourceAdc, "CCD connection") {
		logCurrent(ctx, s, b, pv, "Awake_CCD")
		if err := i.WaitUntilAnySleep(ctx, waitForNoSleep); err == nil {
			s.Error("Ti50 went to sleep while SuzyQ connected")
		}
		b.GpioApplyStrap(ctx, ti50.CcdDisconnected)
		verifyDeepSleep(ctx, s, i, th)
		logCurrent(ctx, s, b, pv, "DeepSleep_CCD")
	} else {
		// Error already reported by `verifyDeepWakeup`, disconnect SuzyQ and move on to
		// testing other wake sources.
		b.GpioApplyStrap(ctx, ti50.CcdDisconnected)
	}

	s.Log("Simulating serial console input")
	th.MustSucceed(b.WriteSerial(ctx, []byte("hello\r")), "Serial write")
	if verifyDeepWakeup(ctx, s, i, b, gpioMonitor, wakeSourceGpio, "console input") {
		verifyDeepSleep(ctx, s, i, th)
	}

	s.Log("Simulating EC_PACKET_MODE toggle")
	b.GpioSet(ctx, ti50.GpioTi50EcPacketMode, true)
	b.GpioSet(ctx, ti50.GpioTi50EcPacketMode, false)
	if verifyDeepWakeup(ctx, s, i, b, gpioMonitor, wakeSourceGpio, "EC_PACKET_MODE") {
		verifyDeepSleep(ctx, s, i, th)
	}

	s.Log("Simulating AP booting")
	b.GpioSet(ctx, ti50.GpioTi50PltRstL, true)
	if !verifyDeepWakeup(ctx, s, i, b, gpioMonitor, wakeSourceGpio, "PltRstL") {
		s.Fatal("Could not get Ti50 into 'AP on' mode, preventing further testing")
	}
	logCurrent(ctx, s, b, pv, "Awake_AP")

	s.Log("Waiting for sleep after boot")
	// Nominally, Ti50 should refrain from sleeping 60 seconds after AP boot, allow 15 seconds
	// either way, to account for test script execution delays.
	if err := i.WaitUntilAnySleep(ctx, 45*time.Second); err == nil {
		s.Error("Ti50 went to sleep too soon after AP boot")
	}
	th.MustSucceed(i.WaitUntilNormalSleep(ctx, 30*time.Second), "Sleep when AP on")
	logCurrent(ctx, s, b, pv, "NormalSleep")

	s.Log("Simulating AP TPM request")
	tpmHandle := b.Tpm(ctx, testParams.tpmCommunication)
	didVid := tpmHandle.ReadRegister(ti50.TpmRegDidVid)
	expectedDidVidValue := b.GscProperties().ExpectedDidVidValue()
	if !bytes.Equal(didVid, expectedDidVidValue) {
		s.Error("Unexpected TPM DID_VID immediately after wakeup: ", didVid)
	}
	if verifyNormalWakeup(ctx, s, i, b, gpioMonitor, wakeSourceGpio, "AP TPM request") {
		s.Log("Waiting for sleep with AP on")
		th.MustSucceed(i.WaitUntilNormalSleep(ctx, time.Minute), "Sleep when AP on")
	}

	s.Logf("Simulating EC packet mode, wait %s", waitForNoSleep)
	b.GpioSet(ctx, ti50.GpioTi50EcPacketMode, true)
	if verifyNormalWakeup(ctx, s, i, b, gpioMonitor, wakeSourceGpio, "EC packet mode") {
		if err := i.WaitUntilAnySleep(ctx, waitForNoSleep); err == nil {
			s.Error("Ti50 went to sleep while in EC packet mode")
		}

		// Having at this point seen that Ti50 stays awake for as long as EC keeps packet
		// mode asserted, we are ready to deassert the signal.  However, doing so seems to
		// currently cause Ti50 to immediately attempt to go to sleep, while still
		// printing a message about the packet mode signal having just changed, and that
		// confuses the test.  The line below triggers trivial activity, such that Ti50
		// will wait a number of seconds before going to sleep, sidestepping the issue.
		// TODO(b/317269418): We might want to have the GPIO edge count as "activity" for
		// the purposes of delaying sleep until a set number of seconds after last
		// activity.
		if _, err := i.Command(ctx, ""); err != nil {
			s.Error("Error poking Ti50")
		}

		b.GpioSet(ctx, ti50.GpioTi50EcPacketMode, false)
		s.Log("Waiting for sleep with AP on")
		th.MustSucceed(i.WaitUntilNormalSleep(ctx, time.Minute), "Sleep when AP on")
	} else {
		// Error already reported by `verifyNormalWakeup`, move on to testing other wake
		// sources.
		b.GpioSet(ctx, ti50.GpioTi50EcPacketMode, false)
	}

	s.Logf("Simulating CCD_MODE asserted, wait %s", waitForNoSleep)
	b.GpioSet(ctx, ti50.GpioTi50CcdModeL, false)
	if verifyNormalWakeup(ctx, s, i, b, gpioMonitor, wakeSourceGpio, "CCD_MODE asserted") {
		if err := i.WaitUntilAnySleep(ctx, waitForNoSleep); err == nil {
			s.Error("Ti50 went to sleep while CCD_MODE asserted")
		}
		b.GpioSet(ctx, ti50.GpioTi50CcdModeL, true)
		s.Log("Waiting for sleep with AP on")
		th.MustSucceed(i.WaitUntilNormalSleep(ctx, time.Minute), "Sleep when AP on")
	} else {
		// Error already reported by `verifyNormalWakeup`, move on to testing other wake
		// sources.
		b.GpioSet(ctx, ti50.GpioTi50CcdModeL, true)
	}

	s.Logf("Simulating SuzyQ inserted, wait %s", waitForNoSleep)
	b.GpioApplyStrap(ctx, ti50.CcdSuzyQ)
	if verifyNormalWakeup(ctx, s, i, b, gpioMonitor, wakeSourceAdc, "CCD connection") {
		logCurrent(ctx, s, b, pv, "Awake_AP_CCD")
		if err := i.WaitUntilAnySleep(ctx, waitForNoSleep); err == nil {
			s.Error("Ti50 went to sleep while SuzyQ connected")
		}
		b.GpioApplyStrap(ctx, ti50.CcdDisconnected)
		s.Log("Waiting for sleep with AP on")
		th.MustSucceed(i.WaitUntilNormalSleep(ctx, time.Minute), "Sleep when AP on")
		logCurrent(ctx, s, b, pv, "NormalSleep_CCD")
		// For some reason, after USB disconnect it takes five seconds for Dauntless power
		// consumption to drop.
		testing.Sleep(ctx, 5*time.Second) // GoBigSleepLint: Wait for power change
		logCurrent(ctx, s, b, pv, "NormalSleep_CCD_2")
	} else {
		// Error already reported by `verifyNormalWakeup`, move on to testing other wake
		// sources.
		b.GpioApplyStrap(ctx, ti50.CcdDisconnected)
	}

	s.Log("Simulating serial console input")
	th.MustSucceed(b.WriteSerial(ctx, []byte("hello\r")), "Serial write")
	if verifyNormalWakeup(ctx, s, i, b, gpioMonitor, wakeSourceGpio, "serial console input") {
		s.Log("Waiting for sleep with AP on")
		th.MustSucceed(i.WaitUntilNormalSleep(ctx, time.Minute), "Sleep when AP on")
	}

	s.Log("Simulating AP powering off")
	b.GpioSet(ctx, ti50.GpioTi50PltRstL, false)
	if !verifyNormalWakeup(ctx, s, i, b, gpioMonitor, wakeSourceGpio, "PltRstL") {
		s.Fatal("Could not get Ti50 out of 'AP on' mode, preventing further testing")
	}
	verifyDeepSleep(ctx, s, i, th)

	b.GpioMonitorFinish(ctx, gpioMonitor)

	if err := pv.Save(s.OutDir()); err != nil {
		s.Error("Failed to save perf data: ", err)
	}
}
