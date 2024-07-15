// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package typec

import (
	"context"
	"strconv"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/common/typecutils"
	"go.chromium.org/tast-tests/cros/remote/typec/mcci"
	"go.chromium.org/tast-tests/cros/services/cros/usb"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     Usb2HidSuspend,
		Desc:     "Check that a USB HID device remains enumerated during suspend/resume",
		Contacts: []string{"chromeos-usb-champs@google.com", "bszpila@google.com", "jthies@google.com"},
		// ChromeOS > Platform > Technologies > USB
		BugComponent: "b:958036",
		Attr:         []string{"group:typec"},
		Vars:         []string{"typec.McciSerial", "typec.McciPort", "typec.McciPath"},
		ServiceDeps:  []string{"tast.cros.usb.SysfsService"},
		Params: []testing.Param{{
			ExtraAttr: []string{"typec_usb_bringup"},
			Val:       10,
			Timeout:   8 * time.Minute,
		}, {
			Name:    "stress",
			Val:     50,
			Timeout: 40 * time.Minute,
		}},
	})
}

// Usb2HidSuspend does the following:
//
// - Toggle USB HID device connection via MCCI switch.
// - Verify that one or more external USB HID devices is connected to the DUT.
// - Suspend/Resume the DUT.
// - Check that the USB HID device(s) are still connected and have not re-enumerated.
//
// This test expects the following hardware topology:
//
//	 ____network___
//	|              |
//	|              |
//	Host -------- DUT ----- MCCI (`portUsed`) ---- USB HID (can be connected via dock or adapter).
//	|                              |
//	|______________________________|
func Usb2HidSuspend(ctx context.Context, s *testing.State) {

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
		if err := performUsb2HidSuspendIteration(ctx, s, d, sw, portUsed); err != nil {
			s.Fatalf("Failed test on iteration %d: %v", i, err)
		}
	}
}

// performUsb2HidSuspendIteration runs 1 iteration of the USB 2.0 HID suspend test.
func performUsb2HidSuspendIteration(ctx context.Context, s *testing.State, d *dut.DUT, sw *mcci.Switch, mcciPort int) error {
	const suspendDurationS = 10

	// Dial rpc
	cl, err := rpc.Dial(ctx, d, s.RPCHint())
	if err != nil {
		return errors.Wrap(err, "unable to connect to the RPC service on the DUT")
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

	// Get the initial USB device state.
	initialDeviceMap, err := usbClient.GetDevices(ctx, &empty.Empty{})
	if err != nil {
		return errors.Wrap(err, "failed to get USB devices before suspend")
	}

	// Create a list of external USB 2.0 HID devices connected to the DUT.
	deviceWatchList, err := typecutils.Usb2GetHidDeviceList(ctx, usbClient)
	if err != nil {
		return errors.Wrap(err, "failed to get HID device list before suspend")
	}

	if len(deviceWatchList) == 0 {
		return errors.Wrap(err, "failed to find valid external USB HID device")
	}

	// Suspend the DUT.
	err = d.Conn().CommandContext(ctx, "powerd_dbus_suspend", "--timeout=120", "--suspend_for_sec="+strconv.Itoa(suspendDurationS)).Start()
	if err != nil {
		return errors.Wrap(err, "unable to suspend the DUT")
	}

	// Wait for DUT to suspend.
	if err := d.WaitUnreachable(ctx); err != nil {
		return errors.Wrap(err, "could not verify DUT is unreachable after suspend")
	}

	// Wait for the DUT to resume.
	if err := d.WaitConnect(ctx); err != nil {
		return errors.Wrap(err, "DUT failed to resume in time")
	}

	// Redial rpc after suspend
	cl, err = rpc.Dial(ctx, d, s.RPCHint())
	if err != nil {
		return errors.Wrap(err, "unable to connect to the RPC service on the DUT")
	}
	defer cl.Close(ctx)
	usbClient = usb.NewSysfsServiceClient(cl.Conn)

	// Get the current USB device state.
	currentDeviceMap, err := usbClient.GetDevices(ctx, &empty.Empty{})
	if err != nil {
		return errors.Wrap(err, "failed to get USB devices after suspend")
	}

	// Confirm all external USB 2.0 HID devices are present and have not re-enumerated.
	for _, d := range deviceWatchList {
		if _, present := currentDeviceMap.Devices[d]; !present {
			return errors.New("could not find expected device in current USB device map")
		}
		if initialDeviceMap.Devices[d].GetDevnum() != currentDeviceMap.Devices[d].GetDevnum() {
			return errors.New("devnum changed during suspend/resume")
		}
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
		if len(hidDevicesWhenOff) != len(hidDevices) {
			return errors.New("failed to disconnect USB HID device")
		}
		return nil
	}, &testing.PollOptions{Timeout: 5 * time.Second, Interval: time.Second}); err != nil {
		return err
	}

	return nil
}
