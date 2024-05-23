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
		Func:     Usb2HidHotplug,
		Desc:     "Check that a USB HID device enumerates successfully on hotplug",
		Contacts: []string{"chromeos-usb-champs@google.com", "pmalani@chromium.org", "jthies@google.com"},
		// ChromeOS > Platform > Technologies > USB
		BugComponent: "b:958036",
		Attr:         []string{"group:typec", "typec_usb_bringup"},
		Vars:         []string{"typec.McciSerial", "typec.McciPort"},
		ServiceDeps:  []string{"tast.cros.usb.SysfsService"},
		Timeout:      5 * time.Minute,
	})
}

// Usb2HidHotplug does the following:
//
// - Disconnect the USB HID device via MCCI switch.
// - Count the number of currently connected USB HID devices.
// - Reconnect the USB HID device via MCCI switch.
// - Verify that the number of USB HID devices connected to the DUT increased.
//
// This test expects the following hardware topology:
//
//        - network -
//       /           \
//      /             \
//     Host -------- DUT ----- MCCI (`portUsed`) ---- USB HID (can be connected via dock or adapter).
//     |                              |
//     |______________________________|
func Usb2HidHotplug(ctx context.Context, s *testing.State) {
	numIterations := 10

	d := s.DUT()

	portUsed, err := strconv.Atoi(s.RequiredVar("typec.McciPort"))
	if err != nil {
		s.Fatal("Failed to parse MCCI port commandline variable: ", err)
	}

	sw, err := mcci.GetSwitch(s.RequiredVar("typec.McciSerial"))
	if err != nil {
		s.Fatal("Failed to get MCCI switch handle: ", err)
	}
	defer sw.Close()

	cl, err := rpc.Dial(ctx, d, s.RPCHint())
	if err != nil {
		s.Fatal("Unable to connect to the RPC service on the DUT: ", err)
	}
	defer cl.Close(ctx)
	usbClient := usb.NewSysfsServiceClient(cl.Conn)

	for i := 1; i <= numIterations; i++ {
		s.Log("Running iteration ", i)
		if err := performUsb2HidHotplugIteration(ctx, d, usbClient, sw, portUsed); err != nil {
			s.Fatalf("Failed test on iteration %d: %v", i, err)
		}
	}
}

// performUsb2HidHotplugIteration runs 1 iteration of the USB 2.0 HID hotplug test.
func performUsb2HidHotplugIteration(ctx context.Context, d *dut.DUT, cl usb.SysfsServiceClient, sw *mcci.Switch, mcciPort int) error {
	// Disable the switch.
	sw.DisablePorts()

	// GoBigSleepLint: Give enough time for the DUT to process device disconnection.
	if err := testing.Sleep(ctx, 5*time.Second); err != nil {
		return errors.Wrap(err, "failed to sleep for USB disconnection")
	}

	hidDevicesBefore, err := typecutils.Usb2GetHidDeviceList(ctx, cl)
	if err != nil {
		return errors.Wrap(err, "could not get HID device list before hotplug")
	}

	// Enable the switch.
	sw.EnablePort(mcciPort)

	// GoBigSleepLint: Give enough time for the DUT to enumerate new USB devices.
	if err := testing.Sleep(ctx, 10*time.Second); err != nil {
		return errors.Wrap(err, "failed to sleep for USB enumeration")
	}

	hidDevicesAfter, err := typecutils.Usb2GetHidDeviceList(ctx, cl)
	if err != nil {
		return errors.Wrap(err, "could not get HID device list after hotplug")
	}

	if len(hidDevicesBefore) >= len(hidDevicesAfter) {
		return errors.New("failed to enumerate new USB HID device")
	}

	return nil
}
