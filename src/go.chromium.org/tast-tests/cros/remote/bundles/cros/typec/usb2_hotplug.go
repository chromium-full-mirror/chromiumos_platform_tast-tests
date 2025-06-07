// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package typec

import (
	"context"
	"time"

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
		Func:     Usb2Hotplug,
		Desc:     "Check that a USB 2.0 device enumerates successfully on hotplug",
		Contacts: []string{"chromeos-usb-champs@google.com", "bszpila@google.com", "jthies@google.com"},
		// ChromeOS > Platform > Technologies > USB
		BugComponent: "b:958036",
		Attr:         []string{"group:typec"},
		Fixture:      "typecSwitch",
		ServiceDeps:  []string{"tast.cros.usb.SysfsService"},
		Params:       typecswitch.GenerateParams(5, 10, usbswitch.Usb2Mode, "typec_usb_bringup"),
	})
}

// Usb2Hotplug does the following:
//
// - Disconnect the USB 2.0 device via USB switch.
// - Count the number of currently connected USB 2.0 devices.
// - Reconnect the USB 2.0 device via USB switch.
// - Verify that the number of USB 2.0 devices connected to the DUT increased.
//
// This test expects the following hardware topology:
//
//	 ____network___
//	|              |
//	|              |
//	Host          DUT ----- USB switch ---- USB 2.0 device (can be connected via dock or adapter).
//	|                           |
//	|___________________________|
func Usb2Hotplug(ctx context.Context, s *testing.State) {
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

	// RPC client setup.
	cl, err := rpc.Dial(ctx, d, s.RPCHint())
	if err != nil {
		s.Fatal("Unable to connect to the RPC service on the DUT: ", err)
	}
	defer cl.Close(ctx)
	usbClient := usb.NewSysfsServiceClient(cl.Conn)

	for i := 1; i <= testData.Iterations; i++ {
		s.Log("Running iteration ", i)
		if err := performUsb2HotplugIteration(ctx, d, usbClient, sw); err != nil {
			s.Fatalf("Failed test on iteration %d: %v", i, err)
		}
	}
}

// performUsb2HotplugIteration runs a single iteration of the USB 2.0 hotplug.
func performUsb2HotplugIteration(ctx context.Context, d *dut.DUT, cl usb.SysfsServiceClient, sw usbswitch.Switch) error {

	// Get the device count when switch is off
	devicesWhenOff, err := typecutils.Usb2GetDeviceList(ctx, cl)
	if err != nil {
		return errors.Wrap(err, "could not get USB2 device list before hotplug")
	}

	// Enable the switch.
	if err := sw.EnablePort(ctx); err != nil {
		return errors.Wrap(err, "failed to enable the port")
	}

	// Check for enumeration
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if devices, err := typecutils.Usb2GetDeviceList(ctx, cl); err != nil {
			return errors.Wrap(err, "could not get USB2 device list after hotplug")
		} else if len(devicesWhenOff) >= len(devices) {
			return errors.New("failed to enumerate new USB device")
		}
		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second, Interval: time.Second}); err != nil {
		return err
	}

	// Disable the switch.
	if err := sw.DisablePorts(ctx); err != nil {
		return errors.Wrap(err, "failed to disable the port")
	}

	// Check for disconnection
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if devices, err := typecutils.Usb2GetDeviceList(ctx, cl); err != nil {
			return errors.Wrap(err, "could not get USB2 device list after disconnection")
		} else if len(devices) != len(devicesWhenOff) {
			return errors.New("failed to disconnect USB device")
		}
		return nil
	}, &testing.PollOptions{Timeout: 5 * time.Second, Interval: time.Second}); err != nil {
		return err
	}

	return nil
}
