// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"io/ioutil"
	"path/filepath"
	"time"

	fwCommon "go.chromium.org/tast-tests/cros/common/firmware"
	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: ToNormConfirmed,
		Desc: "Check that while TO_NORM_CONFIRMED is displayed, ctrl+u, volume and power buttons have no effect",
		Contacts: []string{
			"chromeos-faft@google.com",
			"cienet-firmware@cienet.corp-partner.google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		// TODO: When stable, change firmware_unstable to a different attr and add linto@chromium.org to gerrit review.
		Attr:         []string{"group:firmware", "firmware_unstable", "firmware_detachable", "firmware_usb"},
		SoftwareDeps: []string{"crossystem", "flashrom"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC(), hwdep.FormFactor(hwdep.Detachable), hwdep.SkipOnModel("coachz", "homestar", "wormdingler", "quackingstick")),
		Fixture:      fixture.DevMode,
		Timeout:      2 * time.Hour,
	})
}

func ToNormConfirmed(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}

	ms, err := firmware.NewModeSwitcher(ctx, h)
	if err != nil {
		s.Fatal("Creating mode switcher: ", err)
	}

	// Upload firmware log to Testhaus at the end of the test.
	defer func() {
		logPath := "/sys/firmware/log"
		output, err := h.Reporter.CatFile(ctx, logPath)
		if err != nil {
			s.Fatal("Failed to read firmware log: ", err)
		}
		destPath := filepath.Join(s.OutDir(), "firmware.log")
		if err := ioutil.WriteFile(destPath, []byte(output), 0666); err != nil {
			s.Fatal("Failed to write firmware log: ", err)
		}
	}()

	cs := s.CloudStorage()
	var opts []firmware.SetupUSBOption
	opts = append(opts, firmware.DontFlashIfSameMilestone)
	if err := h.SetupUSBKey(ctx, cs, opts...); err != nil {
		s.Fatal("USBKey not working: ", err)
	}

	// Enable USB connection to DUT for testing Ctrl+U.
	if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxDUT); err != nil {
		s.Fatal("Failed to set USB enable: ", err)
	}

	// If any of the buttons tested below are effective, they would prevent
	// DUT from booting into normal mode, and from the main storage.
	testTrigger := []string{"ctrlU", "volumeUp", "volumeDown", "powerButton"}
	for _, trigger := range testTrigger {
		opts := []firmware.ModeSwitchOption{firmware.CheckToNormConfirmed}
		if err := ms.RebootToMode(ctx, fwCommon.BootModeNormal, opts...); err != nil {
			s.Fatal("Failed to boot to normal mode: ", err)
		}
		// GoBigSleepLint: Simulate a specific speed of button presses.
		if err := testing.Sleep(ctx, h.Config.KeypressDelay); err != nil {
			s.Fatalf("Failed to sleep before testing trigger %s: %v", trigger, err)
		}
		switch trigger {
		case "ctrlU":
			s.Log("Verifying Ctrl+U doesn't trigger a usb boot")
			if err := h.Servo.KeypressWithDuration(ctx, servo.CtrlU, servo.DurTab); err != nil {
				s.Fatal("Failed to make Ctrl+U press: ", err)
			}
		case "volumeUp":
			// In other dev screens, long pressing the volume up button for 3s:
			// DUT boots from USB.
			s.Log("Verifying VolumeUp doesn't trigger a usb boot")
			if err := h.Servo.SetInt(ctx, servo.VolumeUpHold, 3000); err != nil {
				s.Fatal("Failed to make VolumeUp press: ", err)
			}
		case "volumeDown":
			// In other dev screens, long pressing volume down button for 3s:
			// DUT boots into developer mode and from internal disk.
			s.Log("Verifying VolumeDown doesn't trigger boot from main storage in dev mode")
			if err := h.Servo.SetInt(ctx, servo.VolumeDownHold, 3000); err != nil {
				s.Fatal("Failed to make VolumeDown press: ", err)
			}
		case "powerButton":
			// In other dev screens, power button serves as an ENTER key,
			// and pressing it might lead to other screens.
			s.Log("Verifying Power Button doesn't trigger any action")
			if err := h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.DurTab); err != nil {
				s.Fatal("Failed to make power button press: ", err)
			}
		}

		waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, 2*time.Minute)
		defer cancelWaitConnect()
		if err := h.WaitConnect(waitConnectCtx); err != nil {
			s.Fatal("Failed to reconnect to DUT: ", err)
		}

		// Check for DUT's current boot mode.
		curr, err := h.Reporter.CurrentBootMode(ctx)
		s.Logf("Current Boot Mode: %s", curr)
		if err != nil {
			s.Fatal("Failed to check for boot mode: ", err)
		}
		if curr != fwCommon.BootModeNormal {
			s.Fatalf("Expected boot mode: %s, but got: %s", fwCommon.BootModeNormal, curr)
		}

		s.Log("Checking that DUT has booted from the main storage")
		bootedDeviceType, err := h.Reporter.BootedFromRemovableDevice(ctx)
		if err != nil {
			s.Fatal("Failed to check boot device type: ", err)
		}
		if bootedDeviceType {
			s.Fatal("DUT booted from USB unexpectedly")
		}

		s.Log("Rebooting to dev mode before testing the next trigger")
		if err := ms.RebootToMode(ctx, fwCommon.BootModeDev); err != nil {
			s.Fatal("Failed to boot to dev mode: ", err)
		}
	}
}
