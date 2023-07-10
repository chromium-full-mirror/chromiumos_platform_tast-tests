// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	fwCommon "go.chromium.org/tast-tests/cros/common/firmware"
	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type params struct {
	//Set up based on the need for a USB.
	usbPresent       bool
	reconnectTimeout time.Duration
}

type dmfsKeyVal int

const (
	dmfsCtrlD dmfsKeyVal = iota
	dmfsCtrlU
	dmfsUpKey
	dmfsUpArrowKey
	dmfsSpace
)

func init() {
	testing.AddTest(&testing.Test{
		Func: DevModeFwScreen,
		Desc: "Verify the functionality of Ctrl+D and Ctrl+U while on the dev screen",
		Contacts: []string{
			"chromeos-faft@google.com",
			"cienet-firmware@cienet.corp-partner.google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		// TODO: When stable, change firmware_unstable to a different attr and add linto@chromium.org to gerrit review.
		Attr:         []string{"group:firmware", "firmware_unstable"},
		SoftwareDeps: []string{"crossystem"},
		Vars:         []string{"firmware.skipFlashUSB"},
		HardwareDeps: hwdep.D(hwdep.Battery()),
		Fixture:      fixture.DevMode,
		Params: []testing.Param{{
			Val: &params{
				usbPresent: false,
				// To-do: replace with DelayRebootToPing in the future, but monitor results from
				// chromium: 4548855 first to find out how each machine varies in their boot-up
				// time. 8 minutes appeared to help when the test was run on leased machines,
				// though this duration might have also covered the time for remote connection.
				reconnectTimeout: 8 * time.Minute,
			},
			Timeout: 50 * time.Minute,
		}, {
			Name: "usb",
			Val: &params{
				usbPresent:       true,
				reconnectTimeout: 10 * time.Minute,
			},
			ExtraAttr: []string{"firmware_usb"},
			Timeout:   2 * time.Hour,
		}},
	})
}

func DevModeFwScreen(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}

	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to create config: ", err)
	}

	// Set up USB when there is one present, and
	// for cases that depend on it.
	testOpt := s.Param().(*params)
	if testOpt.usbPresent {
		s.Log("Setup USB key")
		skipFlashUSB := false
		if skipFlashUSBStr, ok := s.Var("firmware.skipFlashUSB"); ok {
			var err error
			skipFlashUSB, err = strconv.ParseBool(skipFlashUSBStr)
			if err != nil {
				s.Fatalf("Invalid value for var firmware.skipFlashUSB: got %q, want true/false", skipFlashUSBStr)
			}
		}
		var cs *testing.CloudStorage
		if !skipFlashUSB {
			cs = s.CloudStorage()
		}
		if err := h.SetupUSBKey(ctx, cs); err != nil {
			s.Fatal("USBKey not working: ", err)
		}

		s.Logf("Setting USBMux to %s", servo.USBMuxDUT)
		if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxDUT); err != nil {
			s.Fatal("Failed to set USBMux: ", err)
		}
	} else {
		s.Logf("Setting USBMux to %s", servo.USBMuxOff)
		if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxOff); err != nil {
			s.Fatal("Failed to set USBMux: ", err)
		}
	}
	// For DUTs using MenuSwitcher, or TabletDetachableSwitcher, we would
	// use a goroutine to keep pressing the <up> key in the background,
	// to prevent exit from firmware screen because of timeout.
	goRoutineRequired := h.Config.ModeSwitcherType == firmware.MenuSwitcher || h.Config.ModeSwitcherType == firmware.TabletDetachableSwitcher

	/*
		Notes: This test is parameterized so that steps 1~4 are run in
		'DevModeFwScreen.usb', and steps 5~8 in 'DevModeFwScreen'.

		Ctrl+D/Ctrl+U functionality is tested for scenarios as follows:
		1. Set 'crossystem dev_boot_usb=0', attach USB device to DUT,
			reboot, press ctrl+D and expect boot success into dev mode
			and from main storage.
		2. Set 'crossystem dev_boot_usb=1', attach USB device to DUT,
			reboot, press ctrl+D and expect boot success into dev mode
			and from main storage.
		3. Set 'crossystem dev_boot_usb=0', attach USB device to DUT,
			reboot, press ctrl+U, expect boot to fail. But, following up
			with ctrl+D would allow boot to continue into dev mode and
			from main storage.
		4. Set 'crossystem dev_boot_usb=1', attach USB device to DUT,
			reboot, press ctrl+U, expect boot from USB.
		5. Set 'crossystem dev_boot_usb=0', detach USB device from DUT,
			reboot, press ctrl+D and expect boot success into dev mode
			and from main storage.
		6. Set 'crossystem dev_boot_usb=1', detach USB device from DUT,
			reboot, press ctrl+D and expect boot success into dev mode
			and from main storage.
		7. Set 'crossystem dev_boot_usb=0', detach USB device from DUT,
			reboot, press ctrl+U expect boot to fail. But, following up
			with ctrl+D would allow boot to continue into dev mode and
			from main storage.
		8. Set 'crossystem dev_boot_usb=1', detach USB device from DUT,
			reboot, press ctrl+U, expect boot to fail. But, following up
			with ctrl+D would allow boot to continue into dev mode and
			from main storage.
	*/

	for iter, steps := range []struct {
		devBootUSB      string
		testedShortCuts []dmfsKeyVal
		usbRequired     bool
		expectedMode    fwCommon.BootMode
	}{
		{"0", []dmfsKeyVal{dmfsCtrlD}, true, fwCommon.BootModeDev},
		{"1", []dmfsKeyVal{dmfsCtrlD}, true, fwCommon.BootModeDev},
		{"0", []dmfsKeyVal{dmfsCtrlU, dmfsCtrlD}, true, fwCommon.BootModeDev},
		{"1", []dmfsKeyVal{dmfsCtrlU, dmfsCtrlD}, true, fwCommon.BootModeUSBDev},
		{"0", []dmfsKeyVal{dmfsCtrlD}, false, fwCommon.BootModeDev},
		{"1", []dmfsKeyVal{dmfsCtrlD}, false, fwCommon.BootModeDev},
		{"0", []dmfsKeyVal{dmfsCtrlU, dmfsCtrlD}, false, fwCommon.BootModeDev},
		{"1", []dmfsKeyVal{dmfsCtrlU, dmfsCtrlD}, false, fwCommon.BootModeDev},
	} {
		s.Logf("-------- iteration: %d --------", iter)
		// Run test steps that depend on a usb when there's one present.
		if testOpt.usbPresent != steps.usbRequired {
			continue
		}

		s.Logf("Setting dev boot usb value to %s", steps.devBootUSB)
		if err := h.DUT.Conn().CommandContext(ctx, "crossystem", fmt.Sprintf("dev_boot_usb=%s", steps.devBootUSB)).Run(testexec.DumpLogOnError); err != nil {
			s.Fatalf("Failed to set crossystem dev_boot_usb to %s", steps.devBootUSB)
		}

		s.Log("Power-cycling DUT with a cold reset")
		if err := h.Servo.SetPowerState(ctx, servo.PowerStateReset); err != nil {
			s.Fatal("Failed to cold reset DUT: ", err)
		}

		// Verify that dut becomes unreachable after triggering a cold reset.
		waitDisconnectCtx, cancelWaitConnect := context.WithTimeout(ctx, 10*time.Second)
		defer cancelWaitConnect()

		if err := h.DUT.WaitUnreachable(waitDisconnectCtx); err != nil {
			s.Fatal("Dut is still connected after cold reset: ", err)
		}

		s.Logf("Waiting %s for DUT to get into firmware screen", h.Config.FirmwareScreen)
		// GoBigSleepLint: Wait for the firmware screen.
		if err := testing.Sleep(ctx, h.Config.FirmwareScreen); err != nil {
			s.Fatalf("Failed to sleep for %s to wait for firmware screen: %v", h.Config.FirmwareScreen, err)
		}

		// Ensure DUT disconnected before starting presses
		// on the up key.
		if h.DUT.Connected(ctx) {
			s.Fatal("DUT has already booted past the firmware screen")
		}

		// Document the screen status prior to pressing the up key.
		apPower, screenState, err := h.Servo.GetAPState(ctx)
		if err != nil {
			s.Log("Failed to get information about ap status: ", err)
		}
		s.Logf("Found ap status: %s %s", apPower, screenState)

		var wg sync.WaitGroup
		dutAtFwScreen := false
		done := make(chan bool, 1)
		keyPressErrChan := make(chan error, 1)
		if goRoutineRequired {
			testing.ContextLog(ctx, "Pressing <up> key in the background for extended stay at fw screen")
			go testShortCutsInBackground(ctx, h, &dutAtFwScreen, steps.testedShortCuts, keyPressErrChan, done, &wg)
			// The default timeout at the firmware screen is 30 seconds.
			// Check that pressing the <up> key has worked around this timeout,
			// and that DUT remains disconnected.
			s.Log("Checking for DUT disconnected")
			waitConnectShortCtx, cancelWaitConnectShort := context.WithTimeout(ctx, 1*time.Minute)
			defer cancelWaitConnectShort()
			err := h.WaitConnect(waitConnectShortCtx)
			if err == nil {
				var errMessage error
				if len(keyPressErrChan) != 0 {
					errMessage = <-keyPressErrChan
				}
				s.Fatalf("DUT exited fw screen and reconnected unexpectedly, goroutine error: %v, ap state info prior to pressing the up key: %s %s", errMessage, apPower, screenState)
			}
			if !strings.Contains(err.Error(), context.DeadlineExceeded.Error()) {
				s.Fatal("Unexpected error in waiting for DUT to reconnect: ", err)
			}
			s.Log("DUT is still at dev screen")
			// Trigger the shortcuts to be tested.
			dutAtFwScreen = true
		} else {
			// For DUTs using KeyboardDevSwitcher, pressing a space key would allow
			// an extended stay at the firmware screen. If ctrl+D worked, pressing
			// a space key here would not have any effects, and DUT would
			// eventually boot to ChromeOS. But, if ctrl+D did not work, the space
			// key would stop the boot up process, and DUT would end up disconnected.
			for _, shortcut := range steps.testedShortCuts {
				if err := dmfsPressKey(ctx, h, shortcut, servo.DurTab); err != nil {
					s.Fatal("Failed to simulate key press: ", err)
				}
			}

			s.Log(ctx, "Pressing SPACE key to keep DUT in dev screen")
			if err := dmfsPressKey(ctx, h, dmfsSpace, servo.DurTab); err != nil {
				s.Fatal("Failed to press SPACE to stop in dev screen: ", err)
			}
		}

		if len(keyPressErrChan) != 0 {
			s.Fatal("Unexpected error while pressing keys: ", <-keyPressErrChan)
		}
		s.Log("Waiting for DUT to reconnect")
		waitConnectOps := []firmware.WaitConnectOption{firmware.ResetEthernetDongle}
		waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, testOpt.reconnectTimeout)
		defer cancelWaitConnect()

		if err := h.WaitConnect(waitConnectCtx, waitConnectOps...); err != nil {
			s.Fatal("Failed to reconnect to DUT: ", err)
		}
		if goRoutineRequired {
			close(done)
			close(keyPressErrChan)
			wg.Wait()
		}

		s.Logf("Checking for DUT in %s mode", steps.expectedMode)
		curr, err := h.Reporter.CurrentBootMode(ctx)
		if err != nil {
			s.Fatal("Failed to check for boot mode: ", err)
		}
		if curr != steps.expectedMode {
			s.Fatalf("Expected DUT in %s mode, but got: %s", steps.expectedMode, curr)
		}
	}
}

