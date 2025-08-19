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
	"go.chromium.org/tast-tests/cros/common/usbutils/usbswitch"
	"go.chromium.org/tast-tests/cros/remote/typec/typecswitch"
	"go.chromium.org/tast-tests/cros/services/cros/usb"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     Usb2Suspend,
		Desc:     "Check that a USB 2.0 device remains enumerated though suspend/resume",
		Contacts: []string{"chromeos-usb-champs@google.com", "bszpila@google.com", "jthies@google.com"},
		// ChromeOS > Platform > Technologies > USB
		BugComponent: "b:958036",
		Attr:         []string{"group:typec"},
		Fixture:      "typecSwitch",
		ServiceDeps:  []string{"tast.cros.usb.SysfsService"},
		Params:       typecswitch.GenerateParams(15, 10, usbswitch.Usb2Mode, "typec_usb_bringup"),
	})
}

// Usb2Suspend does the following:
//
// - Toggle USB device connection via USB switch.
// - Verify that one or more external USB devices is connected to the DUT.
// - Suspend/Resume the DUT.
// - Check that the USB device(s) are still connected and have not re-enumerated.
//
// This test expects the following hardware topology:
//
//	 ____network___
//	|              |
//	|              |
//	Host          DUT ----- USB switch ---- USB 2.0 device (can be connected via dock or adapter).
//	|                            |
//	|____________________________|
func Usb2Suspend(ctx context.Context, s *testing.State) {
	d := s.DUT()
	testData := s.Param().(typecswitch.TestSetupData)
	s.Log("Number of iterations: ", testData.Iterations)

	// Get the switch from the fixture.
	fixtData, ok := s.FixtValue().(*typecswitch.FixtureData)
	if !ok {
		s.Fatal("Failed to get fixture data")
	}
	sw := fixtData.TestSwitch

	if err := typecswitch.SetupSwitch(ctx, sw, testData); err != nil {
		s.Fatal("Failed to setup switch: ", err)
	}

	for i := 1; i <= testData.Iterations; i++ {
		s.Log("Running iteration ", i)
		if err := performUsb2SuspendIteration(ctx, s, d, sw); err != nil {
			s.Fatalf("Failed test on iteration %d: %v", i, err)
		}
	}
}

// performUsb2SuspendIteration runs 1 iteration of the USB 2.0 suspend test.
func performUsb2SuspendIteration(ctx context.Context, s *testing.State, d *dut.DUT, sw usbswitch.Switch) error {
	const suspendDurationS = 10

	// Dial rpc
	cl, err := rpc.Dial(ctx, d, s.RPCHint())
	if err != nil {
		return errors.Wrap(err, "unable to connect to the RPC service on the DUT")
	}
	defer cl.Close(ctx)
	usbClient := usb.NewSysfsServiceClient(cl.Conn)

	// Get the device count when switch is off
	devicesWhenOff, err := typecutils.Usb2GetDeviceList(ctx, usbClient)
	if err != nil {
		return errors.Wrap(err, "could not get USB2 device list before hotplug")
	}

	// Enable the switch.
	if err := sw.EnablePort(ctx); err != nil {
		return errors.Wrap(err, "failed to switch on the port")
	}

	// Get the device count when switch is on
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		devices, err := typecutils.Usb2GetDeviceList(ctx, usbClient)
		if err != nil {
			return errors.Wrap(err, "could not get USB2 device list after hotplug")
		}

		if len(devicesWhenOff) >= len(devices) {
			return errors.New("failed to enumerate new USB2 device")
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

	// Create a list of external USB 2.0 devices connected to the DUT.
	deviceWatchList, err := typecutils.Usb2GetDeviceList(ctx, usbClient)
	if err != nil {
		return errors.Wrap(err, "failed to get USB2 device list before suspend")
	}

	if len(deviceWatchList) == 0 {
		return errors.Wrap(err, "failed to find valid external USB2 device")
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

	// Confirm all external USB 2.0 devices are present and have not re-enumerated.
	for _, d := range deviceWatchList {
		if _, present := currentDeviceMap.Devices[d]; !present {
			return errors.New("could not find expected device in current USB device map")
		}
		if initialDeviceMap.Devices[d].GetDevnum() != currentDeviceMap.Devices[d].GetDevnum() {
			return errors.New("devnum changed during suspend/resume")
		}
	}

	// Disable the switch.
	if err := sw.DisablePorts(ctx); err != nil {
		return errors.Wrap(err, "failed to switch off the port")
	}

	// Check for device disconnection
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		devices, err := typecutils.Usb2GetDeviceList(ctx, usbClient)
		if err != nil {
			return errors.Wrap(err, "could not get USB2 device list after disconnection")
		}
		if len(devices) != len(devicesWhenOff) {
			return errors.New("failed to disconnect USB2 device")
		}
		return nil
	}, &testing.PollOptions{Timeout: 5 * time.Second, Interval: time.Second}); err != nil {
		return err
	}

	return nil
}
