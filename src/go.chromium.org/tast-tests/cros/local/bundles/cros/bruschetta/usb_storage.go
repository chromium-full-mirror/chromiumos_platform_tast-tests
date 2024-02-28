// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bruschetta

import (
	"context"
	"regexp"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/common/usbdevice"
	"go.chromium.org/tast-tests/cros/local/bruschetta"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/guestos"
	"go.chromium.org/tast-tests/cros/local/usbutil"
	"go.chromium.org/tast-tests/cros/local/vm"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	// Constants for a virtual USB storage device created by g_mass_storage
	productName = "Linux-USB File-backed Storage Gadget"
	deviceFile  = "/dev/disk/by-id/usb-Linux_File-Stor_Gadget-0:0"
	sizeInMB    = 32
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         USBStorage,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "USB storage functional tests for the bruschetta VM",
		Contacts:     []string{"cros-virt-devices-guests@google.com", "chibar@google.com"},
		SoftwareDeps: []string{"chrome", "vm_host", "untrusted_vm", "dlc", "amd64"},
		HardwareDeps: bruschetta.BruschettaHwDeps,
		Attr:         []string{"group:mainline"},
		BugComponent: "b:1248538",
		Fixture:      bruschetta.BruschettaFixture,
		Timeout:      7 * time.Minute,
	})
}

func USBStorage(ctx context.Context, s *testing.State) {
	bru := s.FixtValue().(bruschetta.FixtureData).BruschettaVM
	cr := s.FixtValue().(bruschetta.FixtureData).Chrome
	const usbOperationTimeout = 15 * time.Second

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	hash, err := cryptohome.UserHash(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to get user hash: ", err)
	}

	// Create a virtual USB mass storage device
	usbMassStorage := usbdevice.NewUSBMassStorage()
	if err := usbMassStorage.Init(ctx, sizeInMB); err != nil {
		s.Fatal("Failed to set up a virtual USB storage: ", err)
	}
	if err := usbMassStorage.PlugIn(ctx, false /*readOnly*/); err != nil {
		s.Fatal("Failed to plug in a virtual USB storage: ", err)
	}
	defer usbMassStorage.CleanUp(cleanupCtx)

	attachedDevices, err := usbutil.AttachedDevices(ctx)
	if err != nil {
		s.Fatal("Failed to run command: ", err)
	}

	// Check if the device is attached to the host
	var device *usbutil.Device
	for i := range attachedDevices {
		if attachedDevices[i].ProductName == productName {
			device = &attachedDevices[i]
			break
		}
	}
	if device == nil {
		s.Fatal("Failed to find the virtual USB storage")
	}

	// Attach the device to bruschetta
	// This command must be run by chronos user due to permission reason
	cmd, err := testexec.CommandContextUser(ctx, "chronos",
		"vmc", "usb-attach", vm.DefaultBruschettaVMName,
		device.BusNumber+":"+device.DevNumber)
	if err != nil {
		s.Fatal("Failed to create command: ", err)
	}

	// Add an environment variable for vmc
	cmd.Cmd.Env = append(cmd.Environ(), "CROS_USER_ID_HASH="+hash)

	output, err := cmd.Output(testexec.DumpLogOnError)
	if err != nil {
		s.Fatal("Failed to run command: ", err)
	}
	outs := string(output)
	if !strings.Contains(outs, "attached to vm "+vm.DefaultBruschettaVMName) {
		s.Fatal("Failed to attach the device")
	}

	// Find the attached port for later detach
	reAttach := regexp.MustCompile(`port=([0-9]+)`)
	m := reAttach.FindStringSubmatch(outs)
	if m == nil {
		s.Fatal("Failed to find the attached port")
	}
	port := m[1]

	// Check if the device is attached to bruschetta and find its device file
	// It takes a few seconds until the device is available on the VM
	if err := testing.Poll(ctx, func(context.Context) error {
		if err := guestos.FindUSBDeviceInVM(ctx, bru, device); err != nil {
			return errors.Wrap(err, "failed to find the device")
		}
		if err := bru.Command(ctx, "ls", deviceFile).Run(); err != nil {
			return errors.Wrap(err, "failed to find the device file")
		}
		return nil

	}, &testing.PollOptions{Timeout: usbOperationTimeout, Interval: 1 * time.Second}); err != nil {
		s.Fatal("Failed to find the device in the VM: ", err)
	}

	// Run the read/write test on the device
	if err := guestos.VMStorageReadWriteTest(ctx, bru, deviceFile, "/tmp/usb_test.bin", 1); err != nil {
		s.Fatal("Read and write test failed: ", err)
	}

	// Detach the device from bruschetta
	// This command must be run by chronos user due to permission reason
	cmd, err = testexec.CommandContextUser(ctx, "chronos",
		"vmc", "usb-detach", vm.DefaultBruschettaVMName, port)
	if err != nil {
		s.Fatal("Failed to create command: ", err)
	}

	// Add an environment variable for vmc
	cmd.Cmd.Env = append(cmd.Environ(), "CROS_USER_ID_HASH="+hash)

	if err := cmd.Run(testexec.DumpLogOnError); err != nil {
		s.Fatal("Failed to detach the device: ", err)
	}

	// Check if the device isn't attached to bruschetta anymore
	// It takes a few seconds until the device is detached from the VM
	if err := testing.Poll(ctx, func(context.Context) error {
		if err := guestos.FindUSBDeviceInVM(ctx, bru, device); err == nil {
			return errors.New("the device is found in the VM")
		}
		return nil

	}, &testing.PollOptions{Timeout: usbOperationTimeout, Interval: 1 * time.Second}); err != nil {
		s.Fatal("Failed to detach the device: ", err)
	}
}
