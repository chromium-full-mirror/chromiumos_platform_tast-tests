// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast-tests/cros/remote/firmware/reporters"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         WilcoNormalToDev,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify wilco transitions from normal to dev using ctrld and power button",
		Contacts: []string{
			"chromeos-faft@google.com",
			"cienet-firmware@cienet.corp-partner.google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		// TODO: When stable, change firmware_unstable to a different attr.
		Attr:         []string{"group:firmware", "firmware_unstable"},
		SoftwareDeps: []string{"wilco"},
		Fixture:      fixture.NormalMode,
		Timeout:      20 * time.Minute,
	})
}

func WilcoNormalToDev(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}
	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to get config: ", err)
	}

	s.Log("Connecting charger")
	if err := h.SetDUTPower(ctx, true); err != nil {
		s.Fatal("Unable to connect charger: ", err)
	}
	if err := h.CheckChgFrmPwrSuppInfo(ctx, true); err != nil {
		s.Fatal("Failed to check charger: ", err)
	}

	defer func() {
		// By default, the usb mux state is set as 'dut_sees_usbkey'.
		// Resume this state before the test ends.
		if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxDUT); err != nil {
			s.Fatal("Failed to power off the USB: ", err)
		}
	}()
	s.Log("Powering off the USB")
	if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxOff); err != nil {
		s.Fatal("Failed to power off the USB: ", err)
	}

	s.Log("Rebooting the DUT into recovery screen")
	if err := h.Servo.SetPowerState(ctx, servo.PowerStateRec); err != nil {
		s.Fatal("Failed to enter recovery mode: ", err)
	}
	s.Logf("Sleeping %s (FirmwareScreen)", h.Config.FirmwareScreen)
	// GoBigSleepLint: Sleeping for model specific time.
	if err := testing.Sleep(ctx, h.Config.FirmwareScreen); err != nil {
		s.Fatal("Failed to sleep: ", err)
	}

	s.Log("Pressing Ctrl-D")
	if err := h.Servo.KeypressWithDuration(ctx, servo.CtrlD, servo.DurTab); err != nil {
		s.Fatal("Failed to press ctrl-d: ", err)
	}
	s.Logf("Sleeping %s (KeypressDelay)", h.Config.KeypressDelay)
	// GoBigSleepLint: Sleeping for model specific time.
	if err := testing.Sleep(ctx, h.Config.KeypressDelay); err != nil {
		s.Fatal("Failed to sleep: ", err)
	}
	s.Log("Pressing power key")
	if err := h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.DurPress); err != nil {
		s.Fatal("Failed to press power key: ", err)
	}

	s.Log("Waiting for connection to DUT")
	connectCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	if err := h.WaitConnect(connectCtx); err != nil {
		s.Fatal("Failed to connect to DUT: ", err)
	}

	firmwareLogOutput, err := h.Reporter.CatFile(ctx, "/sys/firmware/log")
	if err != nil {
		s.Fatal("Failed to read the firmware log from the DUT: ", err)
	}
	defer func() {
		// Save the firmware log for debugging purposes.
		firmwareLogDestPath := filepath.Join(s.OutDir(), "firmware.log")
		if err := ioutil.WriteFile(firmwareLogDestPath, []byte(firmwareLogOutput), 0666); err != nil {
			s.Fatal("Failed to write firmware log: ", err)
		}
	}()

	var (
		developerWarning         = 0x101
		developerWarningString   = fmt.Sprintf("vboot_draw_(legacy_)?screen.*screen=0x%x", developerWarning)
		developerWarningScreenRe = regexp.MustCompile(developerWarningString)
		skipDelayString          = `vb2_developer_ui.*[developer UI|VbBootDeveloper\(\)].*user pressed Ctrl\+D.*skip delay`
		skipDelayRe              = regexp.MustCompile(skipDelayString)
	)

	s.Log("Validating the os-verification-is-off screen in the firmware log")
	screenMatch := developerWarningScreenRe.FindStringSubmatch(firmwareLogOutput)
	if len(screenMatch) != 2 {
		s.Fatal("Did not find developer warning screen in the firmware log")
	}

	s.Log("Validating dut booted from the os-verification-if-off screen because of timeout")
	skipDelayMatch := skipDelayRe.FindStringSubmatch(firmwareLogOutput)
	if skipDelayMatch != nil {
		s.Fatal("Found unexpected records of ctrld at os-verification-is-off screen from the firmware log")
	}

	s.Log("Checking if the DUT is in developer mode")
	if mainfwType, err := h.Reporter.CrossystemParam(ctx, reporters.CrossystemParamMainfwType); err != nil {
		s.Fatal("Failed to get crossystem mainfw_type: ", err)
	} else if mainfwType != "developer" {
		s.Fatal("Expected mainfw_type to be 'developer', got: ", mainfwType)
	}
}
