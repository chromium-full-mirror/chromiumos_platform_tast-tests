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
		Func:     Usb3StorageReboot,
		Desc:     "Check that a USB 3.0 mass storage device enumerates successfully after reboot",
		Contacts: []string{"chromeos-usb-champs@google.com", "bszpila@google.com", "jthies@google.com"},
		// ChromeOS > Platform > Technologies > USB
		BugComponent: "b:958036",
		Attr:         []string{"group:typec"},
		Fixture:      "typecSwitch",
		ServiceDeps:  []string{"tast.cros.usb.SysfsService"},
		Params: []testing.Param{{
			ExtraAttr: []string{"typec_usb_bringup", "typec_unigraf274"},
			Val:       5,
			Timeout:   8 * time.Minute,
		}, {
			Name:    "stress",
			Val:     25,
			Timeout: 40 * time.Minute,
		}},
	})
}

// Usb3StorageReboot does the following:
//
// - Connect USB 3.0 mass storage device connectivity via the USB switch.
// - Count the number of currently connected USB mass storage devices.
// - Reboot the DUT.
// - Check that there is the same number of USB 3.0 mass storage devices as there were before rebooting.
// - Unmount the storage.
// - Disable the USB switch.
// - Verify that the USB 3.0 mass storage device is disconnected.
//
// This test expects the following hardware topology:
//
//	 ____network___
//	|              |
//	|              |
//	Host          DUT ----- USB switch ---- USB 3.0 mass storage (can be connected via dock or adapter).
//	|                              |
//	|______________________________|
func Usb3StorageReboot(ctx context.Context, s *testing.State) {
	d := s.DUT()
	numIterations := s.Param().(int)
	s.Log("Number of iterations: ", numIterations)

	// Get the switch from the fixture.
	fixtData, ok := s.FixtValue().(*typecswitch.FixtureData)
	if !ok {
		s.Fatal("Failed to get fixture data")
	}
	sw := fixtData.TestSwitch

	for i := 1; i <= numIterations; i++ {
		s.Log("Running iteration ", i)
		if err := performUsb3StorageRebootIteration(ctx, s, d, sw); err != nil {
			s.Fatalf("Failed test on iteration %d: %v", i, err)
		}
	}
}

// performUsb3StorageRebootIteration runs 1 iteration of the USB 3.0 storage reboot test.
func performUsb3StorageRebootIteration(ctx context.Context, s *testing.State, d *dut.DUT, sw usbswitch.Switch) error {
	// Dial rpc
	cl, err := rpc.Dial(ctx, d, s.RPCHint())
	if err != nil {
		return errors.Wrap(err, "failed to connect to the RPC service on the DUT")
	}
	defer cl.Close(ctx)
	usbClient := usb.NewSysfsServiceClient(cl.Conn)

	// Get the device count when switch is off
	devicesWhenOff, err := typecutils.Usb3GetExternalStorageList(ctx, usbClient)
	if err != nil {
		return errors.Wrap(err, "could not get external storage device list before reboot")
	}

	// Enable the switch.
	if err := sw.EnablePort(ctx); err != nil {
		return errors.Wrap(err, "failed to enable the port")
	}

	// Check for enumeration
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		devices, err := typecutils.Usb3GetExternalStorageList(ctx, usbClient)
		if err != nil {
			return errors.Wrap(err, "could not get external storage list after hotplug")
		}
		if len(devicesWhenOff) >= len(devices) {
			return errors.New("failed to enumerate new USB storage device")
		}
		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second, Interval: time.Second}); err != nil {
		return err
	}

	externalStorageBefore, err := typecutils.Usb3GetExternalStorageList(ctx, usbClient)
	if err != nil {
		return errors.Wrap(err, "could not get external storage list after hotplug")
	}

	// Reboot the DUT.
	if err := d.Reboot(ctx); err != nil {
		return errors.Wrap(err, "failed to reboot DUT")
	}

	if err := testing.Poll(ctx, d.Connect, &testing.PollOptions{Timeout: time.Minute}); err != nil {
		return errors.Wrap(err, "failed to re-connect to the DUT after reboot")
	}

	cl, err = rpc.Dial(ctx, d, s.RPCHint())
	if err != nil {
		return errors.Wrap(err, "failed to connect to the RPC service on the DUT")
	}
	defer cl.Close(ctx)
	usbClient = usb.NewSysfsServiceClient(cl.Conn)

	// Get device count after rebooting.
	externalStorageAfter, err := typecutils.Usb3GetExternalStorageList(ctx, usbClient)
	if err != nil {
		return errors.Wrap(err, "could not get external storage device list after reboot")
	}
	if len(externalStorageAfter) < len(externalStorageBefore) {
		return errors.New("external storage device failed to enumerate after reboot")
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
		devices, err := typecutils.Usb3GetExternalStorageList(ctx, usbClient)
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
