// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type ecUsbPortTest int

const (
	testUSBOnLidClose ecUsbPortTest = iota
	testUSBOnShutdown
)

func init() {
	testing.AddTest(&testing.Test{
		Func: ECUSBPorts,
		Desc: "Verify usb ports stop read/write after DUT shuts down",
		Contacts: []string{
			"chromeos-faft@google.com",
			"tij@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Attr:         []string{"group:firmware", "firmware_ec"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		Requirements: []string{"sys-fw-0022-v02"},
		Fixture:      fixture.NormalMode,
		Timeout:      5 * time.Minute,
		Params: []testing.Param{
			{
				Name:              "usb_pins_on_lid_close",
				Val:               testUSBOnLidClose,
				ExtraHardwareDeps: hwdep.D(hwdep.Lid()),
			},
			{
				Name: "usb_pins_on_shutdown",
				Val:  testUSBOnShutdown,
			},
		},
	})
}

const (
	// Output from ec console for gpioget or ioexget looks like:
	// "0* EN_USB_A0_5V" for gpio, or "1* O H EN_USB_A0_5V" for ioex.
	reECUSBPortGet           string        = `(?i)(0|1)[^\r\n]*%s`
	usbPortStatePollTimeout  time.Duration = 15 * time.Second
	usbPortStatePollInterval time.Duration = 5 * time.Second
)

func ECUSBPorts(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}

	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}

	enablePins, err := getUSBPorts(ctx, h)
	if err != nil {
		s.Fatal("Failed to probe usb ports: ", err)
	}

	// If device has no USB A ports, there is no reason to run the full test.
	if len(enablePins) == 0 {
		s.Log("No USB A ports to test")
		return
	}

	s.Log("Check that ports are initially enabled")
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if err := checkUSBAPortEnabled(ctx, h, enablePins, 1); err != nil {
			return errors.Wrap(err, "failed to check usb ports")
		}
		return nil
	}, &testing.PollOptions{Timeout: usbPortStatePollTimeout, Interval: usbPortStatePollInterval}); err != nil {
		s.Fatal("Expected USB Ports to be enabled: ", err)
	}

	switch s.Param().(ecUsbPortTest) {
	case testUSBOnShutdown:
		if err := testPortsAfterShutdown(ctx, h, enablePins); err != nil {
			s.Fatal("Some USB Ports enabled after shutdown: ", err)
		}
		defer func() {
			s.Log("Reset DUT after test end in case it was left powered off")
			if err := h.Servo.SetPowerState(ctx, servo.PowerStateReset); err != nil {
				s.Fatal("Failed to make sure DUT is booted after test end: ", err)
			}
			if err := h.WaitConnect(ctx); err != nil {
				s.Fatal("Failed to reconnect to DUT after test end: ", err)
			}
		}()
	case testUSBOnLidClose:
		// Restart UI to ensure no user is logged in, as this will change power state behaviour on lid close.
		if err := h.DUT.Conn().CommandContext(ctx, "restart", "ui").Run(ssh.DumpLogOnError); err != nil {
			s.Fatal("Failed to restart ui before test: ", err)
		}
		if err := testPortsAfterLidClose(ctx, h, enablePins); err != nil {
			s.Fatal("Some USB Ports enabled after lidclose: ", err)
		}
		defer func() {
			s.Log("Reopen DUT lid in case it was left closed at test end")
			if err := h.Servo.OpenLid(ctx); err != nil {
				s.Fatal("Failed to make sure lid is open after test end: ", err)
			}
		}()
	}

	s.Log("Poll for USB ports re enabled")
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if err := checkUSBAPortEnabled(ctx, h, enablePins, 1); err != nil {
			return errors.Wrap(err, "failed to check usb ports")
		}
		return nil
	}, &testing.PollOptions{Timeout: usbPortStatePollTimeout, Interval: usbPortStatePollInterval}); err != nil {
		s.Fatal("Not all usb ports enabled after booting again: ", err)
	}
}

