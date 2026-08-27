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

type sleepMode int

const (
	deepSleep sleepMode = iota
	normalSleep
)

type ti50SleepParam struct {
	sleepMode       sleepMode
	tpmBus          ti50.TpmBus
	servoMicroStrap ti50.GpioStrap
}

func init() {
	testing.AddTest(&testing.Test{
		Func:    Ti50Sleep,
		Desc:    "Test ti50 deep/normal sleep",
		Timeout: 40 * time.Minute,
		Contacts: []string{
			"cros-hwsec@google.com", // CrOS GSC Developers
			"jbk@chromium.org",      // Test Author
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr: []string{"group:gsc",
			"gsc_dt_ab", "gsc_dt_shield", "gsc_ot_fpga_cw310", "gsc_ot_shield",
			"gsc_image_ti50",
			"gsc_nightly"},
		Fixture: fixture.GSCOpenCCD,
		Vars:    []string{"bypass_sleep_check"},
		Params: []testing.Param{{
			Name: "deep_spi_no_uservo",
			Val: ti50SleepParam{
				sleepMode:       deepSleep,
				tpmBus:          ti50.TpmBusSpi,
				servoMicroStrap: ti50.ServoMicroDisconnected,
			},
		}, {
			Name: "deep_spi_uservo",
			Val: ti50SleepParam{
				sleepMode:       deepSleep,
				tpmBus:          ti50.TpmBusSpi,
				servoMicroStrap: ti50.ServoMicroConnected,
			},
		}, {
			Name: "deep_i2c_no_uservo",
			Val: ti50SleepParam{
				sleepMode:       deepSleep,
				tpmBus:          ti50.TpmBusI2c,
				servoMicroStrap: ti50.ServoMicroDisconnected,
			},
		}, {
			Name: "deep_i2c_uservo",
			Val: ti50SleepParam{
				sleepMode:       deepSleep,
				tpmBus:          ti50.TpmBusI2c,
				servoMicroStrap: ti50.ServoMicroConnected,
			},
		}, {
			Name: "normal_spi_no_uservo",
			Val: ti50SleepParam{
				sleepMode:       normalSleep,
				tpmBus:          ti50.TpmBusSpi,
				servoMicroStrap: ti50.ServoMicroDisconnected,
			},
		}, {
			Name: "normal_spi_uservo",
			Val: ti50SleepParam{
				sleepMode:       normalSleep,
				tpmBus:          ti50.TpmBusSpi,
				servoMicroStrap: ti50.ServoMicroConnected,
			},
		}, {
			Name: "normal_i2c_no_uservo",
			Val: ti50SleepParam{
				sleepMode:       normalSleep,
				tpmBus:          ti50.TpmBusI2c,
				servoMicroStrap: ti50.ServoMicroDisconnected,
			},
		}, {
			Name: "normal_i2c_uservo",
			Val: ti50SleepParam{
				sleepMode:       normalSleep,
				tpmBus:          ti50.TpmBusI2c,
				servoMicroStrap: ti50.ServoMicroConnected,
			},
		}},
	})
}

var (
	reBoot       *regexp.Regexp = regexp.MustCompile(`Ravn4|([0-9a-fA-F]{8})`)
	reResetType  *regexp.Regexp = regexp.MustCompile(`Reset Type: ([0-9a-zA-Z_]*)[\r\n]`)
	reWakeSource *regexp.Regexp = regexp.MustCompile(`Wake source: 0x([0-9a-fA-F]{8})`)
	reWhichPin   *regexp.Regexp = regexp.MustCompile(`Which pin: 0x([0-9a-zA-Z_]*)[\r\n]`)
	reConsole    *regexp.Regexp = regexp.MustCompile(`Console is enabled`)
	reAnyOutput  *regexp.Regexp = regexp.MustCompile(`\w+`)
	// Time to wait to ensure GSC did not go to sleep. This should be longer
	// than the longest sleep delay in hil/pmu.rs
	waitForNoSleep = 70 * time.Second
	waitForNoWake  = 3 * time.Second
)

type wakeSource string
type whichPin string

func logCurrent(ctx context.Context, s *testing.State, b utils.DevboardHelper, pv *perf.Values, label string) {
	if b.TestbedType == ti50.GscOpentitanCw310Fpga {
		// Current sensing not supported on FPGA
		return
	}
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
	s.Log("Waiting for deep sleep")
	th.MustSucceed(i.WaitUntilDeepSleep(ctx, ti50.WaitForSleepTimeout), "Did not deep sleep")
	verifyNoWake(ctx, s, i, th)
}

func verifyNormalSleep(ctx context.Context, s *testing.State, i *ti50.CrOSImage, th utils.FirmwareTestingHelper) {
	s.Log("Waiting for normal sleep")
	th.MustSucceed(i.WaitUntilNormalSleep(ctx, ti50.WaitForSleepTimeout), "Did not normal sleep")
	verifyNoWake(ctx, s, i, th)
}

func verifyNoWake(ctx context.Context, s *testing.State, i *ti50.CrOSImage, th utils.FirmwareTestingHelper) {
	s.Log("Check we stay asleep")
	if match, err := i.WaitUntilMatch(ctx, reAnyOutput, waitForNoWake); err == nil {
		s.Errorf("Did not stay asleep: %q", match[0])
	}
}

func verifyNoSleep(ctx context.Context, s *testing.State, i *ti50.CrOSImage, th utils.FirmwareTestingHelper) {
	s.Log("Check we stay awake")
	if err := i.WaitUntilAnySleep(ctx, waitForNoSleep); err == nil {
		s.Error("Did not stay awake")
	}
}

// verifyNormalWakeup verifies that Ti50 emits console output indicating wakeup from normal sleep.
// Aside from reporting any unexpected behavior through `s.Error`, the return value will be true
// only if Ti50 did wake up, which is useful for the top-level test script to know which state
// Ti50 is in, in case it wants to continue testing other aspects.
func verifyNormalWakeup(ctx context.Context, s *testing.State, i *ti50.CrOSImage, b utils.DevboardHelper, gpioMonitor utils.GpioMonitorSession, expected wakeSource, expectedGpio *whichPin, trigger string) bool {
	wakeMatch, err := i.WaitUntilMatch(ctx, reWakeSource, time.Second*3)
	if err != nil {
		s.Errorf("Waking on %s: Unable to recognize wake source", trigger)
		return false
	}
	if string(wakeMatch[1]) != string(expected) {
		s.Errorf("Waking on %s: Unexpected wake mask: got %s, expected %s", trigger, wakeMatch[1], expected)
	}
	// Verify GPIO wake source if specified
	if expectedGpio != nil {
		// Sent a newline without reading output to ensure that the which pin regex
		// will match since there is no guarantee anything will be written to the
		// GSC console after the which pin line, so there might not be a trailing
		// newline to match (and we need that newline to bound the hex string).
		if err := i.WriteSerial(ctx, []byte("\n")); err != nil {
			s.Fatal("Could not write to serial: ", err)
			return false
		}
		whichPinMatch, err := i.WaitUntilMatch(ctx, reWhichPin, time.Second*3)
		if err != nil {
			s.Errorf("Waking on %s: Unable to recognize which pin source", trigger)
			return true
		}
		if string(whichPinMatch[1]) != string(*expectedGpio) {
			s.Errorf("Waking on %s: Unexpected which pin mask: got %s, expected %s", trigger, whichPinMatch[1], *expectedGpio)
		}
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
func verifyDeepWakeup(ctx context.Context, s *testing.State, i *ti50.CrOSImage, b utils.DevboardHelper, gpioMonitor utils.GpioMonitorSession, expectedWake wakeSource, expectedGpio *whichPin, trigger string) bool {
	_, err := i.WaitUntilMatch(ctx, reBoot, time.Second*3)
	if err != nil {
		s.Error("Ti50 did not wake up by ", trigger)
		return false
	}
	resetMatch, err := i.WaitUntilMatch(ctx, reResetType, time.Second*3)
	if err != nil {
		s.Errorf("Waking on %s: did not recognize reset type", trigger)
		return true
	}
	if string(resetMatch[1]) == "Wake" {
		wakeMatch, err := i.WaitUntilMatch(ctx, reWakeSource, time.Second*3)
		if err != nil {
			s.Errorf("Waking on %s: Unable to recognize wake source", trigger)
			return true
		}
		if string(wakeMatch[1]) != string(expectedWake) {
			s.Errorf("Waking on %s: Unexpected wake mask: got %s, expected %s", trigger, wakeMatch[1], expectedWake)
		}

		// Verify GPIO wake source if specified
		if expectedGpio != nil {
			whichPinMatch, err := i.WaitUntilMatch(ctx, reWhichPin, time.Second*3)
			if err != nil {
				s.Errorf("Waking on %s: Unable to recognize which pin source", trigger)
				return true
			}
			if string(whichPinMatch[1]) != string(*expectedGpio) {
				s.Errorf("Waking on %s: Unexpected which pin mask: got %s, expected %s", trigger, whichPinMatch[1], *expectedGpio)
			}
		}

	} else {
		s.Errorf("Waking on %s: Unexpected reset type: got %q, expected %q", trigger, string(resetMatch[1]), "Wake")
	}
	_, err = i.WaitUntilMatch(ctx, reConsole, time.Second*3)
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

// Wake source and pin values for DT chip.
var (
	wakeSourceGpio wakeSource = "00000001"
	wakeSourceRbox wakeSource = "00000004"
	wakeSourceAdc  wakeSource = "00000008"

	whichPinWPSense      whichPin = "0000000000000004"
	whichPinPltRstL      whichPin = "0000000000000800"
	whichPinEcPacketMode whichPin = "0000000001000000"
	whichPinLidOpen      whichPin = "0000000002000000"
	whichPinGscUart      whichPin = "0000010000000000"
	whichPinCcdMode      whichPin = "0000080000000000"

	whichPinTpmSpi whichPin = "0200000000000000"
	whichPinTpmI2C whichPin = "0000000000000003"
)

func Ti50Sleep(ctx context.Context, s *testing.State) {
	// Check if runtime bypass_sleep_check var is present, if so (even if value is
	// "false") set the 70 seconds wait to 1 second the rest of the test
	if _, fastSleep := s.Var("bypass_sleep_check"); fastSleep {
		s.Log("Wait for no sleep reduced from 70 seconds to 1 second")
		waitForNoSleep = 1 * time.Second
	}

	testParams := s.Param().(ti50SleepParam)
	b := utils.NewDevboardHelper(s, s.FixtValue().(*fixture.Value))
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}
	i := ti50.MustOpenCrOSImage(ctx, b, s, b.TestbedType)
	defer i.Close(ctx)
	pv := perf.NewValues()

	// Wake source and pin values for OT chip.
	if b.GscProperties().ChipType() == ti50.GscOT {
		wakeSourceGpio = "00000004"
		wakeSourceRbox = "00000001"
		wakeSourceAdc = "00000002"

		whichPinWPSense = "0000000000000020"
		whichPinPltRstL = "0000000000000001"
		whichPinEcPacketMode = "0000000000000010"
		whichPinLidOpen = "0000000000000002"
		whichPinGscUart = "0000000000000008"
		whichPinCcdMode = "0000000000000004"

		whichPinTpmSpi = "0000000000000040"
		whichPinTpmI2C = "00000000000000c0"
	}

	var tpmStrap ti50.GpioStrap
	if testParams.tpmBus == ti50.TpmBusI2c {
		tpmStrap = ti50.TpmI2c
	} else if testParams.tpmBus == ti50.TpmBusSpi {
		tpmStrap = ti50.TpmSpi
	}

	s.Log("Restarting ti50 with appropriate straps")
	b.GpioSet(ctx, ti50.GpioTi50PltRstL, false)
	b.GpioSet(ctx, ti50.GpioTi50LidOpen, false)
	b.GpioSet(ctx, ti50.GpioTi50EcRstL, true)
	b.GpioSet(ctx, ti50.GpioTi50EcPacketMode, false)
	b.GpioSet(ctx, ti50.GpioTi50CcdModeL, true)
	b.GpioSet(ctx, ti50.GpioTi50WriteProtectSenseL, false)
	b.GpioSet(ctx, ti50.GpioTi50ACPresent, false)
	b.ResetWithStraps(ctx, testParams.servoMicroStrap, ti50.CCDModeOff, tpmStrap)
	th.MustSucceed(i.WaitUntilBooted(ctx), "Ti50 revives after reboot")
	var gpioMonitor utils.GpioMonitorSession
	if b.GscProperties().HasEcRstFet() {
		gpioMonitor = b.GpioMonitorStart(ctx, ti50.GpioTi50EcRstL, ti50.GpioTi50EcRstFet)
	} else {
		gpioMonitor = b.GpioMonitorStart(ctx, ti50.GpioTi50EcRstL)
	}

	if testParams.sleepMode == deepSleep {
		ti50DeepSleep(ctx, s, b, i, th, gpioMonitor, pv)
	} else {
		ti50NormalSleep(ctx, s, b, i, th, gpioMonitor, pv, testParams.tpmBus)
	}

	b.GpioMonitorFinish(ctx, gpioMonitor)

	if err := pv.Save(s.OutDir()); err != nil {
		s.Error("Failed to save perf data: ", err)
	}
}

func ti50DeepSleep(ctx context.Context, s *testing.State, b utils.DevboardHelper, i *ti50.CrOSImage, th utils.FirmwareTestingHelper, gpioMonitor utils.GpioMonitorSession, pv *perf.Values) {

	logCurrent(ctx, s, b, pv, "Awake")
	verifyDeepSleep(ctx, s, i, th)
	logCurrent(ctx, s, b, pv, "DeepSleep")

	s.Log("Simulating power button press")
	b.GpioSet(ctx, ti50.GpioTi50PowerBtnL, false)
	b.GpioSet(ctx, ti50.GpioTi50PowerBtnL, true)
	if verifyDeepWakeup(ctx, s, i, b, gpioMonitor, wakeSourceRbox, nil, "Power Button") {
		verifyDeepSleep(ctx, s, i, th)
	}

	// OpenTitan must wake and then deep sleep when AC present toggles
	if b.GscProperties().ChipType() == ti50.GscOT {
		s.Log("Simulating AC present")
		b.GpioSet(ctx, ti50.GpioTi50ACPresent, true)
		if verifyDeepWakeup(ctx, s, i, b, gpioMonitor, wakeSourceRbox, nil, "AC present") {
			verifyDeepSleep(ctx, s, i, th)
		}
		s.Log("Simulating AC not present")
		b.GpioSet(ctx, ti50.GpioTi50ACPresent, false)
		if verifyDeepWakeup(ctx, s, i, b, gpioMonitor, wakeSourceRbox, nil, "AC not present") {
			verifyDeepSleep(ctx, s, i, th)
		}
	}

	s.Log("Simulating lid low-to-high event")
	b.GpioSet(ctx, ti50.GpioTi50LidOpen, true)
	if verifyDeepWakeup(ctx, s, i, b, gpioMonitor, wakeSourceGpio, &whichPinLidOpen, "Lid low-to-high event") {
		verifyDeepSleep(ctx, s, i, th)
	}

	s.Log("Simulating EC packet mode")
	b.GpioSet(ctx, ti50.GpioTi50EcPacketMode, true)
	if verifyDeepWakeup(ctx, s, i, b, gpioMonitor, wakeSourceGpio, &whichPinEcPacketMode, "EC packet mode") {
		verifyNoSleep(ctx, s, i, th)
		b.GpioSet(ctx, ti50.GpioTi50EcPacketMode, false)
		verifyDeepSleep(ctx, s, i, th)
	}

	s.Log("Simulating CCD_MODE asserted")
	b.GpioSet(ctx, ti50.GpioTi50CcdModeL, false)
	if verifyDeepWakeup(ctx, s, i, b, gpioMonitor, wakeSourceGpio, &whichPinCcdMode, "CCD_MODE asserted") {
		verifyNoSleep(ctx, s, i, th)
		b.GpioSet(ctx, ti50.GpioTi50CcdModeL, true)
		verifyDeepSleep(ctx, s, i, th)
	}

	s.Log("Simulating WP_SENSE_L de-assert pulse event")
	b.GpioSet(ctx, ti50.GpioTi50WriteProtectSenseL, true)
	b.GpioSet(ctx, ti50.GpioTi50WriteProtectSenseL, false)
	if verifyDeepWakeup(ctx, s, i, b, gpioMonitor, wakeSourceGpio, &whichPinWPSense, "WP_SENSE_L de-assert event") {
		verifyDeepSleep(ctx, s, i, th)
	}

	if b.TestbedType == ti50.GscOpentitanCw310Fpga {
		// Analog voltages on CC lines not supported on FPGA
	} else {
		s.Log("Simulating SuzyQ inserted")
		b.GpioApplyStrap(ctx, ti50.CcdSuzyQ)
		if verifyDeepWakeup(ctx, s, i, b, gpioMonitor, wakeSourceAdc, nil, "CCD connection") {
			logCurrent(ctx, s, b, pv, "Awake_CCD")
			verifyNoSleep(ctx, s, i, th)
			b.GpioApplyStrap(ctx, ti50.CcdDisconnected)
			verifyDeepSleep(ctx, s, i, th)
			logCurrent(ctx, s, b, pv, "DeepSleep_CCD")
		} else {
			// Error already reported by `verifyDeepWakeup`, disconnect SuzyQ and move on to
			// testing other wake sources.
			b.GpioApplyStrap(ctx, ti50.CcdDisconnected)
		}
	}

	s.Log("Simulating serial console input")
	th.MustSucceed(b.WriteSerial(ctx, []byte("hello\r")), "Serial write")
	if verifyDeepWakeup(ctx, s, i, b, gpioMonitor, wakeSourceGpio, &whichPinGscUart, "console input") {
		verifyDeepSleep(ctx, s, i, th)
	}

	s.Log("Simulating EC_PACKET_MODE toggle")
	b.GpioSet(ctx, ti50.GpioTi50EcPacketMode, true)
	b.GpioSet(ctx, ti50.GpioTi50EcPacketMode, false)
	if verifyDeepWakeup(ctx, s, i, b, gpioMonitor, wakeSourceGpio, &whichPinEcPacketMode, "EC_PACKET_MODE") {
		verifyDeepSleep(ctx, s, i, th)
	}

	s.Log("Setting WP enabled at boot to prevent wp dirty state. Reset GSC to clear dirty WP state")
	// Pause the gpio monitor since we are about to reboot the GSC on purpose.
	th.MustSucceed(i.WaitUntilBooted(ctx), "Ti50 revives after reboot")
	th.MustSucceed(i.TestlabOpen(ctx), "testlab open")
	th.MustSucceed(i.SetWpAtBoot(ctx, true), "Enable WP at boot")
	th.MustSucceed(i.SendConsoleRebootCmd(ctx), "GSC reboot")
	th.MustSucceed(i.WaitUntilBooted(ctx), "Ti50 revives after reboot")
	// Clear the last gpio monitor events since we expect GSC to reset EC from above commands
	b.GpioMonitorRead(ctx, gpioMonitor)
	verifyDeepSleep(ctx, s, i, th)

	s.Log("Simulating AP booting")
	b.GpioSet(ctx, ti50.GpioTi50PltRstL, true)
	if !verifyDeepWakeup(ctx, s, i, b, gpioMonitor, wakeSourceGpio, &whichPinPltRstL, "PltRstL") {
		s.Fatal("Could not get Ti50 into 'AP on' mode, preventing further testing")
	}
}

func ti50NormalSleep(ctx context.Context, s *testing.State, b utils.DevboardHelper, i *ti50.CrOSImage, th utils.FirmwareTestingHelper, gpioMonitor utils.GpioMonitorSession, pv *perf.Values, tpmBus ti50.TpmBus) {

	b.GpioSet(ctx, ti50.GpioTi50PltRstL, true)
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
	tpmHandle := b.Tpm(ctx, tpmBus)
	// The GSC may not wake up quickly enough to be able to properly respond to the first SPI
	// request, make up to three attempts.
	for attempt := 1; ; attempt++ {
		didVid, err := tpmHandle.OpenTitanToolTpmCommand("read-register", string(ti50.TpmRegDidVid))
		if err != nil {
			if attempt < 3 {
				continue
			} else {
				s.Fatalf("Failed to read TPM register immediately after wakeup: %s", err)
			}
		}
		expectedDidVidValue := b.GscProperties().ExpectedDidVidValue()
		if !bytes.Equal(didVid, expectedDidVidValue) {
			s.Error("Unexpected TPM DID_VID immediately after wakeup: ", didVid)
		}
		break
	}
	var whichPinTpmBus whichPin
	if tpmBus == ti50.TpmBusI2c {
		whichPinTpmBus = whichPinTpmI2C
	} else if tpmBus == ti50.TpmBusSpi {
		whichPinTpmBus = whichPinTpmSpi
	}
	if verifyNormalWakeup(ctx, s, i, b, gpioMonitor, wakeSourceGpio, &whichPinTpmBus, "AP TPM request") {
		verifyNormalSleep(ctx, s, i, th)
	}

	s.Log("Simulating EC packet mode")
	b.GpioSet(ctx, ti50.GpioTi50EcPacketMode, true)
	if verifyNormalWakeup(ctx, s, i, b, gpioMonitor, wakeSourceGpio, &whichPinEcPacketMode, "EC packet mode") {
		verifyNoSleep(ctx, s, i, th)
		b.GpioSet(ctx, ti50.GpioTi50EcPacketMode, false)
		verifyNormalSleep(ctx, s, i, th)
	} else {
		// Error already reported by `verifyNormalWakeup`, move on to testing other wake sources.
		b.GpioSet(ctx, ti50.GpioTi50EcPacketMode, false)
	}

	s.Log("Simulating power button pressed")
	b.GpioSet(ctx, ti50.GpioTi50PowerBtnL, false)
	if verifyNormalWakeup(ctx, s, i, b, gpioMonitor, wakeSourceRbox, nil, "Power button") {
		verifyNoSleep(ctx, s, i, th)
		b.GpioSet(ctx, ti50.GpioTi50PowerBtnL, true)
		verifyNormalSleep(ctx, s, i, th)
	} else {
		// Error already reported by `verifyNormalWakeup`, move on to testing other wake sources.
		b.GpioSet(ctx, ti50.GpioTi50PowerBtnL, true)
	}

	s.Log("Simulating CCD_MODE asserted")
	b.GpioSet(ctx, ti50.GpioTi50CcdModeL, false)
	if verifyNormalWakeup(ctx, s, i, b, gpioMonitor, wakeSourceGpio, &whichPinCcdMode, "CCD_MODE asserted") {
		verifyNoSleep(ctx, s, i, th)
		b.GpioSet(ctx, ti50.GpioTi50CcdModeL, true)
		verifyNormalSleep(ctx, s, i, th)
	} else {
		// Error already reported by `verifyNormalWakeup`, move on to testing other wake sources.
		b.GpioSet(ctx, ti50.GpioTi50CcdModeL, true)
	}

	if b.TestbedType == ti50.GscOpentitanCw310Fpga {
		// Analog voltages on CC lines not supported on FPGA
	} else {
		s.Log("Simulating SuzyQ inserted")
		b.GpioApplyStrap(ctx, ti50.CcdSuzyQ)
		if verifyNormalWakeup(ctx, s, i, b, gpioMonitor, wakeSourceAdc, nil, "CCD connection") {
			logCurrent(ctx, s, b, pv, "Awake_AP_CCD")
			verifyNoSleep(ctx, s, i, th)
			b.GpioApplyStrap(ctx, ti50.CcdDisconnected)
			verifyNormalSleep(ctx, s, i, th)
			logCurrent(ctx, s, b, pv, "NormalSleep_CCD")
			// For some reason, after USB disconnect it takes five seconds for Dauntless power
			// consumption to drop.
			testing.Sleep(ctx, 5*time.Second) // GoBigSleepLint: Wait for power change
			logCurrent(ctx, s, b, pv, "NormalSleep_CCD_2")
		} else {
			// Error already reported by `verifyNormalWakeup`, move on to testing other wake sources.
			b.GpioApplyStrap(ctx, ti50.CcdDisconnected)
		}
	}

	s.Log("Simulating serial console input")
	th.MustSucceed(b.WriteSerial(ctx, []byte("hello\r")), "Serial write")
	if verifyNormalWakeup(ctx, s, i, b, gpioMonitor, wakeSourceGpio, &whichPinGscUart, "serial console input") {
		verifyNormalSleep(ctx, s, i, th)
	}

	s.Log("Simulating WP_SENSE_L de-assert pulse event")
	b.GpioSet(ctx, ti50.GpioTi50WriteProtectSenseL, true)
	b.GpioSet(ctx, ti50.GpioTi50WriteProtectSenseL, false)
	verifyNormalWakeup(ctx, s, i, b, gpioMonitor, wakeSourceGpio, &whichPinWPSense, "WP_SENSE_L pulse")
	// Need to clear an WP dirty state that would cause extra GSC reboots
	th.MustSucceed(i.SendConsoleRebootCmd(ctx), "GSC reboot")
	th.MustSucceed(i.WaitUntilBooted(ctx), "Ti50 revives after reboot")
	// Clear the last gpio monitor events since we expect GSC to reset EC from above commands
	b.GpioMonitorRead(ctx, gpioMonitor)
	verifyNormalSleep(ctx, s, i, th)

	s.Log("Simulating AP powering off")
	b.GpioSet(ctx, ti50.GpioTi50PltRstL, false)
	if !verifyNormalWakeup(ctx, s, i, b, gpioMonitor, wakeSourceGpio, &whichPinPltRstL, "PltRstL") {
		s.Fatal("Could not get Ti50 out of 'AP on' mode, preventing further testing")
	}
	verifyDeepSleep(ctx, s, i, th)
}
