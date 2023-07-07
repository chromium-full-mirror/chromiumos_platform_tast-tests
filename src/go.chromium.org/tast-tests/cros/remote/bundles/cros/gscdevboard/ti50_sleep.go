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
		Attr:         []string{"group:gsc", "gsc_dt_ab", "gsc_image_ti50", "gsc_nightly"},
		Fixture:      fixture.Ti50CcdOpen,
	})
}

var (
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

func verifyWakeup(ctx context.Context, s *testing.State, i *ti50.CrOSImage, board ti50.DevBoard, expected, trigger string) {
	wakeMatch, err := board.ReadSerialSubmatch(ctx, reWakeSource)
	if err != nil {
		s.Error("Ti50 did not wake up by ", trigger)
		return
	}
	if string(wakeMatch[1]) != expected {
		s.Error("Unexpected wake mask ", wakeMatch[1], " after ", trigger)
	}
	if err := i.WaitUntilBooted(ctx); err != nil {
		s.Error("Ti50 did not wake up by ", trigger)
	}
}

func Ti50Sleep(ctx context.Context, s *testing.State) {
	f := s.FixtValue().(*fixture.Value)
	b := utils.NewDevboardHelper(f.DevBoard(), s)
	i := ti50.NewCrOSImage(b)
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}

	s.Log("Restarting ti50 with SPI straps")
	b.GpioApplyStrap(ctx, ti50.CcdDisconnected)
	b.GpioApplyStrap(ctx, ti50.TpmSpi)
	b.GpioSet(ctx, ti50.GpioTi50PltRstL, false)
	th.MustSucceed(b.Reset(ctx), "Reset board")
	th.MustSucceed(i.WaitUntilBooted(ctx), "Ti50 revives after reboot")

	s.Log("Waiting for sleep with AP off")
	th.MustSucceed(i.WaitUntilDeepSleep(ctx, time.Minute), "Ti50 did not sleep when AP off")

	s.Log("Simulating power button press")
	b.GpioSet(ctx, ti50.GpioTi50PowerBtnL, false)
	b.GpioSet(ctx, ti50.GpioTi50PowerBtnL, true)
	verifyWakeup(ctx, s, i, b, wakeSourceRbox, "Power Button")
	s.Log("Waiting for sleep with AP off")
	th.MustSucceed(i.WaitUntilDeepSleep(ctx, time.Minute), "Ti50 did not sleep when AP off")

	s.Log("Simulating SuzyQ inserted, wait 3 minutes")
	b.GpioApplyStrap(ctx, ti50.CcdSuzyQ)
	verifyWakeup(ctx, s, i, b, wakeSourceAdc, "CCD connection")
	if err := i.WaitUntilAnySleep(ctx, 3*time.Minute); err == nil {
		s.Error("Ti50 went to sleep while SuzyQ connected")
	}
	b.GpioApplyStrap(ctx, ti50.CcdDisconnected)
	s.Log("Waiting for sleep with AP off")
	th.MustSucceed(i.WaitUntilDeepSleep(ctx, time.Minute), "Ti50 did not sleep when AP off")

	s.Log("Simulating serial console input")
	th.MustSucceed(b.WriteSerial(ctx, []byte("hello\r")), "Serial write")
	verifyWakeup(ctx, s, i, b, wakeSourceGpio, "console input")
	s.Log("Waiting for sleep with AP off")
	th.MustSucceed(i.WaitUntilDeepSleep(ctx, time.Minute), "Ti50 did not sleep when AP off")

	s.Log("Simulating EC_PACKET_MODE toggle")
	b.GpioSet(ctx, ti50.GpioTi50EcPacketMode, true)
	b.GpioSet(ctx, ti50.GpioTi50EcPacketMode, false)
	verifyWakeup(ctx, s, i, b, wakeSourceGpio, "EC_PACKET_MODE")
	s.Log("Waiting for sleep with AP off")
	th.MustSucceed(i.WaitUntilDeepSleep(ctx, time.Minute), "Ti50 did not sleep when AP off")

	s.Log("Simulating AP booting")
	b.GpioSet(ctx, ti50.GpioTi50PltRstL, true)
	verifyWakeup(ctx, s, i, b, wakeSourceGpio, "PltRstL")

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
	if !bytes.Equal(didVid, ti50.TpmDidVidValue) {
		s.Error("Unexpected TPM DID_VID immediately after wakeup: ", didVid)
	}
	verifyWakeup(ctx, s, i, b, wakeSourceGpio, "SPI TPM request")
	th.MustSucceed(i.WaitUntilNormalSleep(ctx, time.Minute), "Sleep when AP on")

	s.Log("Simulating SuzyQ inserted")
	b.GpioApplyStrap(ctx, ti50.CcdSuzyQ)
	verifyWakeup(ctx, s, i, b, wakeSourceAdc, "CCD connection")
	b.GpioApplyStrap(ctx, ti50.CcdDisconnected)
	s.Log("Waiting for sleep with AP on")
	th.MustSucceed(i.WaitUntilNormalSleep(ctx, time.Minute), "Sleep when AP on")

	s.Log("Simulating serial console input")
	th.MustSucceed(b.WriteSerial(ctx, []byte("hello\r")), "Serial write")
	verifyWakeup(ctx, s, i, b, wakeSourceGpio, "serial console input")
	s.Log("Waiting for sleep with AP on")
	th.MustSucceed(i.WaitUntilNormalSleep(ctx, time.Minute), "Sleep when AP on")
}
