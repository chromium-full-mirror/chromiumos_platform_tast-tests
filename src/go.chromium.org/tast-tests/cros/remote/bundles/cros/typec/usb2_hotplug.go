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
		Func:     Usb2Hotplug,
		Desc:     "Check that a USB 2 device enumerates successfully on hotplug",
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

// Usb2Hotplug does the following:
//
// - Disconnect the USB device via MCCI switch.
// - Count the number of currently connected USB devices.
// - Reconnect the USB device via MCCI switch.
// - Verify that the number of USB devices connected to the DUT increased.
//
// This test expects the following hardware topology:
//
//	 ____network___
//	|              |
//	|              |
//	Host -------- DUT ----- MCCI (`portUsed`) ---- USB device (can be connected via dock or adapter).
//	|                              |
//	|______________________________|
func Usb2Hotplug(ctx context.Context, s *testing.State) {

	numIterations := s.Param().(int)
	d := s.DUT()

	s.Log("Number of iterations: ", numIterations)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 3*time.Second)
	defer cancel()

	sw, err := typecswitch.GetSwitch(ctx, s)
	if err != nil {
		s.Fatal("Failed to get switch handle: ", err)
	}
	defer sw.Close(cleanupCtx)

	if err = sw.EnterUsb2Mode(ctx); err != nil {
		s.Fatal("Failed to enter USB2 mode: ", err)
	}
	defer sw.EnterUsb3Mode(cleanupCtx)

	cl, err := rpc.Dial(ctx, d, s.RPCHint())
	if err != nil {
		s.Fatal("Unable to connect to the RPC service on the DUT: ", err)
	}
	defer cl.Close(ctx)
	usbClient := usb.NewSysfsServiceClient(cl.Conn)

	// Make sure the device is disconnected before testing
	testPort, err := sw.TestPort(ctx)
	if err != nil {
		s.Fatal("Could not get active port before testing: ", err)
	}
	if devicePort, err := sw.DevicePort(ctx); err != nil {
		s.Fatal("Could not get used port before testing: ", err)
	} else if devicePort == testPort {
		devicesWhenOn, err := typecutils.Usb2GetDeviceList(ctx, usbClient)
		if err != nil {
			s.Fatal("Could not get USB2 device list before hotplug: ", err)
		}
		if err := sw.DisablePorts(ctx); err != nil {
			s.Fatal("Could not disable the port before testing: ", err)
		}
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			if devices, err := typecutils.Usb2GetDeviceList(ctx, usbClient); err != nil {
				return errors.Wrap(err, "could not get USB2 device list before testing")
			} else if len(devices) >= len(devicesWhenOn) {
				return errors.New("failed to enumerate new USB device")
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
		if err := performUsb2HotplugIteration(ctx, d, usbClient, sw); err != nil {
			s.Fatalf("Failed test on iteration %d: %v", i, err)
		}
	}
}

// performUsb2HotplugIteration runs 1 iteration of the USB 2.0 hotplug test.
func performUsb2HotplugIteration(ctx context.Context, d *dut.DUT, cl usb.SysfsServiceClient, sw typecswitch.Switch) error {

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
