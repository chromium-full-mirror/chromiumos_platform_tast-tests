// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package typec

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/typecutils"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/typec/typecswitch"
	"go.chromium.org/tast-tests/cros/services/cros/usb"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     Usb3StorageReboot,
		Desc:     "Check that a USB mass storage device enumerates successfully when rebooting",
		Contacts: []string{"chromeos-usb-champs@google.com", "bszpila@google.com", "jthies@google.com"},
		// ChromeOS > Platform > Technologies > USB
		BugComponent: "b:958036",
		Attr:         []string{"group:typec"},
		Vars:         []string{"typec.McciSerial", "typec.McciPort", "typec.McciPath", "typec.UnigrafUri"},
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
// - Unmount any removable media.
// - Toggle USB mass storage device connectivity via the MCCI switch.
// - Count the number of currently connected USB mass storage devices.
// - Reboot the DUT.
// - Check that there is the same number of USB mass storage devices as there were before rebooting.
//
// This test expects the following hardware topology:
//
//	 ____network___
//	|              |
//	|              |
//	Host -------- DUT ----- MCCI (`portUsed`) ---- USB mass storage (can be connected via dock or adapter).
//	|                              |
//	|______________________________|
func Usb3StorageReboot(ctx context.Context, s *testing.State) {

	numIterations := s.Param().(int)
	d := s.DUT()

	s.Log("Number of iterations: ", numIterations)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 3*time.Second)
	defer cancel()

	sw, err := typecswitch.GetSwitch(ctx, s)
	if err != nil {
		s.Fatal("Failed to get MCCI switch handle: ", err)
	}
	defer sw.Close(cleanupCtx)

	if err := typecutils.UnmountRemovableMedia(ctx, d); err != nil {
		s.Fatal("Failed to unmount removable media: ", err)
	}

	// Dial rpc
	cl, err := rpc.Dial(ctx, d, s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	usbClient := usb.NewSysfsServiceClient(cl.Conn)

	// Make sure the device is disconnected before testing
	testPort, err := sw.TestPort(ctx)
	if err != nil {
		s.Fatal("Could not get active port before testing")
	}

	if devicePort, err := sw.DevicePort(ctx); err != nil {
		s.Fatal("Could not get used port before testing: ", err)
	} else if devicePort == testPort {
		devicesWhenOn, err := typecutils.Usb3GetExternalStorageList(ctx, usbClient)
		if err != nil {
			s.Fatal("Could not get storage device list before testing: ", err)
		}
		if err := sw.DisablePorts(ctx); err != nil {
			s.Fatal("Could not disable the port before testing: ", err)
		}
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			if devices, err := typecutils.Usb3GetExternalStorageList(ctx, usbClient); err != nil {
				return errors.Wrap(err, "could not get storage device list before testing")
			} else if len(devices) >= len(devicesWhenOn) {
				return errors.New("failed to disconnect USB device")
			}
			return nil
		}, &testing.PollOptions{Timeout: 10 * time.Second, Interval: time.Second}); err != nil {
			s.Fatal("Failed to disconnect the device before the test: ", err)
		}

	} else if err := sw.DisablePorts(ctx); err != nil {
		s.Fatal("Could not disable the port before testing: ", err)
	}

	cl.Close(ctx)

	for i := 1; i <= numIterations; i++ {
		s.Log("Running iteration ", i)
		if err := performUsb3StorageRebootIteration(ctx, s, d, sw); err != nil {
			s.Fatalf("Failed test on iteration %d: %v", i, err)
		}
	}
}

// performUsb3StorageRebootIteration runs 1 iteration of the USB 3.X storage reboot test.
func performUsb3StorageRebootIteration(ctx context.Context, s *testing.State, d *dut.DUT, sw typecswitch.Switch) error {
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
