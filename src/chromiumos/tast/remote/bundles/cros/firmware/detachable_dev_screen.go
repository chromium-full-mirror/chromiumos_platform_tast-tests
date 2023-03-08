// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"regexp"
	"strings"
	"time"

	fwCommon "chromiumos/tast/common/firmware"
	"chromiumos/tast/common/servo"
	"chromiumos/tast/errors"
	"chromiumos/tast/remote/firmware"
	"chromiumos/tast/remote/firmware/fixture"
	"chromiumos/tast/ssh"
	"chromiumos/tast/testing"
	"chromiumos/tast/testing/hwdep"
)

type devFwParam struct {
	devFwScreenName string
}

func init() {
	testing.AddTest(&testing.Test{
		Func: DetachableDevScreen,
		Desc: "Confirms basic dev screen behaviors for detachables",
		Contacts: []string{
			"chromeos-faft@google.com",
			"cienet-firmware@cienet.corp-partner.google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		// TODO: When stable, change firmware_unstable to a different attr and add linto@chromium.org to gerrit review.
		Attr:         []string{"group:firmware", "firmware_unstable", "firmware_detachable", "firmware_usb"},
		SoftwareDeps: []string{"crossystem"},
		Fixture:      fixture.DevMode,
		HardwareDeps: hwdep.D(hwdep.ChromeEC(), hwdep.FormFactor(hwdep.Detachable)),
		Params: []testing.Param{{
			Timeout: 60 * time.Minute,
			Val: devFwParam{
				devFwScreenName: "devWarningScreen",
			},
		}, {
			Name:    "dev_options",
			Timeout: 60 * time.Minute,
			// The dev_options screen doesn't exist on the menu_switcher ui.
			ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel("coachz", "homestar", "wormdingler", "quackingstick")),
			Val: devFwParam{
				devFwScreenName: "devOptions",
			},
		}},
	})
}

