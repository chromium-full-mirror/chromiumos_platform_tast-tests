// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package usb

import (
	"bytes"
	"context"
	"strconv"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/services/cros/usb"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: SuspendStability,
		Desc: "Checks that USB device connection is maintained through suspend/resume",
		// ChromeOS > Platform > Technologies > USB
		BugComponent: "b:958036",
		Contacts:     []string{"chromeos-usb@google.com", "jthies@google.com"},
		Attr:         []string{"group:mainline", "informational"},
		ServiceDeps:  []string{"tast.cros.usb.SysfsService"},
	})
}

const (
	defaultSuspendCount    = 1
	defaultSuspendDuration = 10
	defaultMinSpeed        = 0
	defaultMaxSpeed        = 20000
)

var (
	suspendCountVar = testing.RegisterVarString(
		"usb.SuspendStability.suspendCount",
		"1",
		"Number of times the DUT suspends during the test.")

	suspendDurationVar = testing.RegisterVarString(
		"usb.SuspendStability.suspendDuration",
		"10",
		"Amount of time, in seconds, that the DUT gets suspended for each time it is suspended.")

	minSpeedVar = testing.RegisterVarString(
		"usb.SuspendStability.minSpeed",
		"0",
		"Minimum speed of USB devices that the test tracks through suspend/resume. Slower devices will not be checked.")

	maxSpeedVar = testing.RegisterVarString(
		"usb.SuspendStability.maxSpeed",
		"20000",
		"Maximum speed of USB devices that the test tracks through suspend/resume. Faster devices will not be checked.")
)

// SuspendStability checks that external USB devices are stable across suspend/resume.
func SuspendStability(ctx context.Context, s *testing.State) {
	// Setup test variables.
	suspendCount, err := strconv.Atoi(suspendCountVar.Value())
	if err != nil {
		suspendCount = defaultSuspendCount
		s.Log("Unable to parse usb.SuspendStability.suspendCount, using default suspend count")
	}

	suspendDuration, err := strconv.Atoi(suspendDurationVar.Value())
	if err != nil {
		suspendCount = defaultSuspendDuration
		s.Log("Unable to parse usb.SuspendStability.suspendCount, using default suspend duration")
	}

	minSpeed, err := strconv.ParseUint(minSpeedVar.Value(), 10, 64)
	if err != nil {
		minSpeed = defaultMinSpeed
		s.Log("Unable to parse usb.SuspendStability.minSpeed, using default minimum speed")
	}

	maxSpeed, err := strconv.ParseUint(maxSpeedVar.Value(), 10, 64)
	if err != nil {
		maxSpeed = defaultMaxSpeed
		s.Log("Unable to parse usb.SuspendStability.maxSpeed, using default maximum speed")
	}

	// Setup connection to the DUT.
	d := s.DUT()
	cl, err := rpc.Dial(ctx, d, s.RPCHint())
	if err != nil {
		s.Fatal("Unable to connect to the RPC service on the DUT: ", err)
	}
	defer cl.Close(ctx)

	// Setup USB sysfs service.
	usbClient := usb.NewSysfsServiceClient(cl.Conn)

	initialDeviceMap, err := usbClient.GetDevices(ctx, &empty.Empty{})
	if err != nil {
		s.Fatal("Unable to check initial devices")
	}

	// Build list of devices to check during suspend/resume cycles.
	var deviceWatchList []string
	for addr, device := range initialDeviceMap.Devices {
		if device.Speed >= minSpeed && device.Speed <= maxSpeed && device.Removable != usb.RemovableAttribute_REMOVABLE_ATTRIBUTE_FIXED {
			deviceWatchList = append(deviceWatchList, addr)
		}
	}

	if len(deviceWatchList) == 0 {
		s.Fatal("Abandoning test without any USB devices meeting test criteria")
	}

	s.Logf("Starting suspend test. Suspend count: %d, suspend duration: %d, device count: %d", suspendCount, suspendDuration, len(deviceWatchList))
	for i := 0; i < suspendCount; i++ {
		s.Log("Running iteration ", i)
		if err := suspendStabilityCheck(ctx, d, usbClient, initialDeviceMap, deviceWatchList, suspendDuration); err != nil {
			s.Fatalf("Failure on iteration %d: %v", i, err)
		}
	}
}

// suspendStabilityCheck will suspend/resume the DUT, then confirm that none of the devices in deviceWatchList based on device numbers in initialDeviceMap.
func suspendStabilityCheck(ctx context.Context, d *dut.DUT, usbClient usb.SysfsServiceClient, initialDeviceMap *usb.DeviceMap, deviceWatchList []string, suspendDuration int) error {
	// Setup new channel to suspend the DUT.
	done := make(chan error, 1)
	go func(ctx context.Context) {
		defer close(done)
		_, err := d.Conn().CommandContext(ctx, "powerd_dbus_suspend", "--timeout=120", "--suspend_for_sec="+strconv.Itoa(suspendDuration)).CombinedOutput()
		done <- err
	 }(ctx)

	if err := d.WaitUnreachable(ctx); err != nil {
		return errors.Wrap(err, "unable to verify DUT is unreachable after suspend")
	}

	// Wait for powerd_dbus_suspend to return output.
	if err := <-done; err != nil {
		if !bytes.Contains([]byte(err.Error()), []byte("remote command exited without exit status")) {
			return errors.Wrap(err, "failing on powerd_dbus_suspend error")
		}
	}

	currentDeviceMap, err := usbClient.GetDevices(ctx, &empty.Empty{})
	if err != nil {
		return errors.Wrap(err, "unable to create USB device map")
	}

	for _, d := range deviceWatchList {
		if _, present := currentDeviceMap.Devices[d]; !present {
			return errors.New("device missing from current device map")
		}
		if initialDeviceMap.Devices[d].GetDevnum() != currentDeviceMap.Devices[d].GetDevnum() {
			return errors.New("devnum change during suspend/resume")
		}
	}
	return nil
}
