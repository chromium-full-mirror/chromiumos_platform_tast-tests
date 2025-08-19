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
		Func:     Usb3StorageHotplug,
		Desc:     "Check that a USB 3.0 mass storage device enumerates successfully on hotplug",
		Contacts: []string{"chromeos-usb-champs@google.com", "bszpila@google.com", "jthies@google.com"},
		// ChromeOS > Platform > Technologies > USB
		BugComponent: "b:958036",
		Attr:         []string{"group:typec"},
		Fixture:      "typecSwitch",
		ServiceDeps:  []string{"tast.cros.usb.SysfsService"},
		Params:       typecswitch.GenerateParams(15, 10, usbswitch.Usb3Mode, "typec_usb_bringup"),
	})
}

// Usb3StorageHotplug does the following:
//
// - Unmount any removable media.
// - Disconnect the USB 3.0 mass storage device via USB switch.
// - Count the number of currently connected USB 3.0 mass storage devices.
// - Reconnect the USB 3.0 mass storage device via USB switch.
// - Verify that the number of USB 3.0 mass storage devices connected to the DUT increased.
//
// This test expects the following hardware topology:
//
//	 ____network___
//	|              |
//	|              |
//	Host          DUT ----- USB switch  ---- USB 3.0 mass storage (can be connected via dock or adapter).
//	|                            |
//	|____________________________|
func Usb3StorageHotplug(ctx context.Context, s *testing.State) {
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

	cl, err := rpc.Dial(ctx, d, s.RPCHint())
	if err != nil {
		s.Fatal("Unable to connect to the RPC service on the DUT: ", err)
	}
	defer cl.Close(ctx)
	usbClient := usb.NewSysfsServiceClient(cl.Conn)

	for i := 1; i <= testData.Iterations; i++ {
		s.Log("Running iteration ", i)
		if err := performUsb3StorageHotplugIteration(ctx, d, usbClient, sw); err != nil {
			s.Fatalf("Failed test on iteration %d: %v", i, err)
		}
	}
}

// performUsb3StorageHotplugIteration runs 1 iteration of the USB 3.0 storage hotplug test.
func performUsb3StorageHotplugIteration(ctx context.Context, d *dut.DUT, cl usb.SysfsServiceClient, sw usbswitch.Switch) error {

	// Get the devices when switch is off
	devicesWhenOff, err := typecutils.Usb3GetStorageList(ctx, cl)
	if err != nil {
		return errors.Wrap(err, "could not get external storage list before hotplug")
	}

	// Enable the switch.
	if err := sw.EnablePort(ctx); err != nil {
		return errors.Wrap(err, "failed to enable the port")
	}

	// Check for enumeration
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		devices, err := typecutils.Usb3GetStorageList(ctx, cl)
		if err != nil {
			return errors.Wrap(err, "could not get external storage list after hotplug")
		}
		if len(devicesWhenOff) >= len(devices) {
			return errors.New("failed to enumerate new USB storage device")
		}
		return nil
	}, &testing.PollOptions{Timeout: 15 * time.Second, Interval: time.Second}); err != nil {
		return err
	}

	// Unmount the storage
	if err := typecutils.UnmountRemovableMedia(ctx, d); err != nil {
		return errors.Wrap(err, "failed to unmount removable media")
	}

	// Disable the switch.
	if err := sw.DisablePorts(ctx); err != nil {
		return errors.Wrap(err, "failed to switch off the port")
	}

	// Check for disconnection
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		devices, err := typecutils.Usb3GetStorageList(ctx, cl)
		if err != nil {
			return errors.Wrap(err, "could not get external storage list after disconnection")
		}
		if len(devices) != len(devicesWhenOff) {
			return errors.New("failed to disconnect USB storage device")
		}
		return nil
	}, &testing.PollOptions{Timeout: 5 * time.Second, Interval: time.Second}); err != nil {
		return err
	}

	return nil
}