func DetachableDevScreen(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}

	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Requiring config")
	}

	cs := s.CloudStorage()
	if err := h.SetupUSBKey(ctx, cs); err != nil {
		s.Fatal("USBKey not working: ", err)
	}

	type triggerArgs struct {
		selectMenuOption    string
		verifyFwScreenInLog string
		pressKeyOrButton    string
	}

	type testCase struct {
		trigger     triggerArgs
		bootFromUSB bool
		bootMode    fwCommon.BootMode
		powerState  string
	}

	const (
		debugInfoScreen   string = `VbDisplayDebugInfo:(\n|.)*?TPM:[^\n\r]*`
		advancedOptScreen string = `vb2ex_display_ui: screen=0x120`
	)

	devWarningScreen := []testCase{
		{triggerArgs{"debugInfo", debugInfoScreen, ""}, false, fwCommon.BootModeDev, "S0"},
		{triggerArgs{"menuPowerOff", "", ""}, false, fwCommon.BootModeDev, "G3"},
		{triggerArgs{"spaceEnter", "", ""}, false, fwCommon.BootModeDev, "G3"},
		{triggerArgs{"", "", "volumeUp"}, true, fwCommon.BootModeUSBDev, "S0"},
		{triggerArgs{"", "", "volumeDown"}, false, fwCommon.BootModeDev, "S0"},
		{triggerArgs{"", "", "powerButtonLong"}, false, fwCommon.BootModeDev, "G3"},
		{triggerArgs{"", "", "ctrlU"}, true, fwCommon.BootModeUSBDev, "S0"},
		{triggerArgs{"", "", "ctrlD"}, false, fwCommon.BootModeDev, "S0"},
	}
	devWarningScreenMS := []testCase{
		{triggerArgs{"advancedOptions", advancedOptScreen, ""}, false, fwCommon.BootModeDev, "S0"},
		{triggerArgs{"menuPowerOffMS", "", ""}, false, fwCommon.BootModeDev, "G3"},
		{triggerArgs{"menuBootFromUSBMS", "", ""}, true, fwCommon.BootModeUSBDev, "S0"},
		{triggerArgs{"menuBootFromInternal", "", ""}, false, fwCommon.BootModeDev, "S0"},
		{triggerArgs{"spaceEnter", "", ""}, false, fwCommon.BootModeDev, "S0"},
		{triggerArgs{"", "", "volumeUp"}, true, fwCommon.BootModeUSBDev, "S0"},
		{triggerArgs{"", "", "volumeDown"}, false, fwCommon.BootModeDev, "S0"},
		{triggerArgs{"", "", "powerButtonLong"}, false, fwCommon.BootModeDev, "G3"},
		{triggerArgs{"", "", "ctrlU"}, true, fwCommon.BootModeUSBDev, "S0"},
		{triggerArgs{"", "", "ctrlD"}, false, fwCommon.BootModeDev, "S0"},
	}
	devOptions := []testCase{
		{triggerArgs{"menuBootFromUSB", "", ""}, true, fwCommon.BootModeUSBDev, "S0"},
		{triggerArgs{"menuBootFromInternal", "", ""}, false, fwCommon.BootModeDev, "S0"},
		{triggerArgs{"menuDevOptionsPowerOff", "", ""}, false, fwCommon.BootModeDev, "G3"},
	}

	args := s.Param().(devFwParam)
	var testSteps []testCase
	switch args.devFwScreenName {
	case "devWarningScreen":
		switch h.Config.ModeSwitcherType {
		case "tablet_detachable_switcher":
			testSteps = devWarningScreen
		case "menu_switcher":
			testSteps = devWarningScreenMS
		default:
			s.Fatalf("Got unexpected mode switcher type: %s", h.Config.ModeSwitcherType)
		}
	case "devOptions":
		testSteps = devOptions
	}

	for _, step := range testSteps {
		s.Log("Enabling dev_boot_usb")
		if err := h.DUT.Conn().CommandContext(ctx, "crossystem", "dev_boot_usb=1").Run(ssh.DumpLogOnError); err != nil {
			s.Fatal("Failed to enable dev_boot_usb: ", err)
		}
		s.Log("Enabling USB connection to DUT")
		if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxDUT); err != nil {
			s.Fatal("Failed to enable USB: ", err)
		}
		// Power cycle the DUT to clear the firmware log, so that records prior
		// to this test are wiped.
		if err := h.Servo.SetPowerState(ctx, servo.PowerStateOff); err != nil {
			s.Fatal("Failed to poweroff the DUT: ", err)
		}
		s.Log(ctx, "Checking for G3 powerstate")
		if err := h.WaitForPowerStates(ctx, firmware.PowerStateInterval, firmware.PowerStateTimeout, "G3", "S5"); err != nil {
			s.Fatal("Failed to get power state at G3: ", err)
		}
		// Sleeping for 5 seconds ensures the DUT's power completely off.
		s.Log("Sleeping for 5 seconds")
		if err := testing.Sleep(ctx, 5*time.Second); err != nil {
			s.Fatal("Failed to wait for 5sec: ", err)
		}
		if err := h.Servo.SetPowerState(ctx, servo.PowerStateReset); err != nil {
			s.Fatal("Failed to power on DUT: ", err)
		}
		s.Logf("Sleeping for %s (FirmwareScreen) ", h.Config.FirmwareScreen)
		if err := testing.Sleep(ctx, h.Config.FirmwareScreen); err != nil {
			s.Fatalf("Failed to sleep for %s: %v", h.Config.FirmwareScreen, err)
		}
		if args.devFwScreenName == "devOptions" {
			if err := nTimesTraverseSelect(ctx, h, servo.VolumeUpHold, 3, "Developer Options"); err != nil {
				s.Fatal("Failed to enter dev screen: ", err)
			}
		}
		if step.trigger.pressKeyOrButton != "" {
			if err := testTriggerPressKeyOrButton(ctx, h, step.trigger.pressKeyOrButton); err != nil {
				s.Fatalf("Failed to press %s: %v", step.trigger.pressKeyOrButton, err)
			}
		}

		if step.trigger.selectMenuOption != "" {
			if err := testTriggerSelectMenuOption(ctx, h, step.trigger.selectMenuOption); err != nil {
				s.Fatalf("Failed to select %s: %v", step.trigger.selectMenuOption, err)
			}
		}
		if err := h.WaitForPowerStates(ctx, firmware.PowerStateInterval, 2*time.Minute, step.powerState); err != nil {
			s.Fatalf("Failed to get powerState:%s", step.powerState)
		}
		if step.powerState == "G3" {
			s.Log("Setting dut's power on")
			if err := h.Servo.SetPowerState(ctx, servo.PowerStateOn); err != nil {
				s.Fatal("Failed to set dut's power on: ", err)
			}
		}
		if err := func() error {
			s.Log("Waiting for DUT to power ON")
			waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, 8*time.Minute)
			defer cancelWaitConnect()
			if err := h.WaitConnect(waitConnectCtx); err != nil {
				return err
			}
			return nil
		}(); err != nil {
			s.Fatal("Failed to reconnect to DUT: ", err)
		}
		s.Log("Checking for boot device type")
		bootFromUSB, err := h.Reporter.BootedFromRemovableDevice(ctx)
		if err != nil {
			s.Fatal("Failed to check boot device type: ", err)
		}
		if bootFromUSB != step.bootFromUSB {
			s.Fatalf("Expected boot from device: %s, but got: %s", bootedDeviceType(step.bootFromUSB), bootedDeviceType(bootFromUSB))
		}
		s.Log("Checking for DUT's boot mode")
		currentMode, err := h.Reporter.CurrentBootMode(ctx)
		if err != nil {
			s.Fatal("Failed to check for boot mode: ", err)
		}
		if currentMode != step.bootMode {
			s.Fatalf("Expected boot mode: %q, but got: %q", step.bootMode, currentMode)
		}
		s.Logf("Current boot mode: %q", currentMode)

		output, err := h.Reporter.CatFile(ctx, "/sys/firmware/log")
		if err != nil {
			s.Fatal("Failed to read firmware log: ", err)
		}
		if step.trigger.verifyFwScreenInLog == "" {
			continue
		}
		re := regexp.MustCompile(step.trigger.verifyFwScreenInLog)
		match := re.FindStringSubmatch(string(output))
		if match == nil {
			switch step.trigger.verifyFwScreenInLog {
			case debugInfoScreen:
				// If the debug info page wasn't found in the log file, the debug info
				// was probably printed in the top-left corner of the dut's screen. When
				// this is the case, check for the background screen, which would be the
				// same screen as the one that the dut has just traversed to. In the firmware
				// log, this screen would get recorded twice.
				devWarningScrenen := "vboot_draw_ui: screen=0x20a locale=0, selected_index=1"
				if strings.Count(string(output), devWarningScrenen) != 2 {
					s.Fatal("Did not find any records that debug info was printed")
				}
				s.Log("Found debug info floating on the dev warning screen")
			default:
				s.Fatalf("Unable to find %s in firmware log", step.trigger.verifyFwScreenInLog)
			}
		}
		s.Logf("Found screen for %s", step.trigger.selectMenuOption)
	}
}