func testPortsAfterLidClose(ctx context.Context, h *firmware.Helper, enablePins []firmware.USBEnablePin) error {
	if err := h.Servo.CloseLid(ctx); err != nil {
		return errors.Wrap(err, "failed to close lid")
	}

	testing.ContextLog(ctx, "Check for G3 powerstate")
	if err := h.WaitForPowerStates(ctx, firmware.PowerStateInterval, firmware.PowerStateTimeout, "G3"); err != nil {
		return errors.Wrap(err, "failed to get G3 powerstate")
	}

	testing.ContextLog(ctx, "Poll for disabled USB ports")
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if err := checkUSBAPortEnabled(ctx, h, enablePins, 0); err != nil {
			return errors.Wrap(err, "failed to check usb ports")
		}
		return nil
	}, &testing.PollOptions{Timeout: usbPortStatePollTimeout, Interval: usbPortStatePollInterval}); err != nil {
		return errors.Wrap(err, "not all usb ports disabled")
	}
	if err := h.Servo.OpenLid(ctx); err != nil {
		return errors.Wrap(err, "failed to open lid")
	}

	testing.ContextLog(ctx, "Waiting for S0 powerstate")
	err := h.WaitForPowerStates(ctx, firmware.PowerStateInterval, firmware.PowerStateTimeout, "S0")
	if err != nil {
		return errors.Wrap(err, "failed to get S0 powerstate")
	}

	if err := h.WaitConnect(ctx); err != nil {
		return errors.Wrap(err, "failed to reconnect to DUT after restarting")
	}

	return nil
}

func testPortsAfterShutdown(ctx context.Context, h *firmware.Helper, enablePins []firmware.USBEnablePin) error {
	testing.ContextLog(ctx, "Shut down DUT")
	cmd := h.DUT.Conn().CommandContext(ctx, "/sbin/shutdown", "-P", "now")
	if err := cmd.Start(); err != nil {
		return errors.Wrap(err, "failed to shut down DUT")
	}

	testing.ContextLog(ctx, "Check for G3 powerstate")
	if err := h.WaitForPowerStates(ctx, firmware.PowerStateInterval, firmware.PowerStateTimeout, "G3"); err != nil {
		return errors.Wrap(err, "failed to get G3 powerstate")
	}

	testing.ContextLog(ctx, "Poll for disabled USB ports")
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if err := checkUSBAPortEnabled(ctx, h, enablePins, 0); err != nil {
			return errors.Wrap(err, "failed to check usb ports")
		}
		return nil
	}, &testing.PollOptions{Timeout: usbPortStatePollTimeout, Interval: usbPortStatePollInterval}); err != nil {
		return errors.Wrap(err, "not all usb ports disabled")
	}

	testing.ContextLog(ctx, "Power DUT back on with short press of the power button")
	if err := h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.DurTab); err != nil {
		return errors.Wrap(err, "failed to power on DUT with short press of the power button")
	}

	testing.ContextLog(ctx, "Waiting for S0 powerstate")
	err := h.WaitForPowerStates(ctx, firmware.PowerStateInterval, firmware.PowerStateTimeout, "S0")
	if err != nil {
		return errors.Wrap(err, "failed to get S0 powerstate")
	}

	if err := h.WaitConnect(ctx); err != nil {
		return errors.Wrap(err, "failed to reconnect to DUT after restarting")
	}

	return nil
}

