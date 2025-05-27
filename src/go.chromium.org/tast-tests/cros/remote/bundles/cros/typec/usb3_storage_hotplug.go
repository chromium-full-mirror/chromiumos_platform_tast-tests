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
		Func:     Usb3StorageHotplug,
		Desc:     "Check that a USB mass storage device enumerates successfully on hotplug",
		Contacts: []string{"chromeos-usb-champs@google.com", "bszpila@google.com", "jthies@google.com"},
		// ChromeOS > Platform > Technologies > USB
		BugComponent: "b:958036",
		Attr:         []string{"group:typec"},
		Vars:         []string{"typec.McciSerial", "typec.McciPort", "typec.McciPath", "typec.UnigrafUri"},
		ServiceDeps:  []string{"tast.cros.usb.SysfsService"},
		Params: []testing.Param{{
			ExtraAttr: []string{"typec_usb_bringup", "typec_unigraf274"},
			Val:       10,
			Timeout:   5 * time.Minute,
		}, {
			Name:    "stress",
			Val:     50,
			Timeout: 25 * time.Minute,
		}},
	})
}

// Usb3StorageHotplug does the following:
//
// - Unmount any removable media.
// - Disconnect the USB mass storage device via MCCI switch.
// - Count the number of currently connected USB mass storage devices.
// - Reconnect the USB mass storage device via MCCI switch.
// - Verify that the number of USB mass storage devices connected to the DUT increased.
//
// This test expects the following hardware topology:
//
//	 ____network___
//	|              |
//	|              |
//	Host -------- DUT ----- MCCI (`portUsed`) ---- USB mass storage (can be connected via dock or adapter).
//	|                              |
//	|______________________________|
func Usb3StorageHotplug(ctx context.Context, s *testing.State) {

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

	if err = sw.EnterUsb3Mode(ctx); err != nil {
		s.Fatal("Failed to enter USB3 mode: ", err)
	}

	cl, err := rpc.Dial(ctx, d, s.RPCHint())
	if err != nil {
		s.Fatal("Unable to connect to the RPC service on the DUT: ", err)
	}
	defer cl.Close(ctx)
	usbClient := usb.NewSysfsServiceClient(cl.Conn)

	if err := typecutils.UnmountRemovableMedia(ctx, d); err != nil {
		s.Fatal("Failed to unmount removable media: ", err)
	}

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

	for i := 1; i <= numIterations; i++ {
		s.Log("Running iteration ", i)
		if err := performUsb3StorageHotplugIteration(ctx, d, usbClient, sw); err != nil {
			s.Fatalf("Failed test on iteration %d: %v", i, err)
		}
	}
}

// performUsb3StorageHotplugIteration runs 1 iteration of the USB 3.2 storage hotplug test.
func performUsb3StorageHotplugIteration(ctx context.Context, d *dut.DUT, cl usb.SysfsServiceClient, sw typecswitch.Switch) error {

	// Get the devices when switch is off
	devicesWhenOff, err := typecutils.Usb3GetExternalStorageList(ctx, cl)
	if err != nil {
		return errors.Wrap(err, "could not get external storage list before hotplug")
	}

	// Enable the switch.
	if err := sw.EnablePort(ctx); err != nil {
		return errors.Wrap(err, "failed to enable the port")
	}

	// Check for enumeration
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		devices, err := typecutils.Usb3GetExternalStorageList(ctx, cl)
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
		devices, err := typecutils.Usb3GetExternalStorageList(ctx, cl)
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
