// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package typec

import (
	"bytes"
	"context"
	"strconv"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

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
		Func:     Usb3StorageSuspend,
		Desc:     "Check that a USB mass storage device remains enumerated during suspend/resume",
		Contacts: []string{"chromeos-usb-champs@google.com", "pmalani@chromium.org", "jthies@google.com"},
		// ChromeOS > Platform > Technologies > USB
		BugComponent: "b:958036",
		Attr:         []string{"group:typec", "typec_usb_bringup"},
		Vars:         []string{"typec.McciSerial", "typec.McciPort"},
		ServiceDeps:  []string{"tast.cros.usb.SysfsService"},
		Timeout:      8 * time.Minute,
	})
}

// Usb3StorageSuspend does the following:
//
// - Unmount any removable media.
// - Toggle USB mass storage device connection via MCCI switch.
// - Verify that one or more external USB mass storage devices is connected to the DUT.
// - Suspend/Resume the DUT.
// - Check that the USB mass storage device(s) are still connected and have not re-enumerated.
//
// This test expects the following hardware topology:
//
//        - network -
//       /           \
//      /             \
//     Host -------- DUT ----- MCCI (`portUsed`) ---- USB mass storage (can be connected via dock or adapter).
//     |                              |
//     |______________________________|
func Usb3StorageSuspend(ctx context.Context, s *testing.State) {
	const numIterations = 10

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
		if err := performUsb3StorageSuspendIteration(ctx, d, usbClient, sw, portUsed); err != nil {
			s.Fatalf("Failed test on iteration %d: %v", i, err)
		}
	}
}

// performUsb3StorageSuspendIteration runs 1 iteration of the USB 3.X storage suspend test.
func performUsb3StorageSuspendIteration(ctx context.Context, d *dut.DUT, cl usb.SysfsServiceClient, sw *mcci.Switch, mcciPort int) error {
	const suspendDurationS = 10

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

	// Get the initial USB device state.
	initialDeviceMap, err := cl.GetDevices(ctx, &empty.Empty{})
	if err != nil {
		return errors.Wrap(err, "failed to get USB devices before suspend")
	}

	// Create a list of external USB 3.X mass storage devices connected to the DUT.
	deviceWatchList, err := typecutils.Usb3GetExternalStorageList(ctx, cl)
	if err != nil {
		return errors.Wrap(err, "failed to get external storage list before suspend")
	}

	if len(deviceWatchList) == 0 {
		return errors.Wrap(err, "failed to find valid external USB mass storage device")
	}

	// Suspend the DUT.
	done := make(chan error, 1)
	go func(ctx context.Context) {
		defer close(done)
		_, err := d.Conn().CommandContext(ctx, "powerd_dbus_suspend", "--timeout=120", "--suspend_for_sec="+strconv.Itoa(suspendDurationS)).CombinedOutput()
		done <- err
	}(ctx)

	if err := d.WaitUnreachable(ctx); err != nil {
		return errors.Wrap(err, "could not verify DUT is unreachable after suspend")
	}

	// Wait for the DUT to resume.
	if err := <-done; err != nil {
		if !bytes.Contains([]byte(err.Error()), []byte("remote command exited without exit status")) {
			return errors.Wrap(err, "failed on powerd_dbus_suspend error")
		}
	}

	// Get the current USB device state.
	currentDeviceMap, err := cl.GetDevices(ctx, &empty.Empty{})
	if err != nil {
		return errors.Wrap(err, "failed to get USB devices after suspend")
	}

	// Confirm all external USB 3.X storage devices are present and have not re-enumerated.
	for _, d := range deviceWatchList {
		if _, present := currentDeviceMap.Devices[d]; !present {
			return errors.New("could not find expected device in current USB device map")
		}
		if initialDeviceMap.Devices[d].GetDevnum() != currentDeviceMap.Devices[d].GetDevnum() {
			return errors.New("devnum changed during suspend/resume")
		}
	}

	return nil
}