func getUSBPorts(ctx context.Context, h *firmware.Helper) ([]firmware.USBEnablePin, error) {
	enablePins := make([]firmware.USBEnablePin, 0)
	ec := firmware.NewECTool(h.DUT, firmware.ECToolNameMain)

	for _, pin := range h.Config.USBEnablePins {
		gpioName := firmware.GpioName(pin.Name)
		if !pin.Ioex {
			// Probe pin to verify it actually exists.
			gpios, err := ec.FindGPIOs(ctx, []firmware.GpioName{gpioName})
			if err != nil {
				return enablePins, errors.Wrapf(err, "failed to probe for gpio %q", pin.Name)
			}
			if len(gpios) == 0 {
				return enablePins, errors.Errorf("GPIO pin %q defined in fw testing configs but not found in gpio, update configs to reflect this", pin.Name)
			}
		} else {
			matchList := []string{fmt.Sprintf(reECUSBPortGet, pin.Name)}
			_, err := h.Servo.RunECCommandGetOutput(ctx, fmt.Sprintf("ioexget %s", pin.Name), matchList)
			if err != nil {
				return enablePins, errors.Wrapf(err, "IOEX pin %q defined in fw testing configs but not found in ioex, update configs to reflect this", pin.Name)
			}
		}
		enablePins = append(enablePins, pin)
	}

	// Check for unset value for usb port a count in testing configs.
	// Many models with custom enable pins set have not set a port count, ignore in that case.
	if h.Config.USBAPortCount == nil && len(h.Config.USBEnablePins) == 0 {
		return enablePins, errors.Errorf("USB A port count for model %s set to null (default value) and no custom usb a gpio pins defined. Set to correct amount in fw-testing-configs", h.Model)
	}

	portsToCheck := 0
	if h.Config.USBAPortCount != nil {
		portsToCheck = *h.Config.USBAPortCount
	}

	// If the value is -1 (unknown), we want to check a bunch of numbers manually.
	if portsToCheck < 0 {
		portsToCheck = 11
	}
	for i := 1; i <= portsToCheck; i++ {
		name := fmt.Sprintf("USB%d_ENABLE", i)
		testing.ContextLogf(ctx, "Probing port %q with gpioget", name)
		gpios, err := ec.FindGPIOs(ctx, []firmware.GpioName{firmware.GpioName(name)})
		if err != nil {
			return enablePins, errors.Wrapf(err, "failed to probe for gpio %q", name)
		}
		if len(gpios) > 0 {
			testing.ContextLogf(ctx, "Found usb port: %q with gpioget", name)
			enablePins = append(enablePins, firmware.USBEnablePin{Name: name, Ioex: false})
		} else if *h.Config.USBAPortCount >= i {
			// If port i doesn't exist (regex fails) but it is expected to exist (0 < i <= h.Config.USBAPortCount), raise an error.
			return enablePins, errors.Errorf("explicit port count is %d; expected port %d to exist but it does not", h.Config.USBAPortCount, i)
		} else {
			testing.ContextLogf(ctx, "Did not find port %q with gpioget", name)
		}
	}
	return enablePins, nil

}

func checkUSBAPortEnabled(ctx context.Context, h *firmware.Helper, enablePins []firmware.USBEnablePin, expectedStatusInt int) error {
	// Collect errors for all usb ports instead of failing at first.
	var unexpectedStatus = map[string]string{}
	expectedStatus := strconv.Itoa(expectedStatusInt)

	for _, pin := range enablePins {
		gpioOrIoex := "gpio"
		if pin.Ioex {
			gpioOrIoex = "ioex"
		}
		testing.ContextLogf(ctx, "Checking status of %q pin name: %q", gpioOrIoex, pin.Name)
		cmd := fmt.Sprintf("%sget %s", gpioOrIoex, pin.Name)
		matchList := []string{fmt.Sprintf(reECUSBPortGet, pin.Name)}
		out, err := h.Servo.RunECCommandGetOutput(ctx, cmd, matchList)
		if err != nil {
			return errors.Wrapf(err, "failed to run cmd %q, got error", cmd)
		}
		if out[0][1] != expectedStatus {
			unexpectedStatus[pin.Name] = out[0][1]
		}
	}

	if len(unexpectedStatus) != 0 {
		failStr := fmt.Sprintf("The following USB Ports didn't have state %q", expectedStatus)
		for name, state := range unexpectedStatus {
			failStr += fmt.Sprintf(", %q had status %q", name, state)
		}
		return errors.New(failStr)
	}
	return nil
}
