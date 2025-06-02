// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package typec

import (
	"context"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/exp/slices"

	"go.chromium.org/tast-tests/cros/common/typecutils"
	"go.chromium.org/tast-tests/cros/remote/typec/typecswitch"
	"go.chromium.org/tast-tests/cros/services/cros/usb"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     Usb3StorageSpeed,
		Desc:     "Checks data transfer speed with a USB 3.0 mass storage device",
		Contacts: []string{"chromeos-usb-champs@google.com", "bszpila@google.com", "jthies@google.com"},
		// ChromeOS > Platform > Technologies > USB
		BugComponent: "b:958036",
		Attr:         []string{"group:typec"},
		Fixture:      "typecSwitch",
		ServiceDeps:  []string{"tast.cros.usb.SysfsService"},
		Params: []testing.Param{{
			ExtraAttr: []string{"typec_usb_bringup", "typec_unigraf274"},
			Val:       10,
			Timeout:   10 * time.Minute,
		}, {
			Name:    "stress",
			Val:     50,
			Timeout: 50 * time.Minute,
		}},
	})
}

// WARNING: This test will corrupt the filesystem on the external storage device.

// Usb3StorageSpeed does the following:
//
// - Unmount any removable media.
// - Disconnect the USB 3.0 mass storage device via USB switch.
// - Reconnect the USB 3.0 mass storage device via USB switch.
// - Verify that at least one external USB 3.0 mass storage device is connected to the DUT.
// - Writes 250000 blocks of random data to a newly enumerated block device (1 GB for typical flash drives).
// - Confirms the data transfer speed is not unexpectedly slow.
//
// This test expects the following hardware topology.
//
//	 ____network___
//	|              |
//	|              |
//	Host          DUT ----- USB switch ---- USB 3.0 mass storage (can be connected via dock or adapter).
//	|                            |
//	|____________________________|
func Usb3StorageSpeed(ctx context.Context, s *testing.State) {
	const minPassingSpeed = 60 // MB/s

	d := s.DUT()
	numIterations := s.Param().(int)
	s.Log("Number of iterations: ", numIterations)

	// Get the switch from the fixture.
	fixtData, ok := s.FixtValue().(*typecswitch.FixtureData)
	if !ok {
		s.Fatal("Failed to get fixture data")
	}
	sw := fixtData.TestSwitch

	cl, err := rpc.Dial(ctx, d, s.RPCHint())
	if err != nil {
		s.Fatal("Unable to connect to the RPC service on the DUT: ", err)
	}
	defer cl.Close(ctx)
	usbClient := usb.NewSysfsServiceClient(cl.Conn)

	for i := 1; i <= numIterations; i++ {
		s.Log("Running iteration ", i)
		if err := performUsb3StorageSpeedIteration(ctx, d, usbClient, sw, minPassingSpeed); err != nil {
			s.Fatalf("Failed test on iteration %d: %v", i, err)
		}
	}
}

// performUsb3StorageSpeedIteration runs 1 iteration of the USB 3.0 storage speed test.
func performUsb3StorageSpeedIteration(ctx context.Context, d *dut.DUT, cl usb.SysfsServiceClient, sw typecswitch.Switch, minPassingSpeed float64) error {
	// Get the devices when switch is off
	devicesWhenOff, err := typecutils.Usb3GetExternalStorageList(ctx, cl)
	if err != nil {
		return errors.Wrap(err, "could not get device list before hotplug")
	}

	blockDevsWhenOff, err := typecutils.ListBlockDevices(ctx, d)
	if err != nil {
		return errors.Wrap(err, "could not get details on block devices before hotplug")
	}

	// Enable the switch.
	if err := sw.EnablePort(ctx); err != nil {
		return errors.Wrap(err, "failed to enable the port")
	}

	// Check for enumeration
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		blockDevs, err := typecutils.ListBlockDevices(ctx, d)
		if err != nil {
			return errors.Wrap(err, "could not get details on block devices after hotplug")
		}

		if len(blockDevs) <= len(blockDevsWhenOff) {
			return errors.New("failed due to unexpected number of block devices after hotplug")
		}
		return nil
	}, &testing.PollOptions{Timeout: 15 * time.Second, Interval: time.Second}); err != nil {
		return err
	}

	blockDevsWhenOn, err := typecutils.ListBlockDevices(ctx, d)
	if err != nil {
		return errors.Wrap(err, "could not get details on block devices after hotplug")
	}

	// Find a test device that enumerated after enabling the switch.
	testDev := ""
	for _, d := range blockDevsWhenOn {
		if !slices.Contains(blockDevsWhenOff, d) {
			testDev = d
		}
	}
	if testDev == "" {
		return errors.New("failed to find newly enumerated block device")
	}

	testDevPath := filepath.Join("/dev", testDev)
	blockSize, err := d.Conn().CommandContext(ctx, "blockdev", "--getbsz", testDevPath).Output()
	if err != nil {
		return errors.Wrap(err, "failed to get block size")
	}

	// Transfer random data to the block device.
	ddOut, err := d.Conn().CommandContext(ctx, "dd", "if=/dev/urandom", "of="+testDevPath, "bs="+strings.TrimSpace(string(blockSize)), "count=250000").CombinedOutput()
	if err != nil {
		return errors.Wrap(err, "failed to transfer data to block device")
	}

	// Check that the data transfer exceeds minPassingSpeed.
	ddResultRe := regexp.MustCompile(`([0-9.]+)\s+MB\/s`)
	ddResult := ddResultRe.FindSubmatch([]byte(ddOut))
	if len(ddResult) != 2 {
		return errors.New("failed due to unexpected dd output format")
	}
	if speed, err := strconv.ParseFloat(string(ddResult[1]), 64); err != nil {
		return errors.Wrap(err, "failed to parse transfer speed")
	} else if speed < minPassingSpeed {
		return errors.New("failed due to transfer speed: " + string(ddResult[1]) + "MB/s")
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
