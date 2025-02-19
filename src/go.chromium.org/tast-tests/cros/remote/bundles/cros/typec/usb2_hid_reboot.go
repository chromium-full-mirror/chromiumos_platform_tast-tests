// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package typec

import (
	"context"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/remote/bundles/cros/typec/typecutils"
	"go.chromium.org/tast-tests/cros/remote/typec/mcci"
	"go.chromium.org/tast-tests/cros/services/cros/usb"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     Usb2HidReboot,
		Desc:     "Check that a USB HID device enumerates successfully when rebooting",
		Contacts: []string{"chromeos-usb-champs@google.com", "bszpila@google.com", "jthies@google.com"},
		// ChromeOS > Platform > Technologies > USB
		BugComponent: "b:958036",
		Attr:         []string{"group:typec"},
		Vars:         []string{"typec.McciSerial", "typec.McciPort", "typec.McciPath"},
		ServiceDeps:  []string{"tast.cros.usb.SysfsService"},
		Params: []testing.Param{{
			ExtraAttr: []string{"typec_usb_bringup"},
			Val:       5,
			Timeout:   12 * time.Minute,
		}, {
			Name:    "stress",
			Val:     25,
			Timeout: 60 * time.Minute,
		}},
	})
}

// Usb2HidReboot does the following:
//
// - Toggle USB HID device connectivity via the MCCI switch.
// - Count the number of currently connected USB HID devices.
// - Reboot the DUT.
// - Check that there is the same number of USB HID devices as there were before rebooting.
//
// This test expects the following hardware topology:
//
//	 ____network___
//	|              |
//	|              |
//	Host -------- DUT ----- MCCI (`portUsed`) ---- USB HID (can be connected via dock or adapter).
//	|                              |
//	|______________________________|
func Usb2HidReboot(ctx context.Context, s *testing.State) {

	numIterations := s.Param().(int)
	d := s.DUT()

	s.Log("Number of iterations: ", numIterations)

	portUsed, err := strconv.Atoi(s.RequiredVar("typec.McciPort"))
	if err != nil {
		s.Fatal("Failed to parse MCCI port commandline variable: ", err)
	}

	path, _ := s.Var("typec.McciPath")
	sw, err := mcci.GetSwitch(s.RequiredVar("typec.McciSerial"), path)
	if err != nil {
		s.Fatal("Failed to get MCCI switch handle: ", err)
	}
	defer sw.Close()

	// Dial rpc
	cl, err := rpc.Dial(ctx, d, s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	usbClient := usb.NewSysfsServiceClient(cl.Conn)

	// Make sure the device is disconnected before testing
	if port, err := sw.GetActivePort(); err != nil {
		s.Fatal("Could not get used port before testing: ", err)
	} else if port == portUsed {
		hidDevicesWhenOn, err := typecutils.Usb2GetHidDeviceList(ctx, usbClient)
		if err != nil {
			s.Fatal("Could not get HID device list before testing: ", err)
		}
		if err := sw.DisablePorts(); err != nil {
			s.Fatal("Could not disable the port before testing: ", err)
		}
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			if hidDevices, err := typecutils.Usb2GetHidDeviceList(ctx, usbClient); err != nil {
				return errors.Wrap(err, "could not get HID device list before testing")
			} else if len(hidDevices) >= len(hidDevicesWhenOn) {
				return errors.New("failed to disconnect new USB HID device")
			}
			return nil
		}, &testing.PollOptions{Timeout: 10 * time.Second, Interval: time.Second}); err != nil {
			s.Fatal("Failed to disconnect the device before the test: ", err)
		}

	} else if err := sw.DisablePorts(); err != nil {
		s.Fatal("Could not disable the port before testing: ", err)
	}

	cl.Close(ctx)

	for i := 1; i <= numIterations; i++ {
		s.Log("Running iteration ", i)
		if err := performUsb2HidRebootIteration(ctx, s, d, sw, portUsed); err != nil {
			s.Fatalf("Failed test on iteration %d: %v", i, err)
		}
	}
}

// performUsb2HidRebootIteration runs 1 iteration of the USB 2.0 HID reboot test.
func performUsb2HidRebootIteration(ctx context.Context, s *testing.State, d *dut.DUT, sw *mcci.Switch, mcciPort int) error {

	// Dial rpc
	cl, err := rpc.Dial(ctx, d, s.RPCHint())
	if err != nil {
		return errors.Wrap(err, "failed to connect to the RPC service on the DUT")
	}
	defer cl.Close(ctx)
	usbClient := usb.NewSysfsServiceClient(cl.Conn)

	// Get the device count when switch is off
	hidDevicesWhenOff, err := typecutils.Usb2GetHidDeviceList(ctx, usbClient)
	if err != nil {
		return errors.Wrap(err, "could not get HID device list before hotplug")
	}

	// Enable the switch.
	if err := sw.EnablePort(mcciPort); err != nil {
		return errors.Wrap(err, "failed to switch on the port")
	}

	// Get the device count when switch is on
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		hidDevices, err := typecutils.Usb2GetHidDeviceList(ctx, usbClient)
		if err != nil {
			return errors.Wrap(err, "could not get HID device list after hotplug")
		}

		if len(hidDevicesWhenOff) >= len(hidDevices) {
			return errors.New("failed to enumerate new USB HID device")
		}
		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second, Interval: time.Second}); err != nil {
		return err
	}

	hidDevicesBeforeReboot, err := typecutils.Usb2GetHidDeviceList(ctx, usbClient)
	if err != nil {
		return errors.Wrap(err, "could not get HID device list before reboot")
	}

	// Reboot the DUT.
	if err := d.Reboot(ctx); err != nil {
		return errors.Wrap(err, "failed to reboot DUT")
	}

	if err := testing.Poll(ctx, d.Connect, &testing.PollOptions{Timeout: time.Minute}); err != nil {
		return errors.Wrap(err, "failed to re-connect to the DUT after reboot")
	}

	// Redial rpc after reboot
	cl, err = rpc.Dial(ctx, d, s.RPCHint())
	if err != nil {
		return errors.Wrap(err, "failed to connect to the RPC service on the DUT")
	}
	defer cl.Close(ctx)
	usbClient = usb.NewSysfsServiceClient(cl.Conn)

	// Get device count after rebooting.
	hidDevicesAfterReboot, err := typecutils.Usb2GetHidDeviceList(ctx, usbClient)
	if err != nil {
		return errors.Wrap(err, "could not get external HID device list after reboot")
	} else if len(hidDevicesAfterReboot) < len(hidDevicesBeforeReboot) {
		return errors.New("external HID device failed to enumerate after reboot")
	}

	// Disable the switch.
	if err := sw.DisablePorts(); err != nil {
		return errors.Wrap(err, "failed to switch off the port")
	}

	// Check for device disconnection
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		hidDevices, err := typecutils.Usb2GetHidDeviceList(ctx, usbClient)
		if err != nil {
			return errors.Wrap(err, "could not get HID device list after disconnection")
		}
		if len(hidDevices) != len(hidDevicesWhenOff) {
			return errors.New("failed to disconnect USB HID device")
		}
		return nil
	}, &testing.PollOptions{Timeout: 5 * time.Second, Interval: time.Second}); err != nil {
		return err
	}

	return nil
}