func bootedDeviceType(bootFromUSB bool) string {
	if bootFromUSB {
		return "USB"
	}
	return "Internal Disk"
}

func testTriggerPressKeyOrButton(ctx context.Context, h *firmware.Helper, trigger string) error {
	var err error
	testing.ContextLogf(ctx, "Testing trigger %s", trigger)
	switch trigger {
	case "ctrlD":
		err = h.Servo.KeypressWithDuration(ctx, servo.CtrlD, servo.DurTab)
	case "ctrlU":
		err = h.Servo.KeypressWithDuration(ctx, servo.CtrlU, servo.DurTab)
	case "volumeUp":
		err = h.Servo.SetInt(ctx, servo.VolumeUpHold, 5000)
	case "volumeDown":
		err = h.Servo.SetInt(ctx, servo.VolumeDownHold, 5000)
	case "powerButtonLong":
		testing.ContextLogf(ctx, "Pressing power button for %s", h.Config.HoldPwrButtonNoPowerdShutdown)
		err = h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.Dur(h.Config.HoldPwrButtonNoPowerdShutdown))
	default:
		return errors.Errorf("failed to trigger %s, no command found", trigger)
	}
	if err != nil {
		return errors.Wrapf(err, "failed to press %s", trigger)
	}
	return nil
}

