// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"io/ioutil"
	"path/filepath"
	"regexp"
	"time"

	fwCommon "go.chromium.org/tast-tests/cros/common/firmware"
	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/errors"
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

	if err := h.Servo.SetString(ctx, "ec_uart_timeout", "10"); err != nil {
		s.Fatal("Failed to extend ec_uart_timeout to 10s: ", err)
	}
	// Reset ec_uart_timeout to default.
	defer func() {
		if err := h.Servo.SetString(ctx, "ec_uart_timeout", "3"); err != nil {
			s.Fatal("Failed to reset ec_uart_timeout to 3s: ", err)
		}
	}()

	// Upload firmware log to Testhaus at the end of the test.
	defer func() {
		if err := h.EnsureDUTBooted(ctx); err != nil {
			s.Fatal("Failed to ensure DUT booted: ", err)
		}
		output, err := h.Reporter.CatFile(ctx, "/sys/firmware/log")
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

	// Find the channel masks required to scan for
	// the outputs of ec button presses.
	chanNames := []servo.ECChannelName{servo.ECChanKeyboard, servo.ECChanSwitch}
	masks := make(map[servo.ECChannelName]string)
	for _, chanName := range chanNames {
		mask, err := h.Servo.FindECChanMask(ctx, chanName)
		if err != nil {
			s.Fatal("Failed to find corresponding channel: ", err)
		}
		masks[chanName] = mask
	}

	// If any of the buttons tested below are effective, they would prevent
	// DUT from booting into normal mode, and from the main storage.
	testTrigger := []string{"ctrlU", "volumeUp", "volumeDown", "powerButton"}
	for _, trigger := range testTrigger {
		if err := h.Servo.SetPowerState(ctx, servo.PowerStateReset); err != nil {
			s.Fatal("Failed to reset the DUT: ", err)
		}
		s.Logf("Sleeping for %s (FirmwareScreen) ", h.Config.FirmwareScreen)
		// GoBigSleepLint: Delay to wait for the firmware screen during boot-up.
		if err := testing.Sleep(ctx, h.Config.FirmwareScreen); err != nil {
			s.Fatalf("Failed to sleep for %s: %v", h.Config.FirmwareScreen, err)
		}

		if err := bootToNormInterstitialScreen(ctx, h, masks); err != nil {
			s.Fatal("Failed to enter to_norm_confirmed page: ", err)
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
			if err := h.Servo.PressECBtnVerifyOutput(ctx, servo.ECVupButton, 3000, masks); err != nil {
				s.Fatal("Failed to make VolumeUp press: ", err)
			}
		case "volumeDown":
			// In other dev screens, long pressing volume down button for 3s:
			// DUT boots into developer mode and from internal disk.
			s.Log("Verifying VolumeDown doesn't trigger boot from main storage in dev mode")
			if err := h.Servo.PressECBtnVerifyOutput(ctx, servo.ECVdownButton, 3000, masks); err != nil {
				s.Fatal("Failed to make VolumeDown press: ", err)
			}
		case "powerButton":
			// In other dev screens, power button serves as an ENTER key,
			// and pressing it might lead to other screens.
			s.Log("Verifying Power Button doesn't trigger any action")
			if err := h.Servo.PressECBtnVerifyOutput(ctx, servo.ECPwrButton, 3000, masks); err != nil {
				s.Fatal("Failed to make power button press: ", err)
			}
		}

		waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, h.Config.DelayRebootToPing)
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

		// On kukui machines, we could scan the firmware log to verify that
		// the 'to_norm_interstitial_screen' was reached. But, this is not
		// possible on other duts also using old fw screen, for example, nocturne
		// soraka, due to the 'console overflowed, log truncated' error.
		// Note, EOF dates for nocturne and soraka are 'June 2026', and 'June 2024'.
		if h.Config.Platform == "kukui" {
			s.Log("Verifing firmware log")
			output, err := h.Reporter.CatFile(ctx, "/sys/firmware/log")
			if err != nil {
				s.Fatal("Failed to read firmware log: ", err)
			}
			re := regexp.MustCompile(`vboot_draw_ui: screen=0x207.*selected_index=0`)
			if match := re.FindStringSubmatch(string(output)); match == nil {
				s.Fatal("Failed to find to_norm_confirmed screen in the firmware log")
			}
		}

		s.Log("Rebooting to dev mode before testing the next trigger")
		if err := ms.RebootToMode(ctx, fwCommon.BootModeDev); err != nil {
			s.Fatal("Failed to boot to dev mode: ", err)
		}
	}
}

// bootToNormInterstitialScreen presses ec buttons to boot dut to
// the 'to_norm_interstitial_screen'.
func bootToNormInterstitialScreen(ctx context.Context, h *firmware.Helper, masks map[servo.ECChannelName]string) error {
	for _, button := range []servo.DetachableECButton{servo.ECVupButton, servo.ECPwrButton, servo.ECPwrButton} {
		// Ensure that there will at least be three attempt in pressing an ec button.
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			if err := h.Servo.PressECBtnVerifyOutput(ctx, button, 500, masks); err != nil {
				return err
			}
			return nil
		}, &testing.PollOptions{Timeout: 30 * time.Second}); err != nil {
			return errors.Wrapf(err, "failed to press %s ", button)
		}
		// GoBigSleepLint: Simulate a specific speed of pressing buttons.
		// Also, the 'to_norm_interstitial_screen' only appears for 5 seconds.
		// We want to restrict the delay here as much as possible to
		// make sure that we don't exceed the timout of this screen,
		// and save time for other test actions that follow after.
		if err := testing.Sleep(ctx, 100*time.Millisecond); err != nil {
			return errors.Wrap(err, "sleeping for 100 milliseconds between button presses")
		}
	}
	return nil
}
