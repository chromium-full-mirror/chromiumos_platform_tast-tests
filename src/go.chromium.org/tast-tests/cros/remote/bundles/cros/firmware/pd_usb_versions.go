// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: PDUsbVersions,
		Desc: "Verify USB-C/PD USB 2.0 and 3.0 support",
		Contacts: []string{
			"chromeos-faft@google.com",
			"jasonyuan@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Fixture:      fixture.NormalMode,
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		Timeout:      20 * time.Minute,
		TestBedDeps:  tbdep.ServoPresentAndWorking,
		Attr:         []string{"group:firmware", "firmware_pd_unstable"},
		Params: []testing.Param{{
			Name: "normal",
			Val: firmware.PDTestParams{
				DTS: firmware.DTSModeOff,
			},
		}, {
			Name: "normal_snk",
			Val: firmware.PDTestParams{
				PowerRole: firmware.RoleSink,
				DTS:       firmware.DTSModeOff,
			},
		}, {
			Name: "flipcc",
			Val: firmware.PDTestParams{
				CC:  firmware.CCPolarityFlipped,
				DTS: firmware.DTSModeOff,
			},
		}, {
			Name: "flipcc_snk",
			Val: firmware.PDTestParams{
				CC:        firmware.CCPolarityFlipped,
				PowerRole: firmware.RoleSink,
				DTS:       firmware.DTSModeOff,
			},
		}, {
			Name: "dts",
			Val: firmware.PDTestParams{
				DTS: firmware.DTSModeOn,
			},
		}, {
			Name: "dts_snk",
			Val: firmware.PDTestParams{
				DTS:       firmware.DTSModeOn,
				PowerRole: firmware.RoleSink,
			},
		}, {
			Name: "flipcc_dts",
			Val: firmware.PDTestParams{
				CC:  firmware.CCPolarityFlipped,
				DTS: firmware.DTSModeOn,
			},
		}, {
			Name: "flipcc_dts_snk",
			Val: firmware.PDTestParams{
				CC:        firmware.CCPolarityFlipped,
				DTS:       firmware.DTSModeOn,
				PowerRole: firmware.RoleSink,
			},
		}, {
			Name: "shutdown",
			Val: firmware.PDTestParams{
				Shutdown: true,
				DTS:      firmware.DTSModeOff,
			},
		}, {
			Name: "suspend",
			Val: firmware.PDTestParams{
				Suspend: true,
				DTS:     firmware.DTSModeOff,
			},
		}},
	})
}

const (
	// vendor id of the servo_v4.1 hub as seen by the DUT
	servoHubVendorID string = "04b4:"
	reUsbVersion     string = `bcdUSB[\s]+([\d]+).([\d]+)`

	// usbVersionPollTimeout is the timeout for a usb connection
	usbVersionPollTimeout time.Duration = 30 * time.Second
	// usbVersionPollInterval is the time before testing for a usb connection
	usbVersionPollInterval time.Duration = 5 * time.Second
)

func PDUsbVersions(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to create config: ", err)
	}

	testParams := s.Param().(firmware.PDTestParams)

	if err := firmware.SetupPDTester(ctx, h, testParams, s.OutDir()); err != nil {
		s.Fatal("Failed to configure Servo for PD testing: ", err)
	}

	// Ensure that the DUT is reset and a valid connection exists at the end of the test
	defer func() {
		h.Servo.ServoSetUSBVersion3(ctx, false)

		if err := h.Servo.RunECCommand(ctx, "reboot"); err != nil {
			s.Fatal("Failed to reboot: ", err)
		}

		if err := h.WaitConnect(ctx, firmware.SkipPDRoleSnk); err != nil {
			s.Fatal("Failed to boot after test: ", err)
		}
	}()

	testing.ContextLog(ctx, "turning USB 3 off")
	h.Servo.ServoSetUSBVersion3(ctx, false)

	if testParams.Shutdown || testParams.Suspend {
		if err := h.Servo.SetPowerState(ctx, servo.PowerStateOn); err != nil {
			testing.ContextLog(ctx, "Failed to power on DUT: ", err)
		}
		if err := h.WaitConnect(ctx); err != nil {
			s.Fatal("Failed to boot after test: ", err)
		}
	}

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		testing.ContextLog(ctx, "retrieving USB information")
		usbInfoByteArr, err := h.DUT.Conn().CommandContext(ctx, "lsusb", "-d", servoHubVendorID, "-v").Output(ssh.DumpLogOnError)
		if err != nil {
			return errors.Wrap(err, "failed to retrieve USB information")
		}

		usbVersionRe := regexp.MustCompile(reUsbVersion)
		matches := usbVersionRe.FindAllStringSubmatch(string(usbInfoByteArr), -1)

		for _, usbVersion := range matches {
			testing.ContextLogf(ctx, "Found usb version: %s", usbVersion[0])
			if usbVersion[1] == "3" {
				return errors.Wrap(err, "found usb 3 connection when it should be disabled")
			}
		}

		return nil
	}, &testing.PollOptions{Timeout: usbVersionPollTimeout, Interval: usbVersionPollInterval}); err != nil {
		s.Fatal("Expected only usb 2 connection: ", err)
	}

	if testParams.Shutdown {
		if err := firmware.ShutdownDUT(ctx, h); err != nil {
			s.Fatal("Failed to shutdown: ", err)
		}
	}
	if testParams.Suspend {
		cmd := h.DUT.Conn().CommandContext(ctx, "powerd_dbus_suspend", "--delay=3")
		if err := cmd.Start(); err != nil {
			s.Fatal("Failed to invoke powerd_dbus_suspend: ", err)
		}

		if err := h.WaitForPowerStates(ctx, firmware.PowerStateInterval, firmware.PowerStateTimeout, "S3", "S0ix"); err != nil {
			s.Fatal("Failed to suspend: ", err)
		}
	}

	testing.ContextLog(ctx, "turning USB 3 on")
	h.Servo.ServoSetUSBVersion3(ctx, true)

	if testParams.Shutdown || testParams.Suspend {
		if err := h.Servo.SetPowerState(ctx, servo.PowerStateOn); err != nil {
			testing.ContextLog(ctx, "Failed to power on DUT: ", err)
		}
		if err := h.WaitConnect(ctx); err != nil {
			s.Fatal("Failed to boot after test: ", err)
		}
	}

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		testing.ContextLog(ctx, "retrieving USB information")
		usbInfoByteArr, err := h.DUT.Conn().CommandContext(ctx, "lsusb", "-d", servoHubVendorID, "-v").Output(ssh.DumpLogOnError)
		if err != nil {
			return errors.Wrap(err, "failed to retrieve USB information")
		}

		usbVersionRe := regexp.MustCompile(reUsbVersion)
		matches := usbVersionRe.FindAllStringSubmatch(string(usbInfoByteArr), -1)

		foundUsb3 := false
		for _, usbVersion := range matches {
			testing.ContextLogf(ctx, "Found usb version: %s", usbVersion[0])
			if usbVersion[1] == "3" {
				foundUsb3 = true
			}
		}

		if !foundUsb3 {
			return errors.Wrap(err, "could not find usb 3 connection when it should be enabled")
		}

		return nil
	}, &testing.PollOptions{Timeout: usbVersionPollTimeout, Interval: usbVersionPollInterval}); err != nil {
		s.Fatal("Expected usb 3 connection: ", err)
	}
}