func testTriggerSelectMenuOption(ctx context.Context, h *firmware.Helper, trigger string) error {
	var err error
	testing.ContextLogf(ctx, "Testing trigger %s", trigger)
	switch trigger {
	case "menuPowerOff":
		err = h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.DurTab)
	case "menuPowerOffMS":
		// Traverse the menu and select the option 'power off' on the menu_switcher ui.
		err = nTimesTraverseSelect(ctx, h, servo.VolumeDownHold, 3, "Power Off")
	case "spaceEnter":
		// Pressing space & enter boots the dut to normal mode if its
		// firmware screen uses KeyboardDevSwitcher. On tablet_detachable_switcher,
		// this combination would power the dut off because pressing
		// space triggers nothing, and pressing enter selects the default
		// menu option 'power off'. On the menu_switcher ui, the default highlighted menu
		// option would be 'boot from internal disk', which would boot the dut to S0.
		err = func() error {
			testing.ContextLog(ctx, "Pressing SPACE")
			if err := h.Servo.PressKey(ctx, " ", servo.DurTab); err != nil {
				return errors.Wrap(err, "failed to press space")
			}
			testing.ContextLogf(ctx, "Sleeping %s (KeypressDelay)", h.Config.KeypressDelay)
			if err := testing.Sleep(ctx, h.Config.KeypressDelay); err != nil {
				return errors.Wrapf(err, "failed to press %s seconds", h.Config.KeypressDelay)
			}
			testing.ContextLog(ctx, "Pressing ENTER")
			if err := h.Servo.KeypressWithDuration(ctx, servo.Enter, servo.DurTab); err != nil {
				return errors.Wrap(err, "failed to press enter")
			}
			return nil
		}()
	case "menuBootFromUSB":
		err = nTimesTraverseSelect(ctx, h, servo.VolumeUpHold, 1, "Boot From USB")
	case "menuBootFromUSBMS":
		err = nTimesTraverseSelect(ctx, h, servo.VolumeDownHold, 1, "Boot From USB")
	case "menuBootFromInternal":
		err = nTimesTraverseSelect(ctx, h, servo.VolumeUpHold, 0, "Boot From Internal Disk")
	case "menuDevOptionsPowerOff":
		err = nTimesTraverseSelect(ctx, h, servo.VolumeDownHold, 2, "Power Off")
	case "debugInfo":
		err = func() error {
			if err := nTimesTraverseSelect(ctx, h, servo.VolumeUpHold, 2, "Show Debug Info"); err != nil {
				return errors.Wrap(err, "failed to select Debug Info")
			}
			if err := testing.Sleep(ctx, h.Config.KeypressDelay); err != nil {
				return errors.Wrapf(err, "failed to wait %s", h.Config.KeypressDelay)
			}
			// While at the debug info page, press ctrld to boot because this screen doesn't time out.
			if err := h.Servo.KeypressWithDuration(ctx, servo.CtrlD, servo.DurTab); err != nil {
				return errors.Wrap(err, "failed to press ctrld")
			}
			return nil
		}()
	case "advancedOptions":
		err = func() error {
			if err := nTimesTraverseSelect(ctx, h, servo.VolumeDownHold, 2, "Advanced Options"); err != nil {
				return errors.Wrap(err, "failed to select Advanced Options")
			}
			if err := testing.Sleep(ctx, h.Config.KeypressDelay); err != nil {
				return errors.Wrapf(err, "failed to wait %s", h.Config.KeypressDelay)
			}
			// While at the advanced options page, press ctrld to boot because this screen doesn't time out.
			if err := h.Servo.KeypressWithDuration(ctx, servo.CtrlD, servo.DurTab); err != nil {
				return errors.Wrap(err, "failed to press ctrld")
			}
			return nil
		}()
	default:
		return errors.Errorf("failed to trigger %s, no command found", trigger)
	}
	if err != nil {
		return errors.Wrapf(err, "failed to select %s", trigger)
	}
	return nil
}

func nTimesTraverseSelect(ctx context.Context, h *firmware.Helper, upOrDown servo.IntControl, times int, menuOpt string) error {
	for ; times > 0; times-- {
		testing.ContextLogf(ctx, "Pressing %s", upOrDown)
		if err := h.Servo.SetInt(ctx, upOrDown, 100); err != nil {
			return errors.Wrapf(err, "pressing %s", upOrDown)
		}
		if err := testing.Sleep(ctx, 3*time.Second); err != nil {
			return errors.Wrap(err, "sleeping for 3s")
		}
	}
	testing.ContextLogf(ctx, "Selecting %q", menuOpt)
	if err := h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.DurTab); err != nil {
		return errors.Wrapf(err, "selecting %s", menuOpt)
	}
	if err := testing.Sleep(ctx, 3*time.Second); err != nil {
		return errors.Wrap(err, "sleeping for 3s")
	}
	return nil
}