func dmfsPressKey(ctx context.Context, h *firmware.Helper, key dmfsKeyVal, keypressDuration servo.KeypressDuration) error {
	var err error
	switch key {
	case dmfsCtrlD:
		testing.ContextLog(ctx, "Pressing ctrlD")
		err = h.Servo.KeypressWithDuration(ctx, servo.CtrlD, keypressDuration)
	case dmfsCtrlU:
		testing.ContextLog(ctx, "Pressing ctrlU")
		err = h.Servo.KeypressWithDuration(ctx, servo.CtrlU, keypressDuration)
	case dmfsUpKey:
		err = h.Servo.KeypressWithDuration(ctx, servo.ArrowUp, keypressDuration)
	case dmfsSpace:
		testing.ContextLog(ctx, "Pressing space key")
		err = h.Servo.PressKey(ctx, " ", keypressDuration)
	default:
		return errors.Errorf("found unknown key %d", key)
	}
	if err != nil {
		return err
	}
	// GoBigSleepLint: Simulate a specific speed of key presses.
	if err := testing.Sleep(ctx, 2*time.Second); err != nil {
		return errors.Wrap(err, "failed to sleep")
	}
	return nil
}

func testShortCutsInBackground(ctx context.Context, h *firmware.Helper, dutAtFwScreen *bool, testedShortCuts []dmfsKeyVal, keyPressErrChan chan error, done chan bool, wg *sync.WaitGroup) {
	counter := 0
	defer wg.Done()
	wg.Add(1)
	for {
		if err := func() error {
			if err := dmfsPressKey(ctx, h, dmfsUpKey, servo.DurTab); err != nil {
				return errors.Wrap(err, "failed to extend fw screen")
			}
			// Pressing the up key would ensure an extended stay at the
			// firmware screen, beyond the default timeout of 30 secs.
			// Start pressing and testing shortcuts in the background
			// during the extended period.
			if *dutAtFwScreen {
				if counter < len(testedShortCuts) {
					if err := dmfsPressKey(ctx, h, testedShortCuts[counter], servo.DurTab); err != nil {
						return errors.Wrap(err, "failed to simulate key press")
					}
					counter++
				} else {
					// To avoid DUT stuck at the firmware screen, after all shortcuts were
					// tested, press ctrl_d till DUT connected.
					if err := dmfsPressKey(ctx, h, dmfsCtrlD, servo.DurTab); err != nil {
						return errors.Wrap(err, "failed to simulate key press")
					}
				}
			}
			return nil
		}(); err != nil && !errors.Is(err, context.Canceled) {
			keyPressErrChan <- err
			return
		}

		select {
		case <-done:
			return
		default:
		}
	}
}
