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
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/gscdevboard/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"

	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:    Ti50Sleep,
		Desc:    "Test ti50 deep/normal sleep on Andreiboard connected to devboardsvc host",
		Timeout: 10 * time.Minute,
		Contacts: []string{
			"chromeos-faft@google.com", // CrOS Firmware Developers
			"jbk@chromium.org",         // Test Author
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr:         []string{"group:gsc", "gsc_dt_ab", "gsc_dt_shield", "gsc_image_ti50", "gsc_nightly"},
		Fixture:      fixture.Ti50CcdOpen,
		Params: []testing.Param{{
			Name: "no_uservo",
			Val:  noServoMicro,
		}, {
			Name: "uservo",
			Val:  servoMicro,
		}},
	})
}

var (
	reBoot       *regexp.Regexp = regexp.MustCompile(`Ravn4|([0-9a-fA-F]{8})`)
	reResetType  *regexp.Regexp = regexp.MustCompile(`Reset Type: ([0-9a-zA-Z_]*)[\r\n]`)
	reWakeSource *regexp.Regexp = regexp.MustCompile(`Wake source: 0x([0-9a-fA-F]{8})`)
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

func verifyStaysAsleep(ctx context.Context, s *testing.State, b utils.DevboardHelper) {
	//waitCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	//defer cancel()
	_, err := b.ReadSerialSubmatch(ctx, reBoot)
	if err != nil {
		// We expected to NOT see boot a message.
		return
	}
	s.Error("Ti50 did not stay asleep")
}

func verifyNormalWakeup(ctx context.Context, s *testing.State, i *ti50.CrOSImage, b utils.DevboardHelper, gpioMonitor utils.GpioMonitorSession, expected, trigger string) {
	wakeMatch, err := b.ReadSerialSubmatch(ctx, reWakeSource)
	if err != nil {
		s.Errorf("Waking on %s: Unable to recognize wake source", trigger)
		return
	}
	if string(wakeMatch[1]) != expected {
		s.Errorf("Waking on %s: Unexpected wake mask %s", trigger, wakeMatch[1])
	}
	if err := i.WaitUntilBooted(ctx); err != nil {
		s.Error("Ti50 did not wake up by ", trigger)
	}
	events := b.GpioMonitorRead(ctx, gpioMonitor)
	if len(events.Sorted) != 0 {
		s.Errorf("Waking on %s: Unexpected gpio events: %s", trigger, events)
	}
}

func verifyDeepWakeup(ctx context.Context, s *testing.State, i *ti50.CrOSImage, b utils.DevboardHelper, gpioMonitor utils.GpioMonitorSession, expected, trigger string) {
	_, err := b.ReadSerialSubmatch(ctx, reBoot)
	if err != nil {
		s.Error("Ti50 did not wake up by ", trigger)
		return
	}
	resetMatch, err := b.ReadSerialSubmatch(ctx, reResetType)
	if err != nil {
		s.Errorf("Waking on %s: did not recognize reset type", trigger)
		return
	}
	if string(resetMatch[1]) == "Wake" {
		wakeMatch, err := b.ReadSerialSubmatch(ctx, reWakeSource)
		if err != nil {
			s.Errorf("Waking on %s: Unable to recognize wake source", trigger)
			return
		}
		if string(wakeMatch[1]) != expected {
			s.Errorf("Waking on %s: Unexpected wake mask %s", trigger, wakeMatch[1])
		}
	} else {
		s.Errorf("Waking on %s: Unexpected reset type: %s", trigger, string(resetMatch[1]))
	}
	if err := i.WaitUntilBooted(ctx); err != nil {
		s.Error("Ti50 did not wake up by ", trigger)
	}
	events := b.GpioMonitorRead(ctx, gpioMonitor)
	if len(events.Sorted) != 0 {
		s.Errorf("Waking on %s: Unexpected gpio events: %s", trigger, events)
	}
}

func servoMicro(ctx context.Context, b utils.DevboardHelper) {
	b.GpioApplyStrap(ctx, ti50.ServoMicroConnected)
}

func noServoMicro(ctx context.Context, b utils.DevboardHelper) {
	b.GpioApplyStrap(ctx, ti50.ServoMicroDisconnected)
}

func Ti50Sleep(ctx context.Context, s *testing.State) {
	f := s.FixtValue().(*fixture.Value)
	b := utils.NewDevboardHelper(f, s)
	i := ti50.NewCrOSImage(b)
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}

	setup := s.Param().(func(context.Context, utils.DevboardHelper))
	setup(ctx, b)

	s.Log("Restarting ti50 with SPI straps")
	b.GpioApplyStrap(ctx, ti50.CcdDisconnected)
	b.GpioApplyStrap(ctx, ti50.TpmSpi)
	b.GpioSet(ctx, ti50.GpioTi50PltRstL, false)
	th.MustSucceed(b.Reset(ctx), "Reset board")
	th.MustSucceed(i.WaitUntilBooted(ctx), "Ti50 revives after reboot")
	gpioMonitor := b.GpioMonitorStart(ctx, ti50.GpioTi50EcRstL, ti50.GpioTi50EcRstFet)

	s.Log("Waiting for sleep with AP off")
	th.MustSucceed(i.WaitUntilDeepSleep(ctx, time.Minute), "Ti50 did not sleep when AP off")
	verifyStaysAsleep(ctx, s, b)
	s.Log("Simulating power button press")
	b.GpioSet(ctx, ti50.GpioTi50PowerBtnL, false)
	b.GpioSet(ctx, ti50.GpioTi50PowerBtnL, true)
	verifyDeepWakeup(ctx, s, i, b, gpioMonitor, wakeSourceRbox, "Power Button")
	s.Log("Waiting for sleep with AP off")
	th.MustSucceed(i.WaitUntilDeepSleep(ctx, time.Minute), "Ti50 did not sleep when AP off")
	verifyStaysAsleep(ctx, s, b)

	s.Log("Simulating SuzyQ inserted, wait 3 minutes")
	b.GpioApplyStrap(ctx, ti50.CcdSuzyQ)
	verifyDeepWakeup(ctx, s, i, b, gpioMonitor, wakeSourceAdc, "CCD connection")
	if err := i.WaitUntilAnySleep(ctx, 3*time.Minute); err == nil {
		s.Error("Ti50 went to sleep while SuzyQ connected")
	}
	b.GpioApplyStrap(ctx, ti50.CcdDisconnected)
	s.Log("Waiting for sleep with AP off")
	th.MustSucceed(i.WaitUntilDeepSleep(ctx, time.Minute), "Ti50 did not sleep when AP off")
	verifyStaysAsleep(ctx, s, b)
	s.Log("Simulating serial console input")
	th.MustSucceed(b.WriteSerial(ctx, []byte("hello\r")), "Serial write")
	verifyDeepWakeup(ctx, s, i, b, gpioMonitor, wakeSourceGpio, "console input")
	s.Log("Waiting for sleep with AP off")
	th.MustSucceed(i.WaitUntilDeepSleep(ctx, time.Minute), "Ti50 did not sleep when AP off")
	verifyStaysAsleep(ctx, s, b)

	s.Log("Simulating EC_PACKET_MODE toggle")
	b.GpioSet(ctx, ti50.GpioTi50EcPacketMode, true)
	b.GpioSet(ctx, ti50.GpioTi50EcPacketMode, false)
	verifyDeepWakeup(ctx, s, i, b, gpioMonitor, wakeSourceGpio, "EC_PACKET_MODE")
	s.Log("Waiting for sleep with AP off")
	th.MustSucceed(i.WaitUntilDeepSleep(ctx, time.Minute), "Ti50 did not sleep when AP off")

	s.Log("Simulating AP booting")
	b.GpioSet(ctx, ti50.GpioTi50PltRstL, true)
	verifyDeepWakeup(ctx, s, i, b, gpioMonitor, wakeSourceGpio, "PltRstL")

	s.Log("Waiting for sleep after boot")
	// Nominally, Ti50 should refrain from sleeping 60 seconds after AP boot, allow 15 seconds
	// either way, to account for test script execution delays.
	if err := i.WaitUntilAnySleep(ctx, 45*time.Second); err == nil {
		s.Error("Ti50 went to sleep too soon after AP boot")
	}
	th.MustSucceed(i.WaitUntilNormalSleep(ctx, 30*time.Second), "Sleep when AP on")

	s.Log("Simulating AP SPI request")
	tpmHandle := b.Tpm(ctx, ti50.TpmBusSpi)
	didVid := tpmHandle.ReadRegister(ti50.TpmRegDidVid)
	expectedDidVidValue := b.ExpectedDidVidValue(ctx)
	if !bytes.Equal(didVid, expectedDidVidValue) {
		s.Error("Unexpected TPM DID_VID immediately after wakeup: ", didVid)
	}
	verifyNormalWakeup(ctx, s, i, b, gpioMonitor, wakeSourceGpio, "SPI TPM request")
	th.MustSucceed(i.WaitUntilNormalSleep(ctx, time.Minute), "Sleep when AP on")

	s.Log("Simulating SuzyQ inserted")
	b.GpioApplyStrap(ctx, ti50.CcdSuzyQ)
	verifyNormalWakeup(ctx, s, i, b, gpioMonitor, wakeSourceAdc, "CCD connection")
	b.GpioApplyStrap(ctx, ti50.CcdDisconnected)
	s.Log("Waiting for sleep with AP on")
	th.MustSucceed(i.WaitUntilNormalSleep(ctx, time.Minute), "Sleep when AP on")

	s.Log("Simulating serial console input")
	th.MustSucceed(b.WriteSerial(ctx, []byte("hello\r")), "Serial write")
	verifyNormalWakeup(ctx, s, i, b, gpioMonitor, wakeSourceGpio, "serial console input")
	s.Log("Waiting for sleep with AP on")
	th.MustSucceed(i.WaitUntilNormalSleep(ctx, time.Minute), "Sleep when AP on")
	b.GpioMonitorFinish(ctx, gpioMonitor)
}
