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
		Func:     Usb3StorageSuspend,
		Desc:     "Check that a USB 3.0 mass storage device remains enumerated through suspend/resume",
		Contacts: []string{"chromeos-usb-champs@google.com", "bszpila@google.com", "jthies@google.com"},
		// ChromeOS > Platform > Technologies > USB
		BugComponent: "b:958036",
		Attr:         []string{"group:typec"},
		Fixture:      "typecSwitch",
		ServiceDeps:  []string{"tast.cros.usb.SysfsService"},
		Params:       typecswitch.GenerateParams(8, 10, usbswitch.Usb3Mode, "typec_usb_bringup"),
	})
}

// Usb3StorageSuspend does the following:
//
// - Unmount any removable media.
// - Toggle USB 3.0 mass storage device connection via USB switch.
// - Verify that one or more external USB 3.0 mass storage devices is connected to the DUT.
// - Suspend/Resume the DUT.
// - Check that the USB 3.0 mass storage device(s) are still connected and have not re-enumerated.
//
// This test expects the following hardware topology:
//
//	 ____network___
//	|              |
//	|              |
//	Host          DUT ----- USB switch ---- USB mass storage (can be connected via dock or adapter).
//	|                            |
//	|____________________________|
func Usb3StorageSuspend(ctx context.Context, s *testing.State) {
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
		if err := performUsb3StorageSuspendIteration(ctx, s, d, sw); err != nil {
			s.Fatalf("Failed test on iteration %d: %v", i, err)
		}
	}
}

// performUsb3StorageSuspendIteration runs 1 iteration of the USB 3.0 storage suspend test.
func performUsb3StorageSuspendIteration(ctx context.Context, s *testing.State, d *dut.DUT, sw usbswitch.Switch) error {
	const suspendDurationS = 10

	// Dial rpc
	cl, err := rpc.Dial(ctx, d, s.RPCHint())
	if err != nil {
		return errors.Wrap(err, "unable to connect to the RPC service on the DUT")
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
		return errors.Wrap(err, "failed to switch on the port")
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

	// Create a list of external USB 3.X mass storage devices connected to the DUT.
	devicesWhenOn, err := typecutils.Usb3GetExternalStorageList(ctx, usbClient)
	if err != nil {
		return errors.Wrap(err, "could not get external storage list after hotplug")
	}

	// Get the initial USB device state.
	initialDeviceMap, err := usbClient.GetDevices(ctx, &empty.Empty{})
	if err != nil {
		return errors.Wrap(err, "failed to get USB devices before suspend")
	}

	// GoBigSleepLint: Give enough time for a new Unigraf display modeset after hot plug,
	// otherwise the system won't sleep.
	if err := testing.Sleep(ctx, 2*time.Second); err != nil {
		return errors.Wrap(err, "failed to sleep for display unplug modeset")
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
		s.Fatal("Unable to connect to the RPC service on the DUT: ", err)
	}
	defer cl.Close(ctx)
	usbClient = usb.NewSysfsServiceClient(cl.Conn)

	// Get the current USB device state.
	currentDeviceMap, err := usbClient.GetDevices(ctx, &empty.Empty{})
	if err != nil {
		return errors.Wrap(err, "failed to get USB devices after suspend")
	}

	// Confirm all external USB 3.X storage devices are present and have not re-enumerated.
	for _, d := range devicesWhenOn {
		if _, present := currentDeviceMap.Devices[d]; !present {
			return errors.New("could not find expected device in current USB device map")
		}
		if initialDeviceMap.Devices[d].GetDevnum() != currentDeviceMap.Devices[d].GetDevnum() {
			return errors.New("devnum changed during suspend/resume")
		}
	}

	// Unmount the storage
	if err := typecutils.UnmountRemovableMedia(ctx, d); err != nil {
		return errors.Wrap(err, "failed to unmount removable media")
	}

	// Disable the switch.
	if err := sw.DisablePorts(ctx); err != nil {
		return errors.Wrap(err, "failed to switch off the port")
	}

	// Check for device disconnection
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		devices, err := typecutils.Usb3GetExternalStorageList(ctx, usbClient)
		if err != nil {
			return errors.Wrap(err, "could not get HID device list after disconnection")
		}
		if len(devices) >= len(devicesWhenOn) {
			return errors.New("failed to disconnect USB HID device")
		}
		return nil
	}, &testing.PollOptions{Timeout: 5 * time.Second, Interval: time.Second}); err != nil {
		return err
	}

	return nil
}
