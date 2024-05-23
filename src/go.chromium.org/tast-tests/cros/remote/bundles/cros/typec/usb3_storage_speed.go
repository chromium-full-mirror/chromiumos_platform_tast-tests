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
		Func:     Usb3StorageSpeed,
		Desc:     "Checks data transfer speed with a USB 3.X mass storage device",
		Contacts: []string{"chromeos-usb-champs@google.com", "pmalani@chromium.org", "jthies@google.com"},
		// ChromeOS > Platform > Technologies > USB
		BugComponent: "b:958036",
		Attr:         []string{"group:typec", "typec_usb_bringup"},
		Vars:         []string{"typec.McciSerial", "typec.McciPort"},
		ServiceDeps:  []string{"tast.cros.usb.SysfsService"},
		Timeout:      10 * time.Minute,
	})
}

// WARNING: This test will corrupt the filesystem on the external storage device.

// Usb3StorageSpeed does the following:
//
// - Unmount any removable media.
// - Disconnect the USB 3.X mass storage device via MCCI switch.
// - Reconnect the USB 3.X mass storage device via MCCI switch.
// - Verify that at least one external USB 3.X mass storage device is connected to the DUT.
// - Writes 250000 blocks of random data to a newly enumerated block device (1 GB for typical flash drives).
// - Confirms the data transfer speed is not unexpectedly slow.
//
// This test expects the following hardware topology.
//
//        - network -
//       /           \
//      /             \
//     Host -------- DUT ----- MCCI (`portUsed`) ---- USB 3.X mass storage (can be connected via dock or adapter).
//     |                              |
//     |______________________________|
func Usb3StorageSpeed(ctx context.Context, s *testing.State) {
	const numIterations = 10
	const minPassingSpeed = 60 // MB/s

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
		if err := performUsb3StorageSpeedIteration(ctx, d, usbClient, sw, portUsed, minPassingSpeed); err != nil {
			s.Fatalf("Failed test on iteration %d: %v", i, err)
		}
	}
}

// performUsb3StorageSpeedIteration runs 1 iteration of the USB 3.X storage speed test.
func performUsb3StorageSpeedIteration(ctx context.Context, d *dut.DUT, cl usb.SysfsServiceClient, sw *mcci.Switch, mcciPort int, minPassingSpeed float64) error {
	// Disable the switch.
	sw.DisablePorts()

	// GoBigSleepLint: Give enough time for the DUT to process device disconnection.
	if err := testing.Sleep(ctx, 5*time.Second); err != nil {
		return errors.Wrap(err, "failed to sleep for USB disconnection")
	}

	blockDevsBefore, err := typecutils.ListBlockDevices(ctx, d)
	if err != nil {
		return errors.Wrap(err, "could not get details on block devices before hotplug")
	}

	// Enable the switch.
	sw.EnablePort(mcciPort)

	// GoBigSleepLint: Give enough time for the DUT to enumerate new USB devices.
	if err := testing.Sleep(ctx, 12*time.Second); err != nil {
		return errors.Wrap(err, "failed to sleep for USB enumeration")
	}

	// Check that an external storage device enumerated.
	if externalStorage, err := typecutils.Usb3GetExternalStorageList(ctx, cl); err != nil {
		return errors.Wrap(err, "could not get external storage list after hotplug")
	} else if len(externalStorage) == 0 {
		return errors.New("failed to enumerate new USB 3.X storage device")
	}

	blockDevsAfter, err := typecutils.ListBlockDevices(ctx, d)
	if err != nil {
		return errors.Wrap(err, "could not get details on block devices after hotplug")
	}

	if len(blockDevsAfter) != (len(blockDevsBefore) + 1) {
		return errors.New("failed due to unexpected number of block devices after hotplug")
	}

	// Find a test device that enumerated after enabling the switch.
	testDev := ""
	for _, d := range blockDevsAfter {
		if !slices.Contains(blockDevsBefore, d) {
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
	} else if speed, err := strconv.ParseFloat(string(ddResult[1]), 64); err != nil {
		return errors.Wrap(err, "failed to parse transfer speed")
	} else if speed < minPassingSpeed {
		return errors.New("failed due to transfer speed: " + string(ddResult[1]) + "MB/s")
	}

	return nil
}
