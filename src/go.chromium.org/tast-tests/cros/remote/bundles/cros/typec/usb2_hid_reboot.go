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
		Contacts: []string{"chromeos-usb@google.com", "pmalani@chromium.org", "jthies@google.com"},
		// ChromeOS > Platform > Technologies > USB
		BugComponent: "b:958036",
		Attr:         []string{"group:typec", "typec_usb_bringup"},
		Vars:         []string{"typec.McciSerial", "typec.McciPort"},
		ServiceDeps:  []string{"tast.cros.usb.SysfsService"},
		Timeout:      12 * time.Minute,
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
//        - network -
//       /           \
//      /             \
//     Host -------- DUT ----- MCCI (`portUsed`) ---- USB HID (can be connected via dock or adapter).
//     |                              |
//     |______________________________|
func Usb2HidReboot(ctx context.Context, s *testing.State) {
	numIterations := 5

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

	for i := 1; i <= numIterations; i++ {
		s.Log("Running iteration ", i)
		if err := performUsb2HidRebootIteration(ctx, s, d, sw, portUsed); err != nil {
			s.Fatalf("Failed test on iteration %d: %v", i, err)
		}
	}
}

// performUsb2HidRebootIteration runs 1 iteration of the USB 2.0 HID reboot test.
func performUsb2HidRebootIteration(ctx context.Context, s *testing.State, d *dut.DUT, sw *mcci.Switch, mcciPort int) error {
	if err := typecutils.UnmountRemovableMedia(ctx, d); err != nil {
		return errors.Wrap(err, "failed to unmount removable media")
	}

	// Disable the switch.
	sw.DisablePorts()

	// GoBigSleepLint: Give enough time for the DUT to process device disconnection.
	if err := testing.Sleep(ctx, 5*time.Second); err != nil {
		return errors.Wrap(err, "failed to sleep for USB disconnection")
	}

	// Enable the switch.
	sw.EnablePort(mcciPort)

	// GoBigSleepLint: Give enough time for the DUT to enumerate new USB devices.
	if err := testing.Sleep(ctx, 10*time.Second); err != nil {
		return errors.Wrap(err, "failed to sleep for USB enumeration")
	}

	cl, err := rpc.Dial(ctx, d, s.RPCHint())
	if err != nil {
		return errors.Wrap(err, "failed to connect to the RPC service on the DUT")
	}
	defer cl.Close(ctx)

	// Get initial device count.
	usbClient := usb.NewSysfsServiceClient(cl.Conn)
	hidDevicesBefore, err := typecutils.Usb2GetHidDeviceList(ctx, usbClient)
	if err != nil {
		return errors.Wrap(err, "could not get external HID device list before reboot")
	}

	if len(hidDevicesBefore) == 0 {
		return errors.New("could not find an external HID device before reboot")
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

	// Get device count after rebooting.
	usbClient = usb.NewSysfsServiceClient(cl.Conn)
	hidDevicesAfter, err := typecutils.Usb2GetHidDeviceList(ctx, usbClient)
	if err != nil {
		return errors.Wrap(err, "could not get external HID device list after reboot")
	} else if len(hidDevicesAfter) < len(hidDevicesBefore) {
		return errors.New("external HID device failed to enumerate after reboot")
	}

	return nil
}
