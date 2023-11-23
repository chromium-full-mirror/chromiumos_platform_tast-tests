// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package usbdevice

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	backingFilePath = "/tmp/virtual_usb_file"
)

type usbMassStorageImpl struct {
	devicePath string
}

// NewUSBMassStorage returns an instance to help creating the virtual USB mass storage device.
// How to use:
//  1. Init
//  2. PlugIn
//  3. FormatFileSystem
//  4. (Optional) PlugOut
//  5. CleanUp
//
// Limitation: Since we use modprobe, there is at most one virtual USB mass storage device at a time.
func NewUSBMassStorage() *usbMassStorageImpl {
	return &usbMassStorageImpl{}
}

// Inits create the USB backing file and the virtual hub.
func (m *usbMassStorageImpl) Init(ctx context.Context, sizeInMb uint64) error {
	// Create the virtual root hub.
	if err := testexec.CommandContext(ctx, "modprobe", "dummy_hcd").Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to load dummy_hcd module")
	}
	if err := m.setupBackingFile(ctx, sizeInMb); err != nil {
		return errors.Wrap(err, "failed to set up backing file")
	}

	return nil
}

// setupBackingFile sets up the backing file used by the virtual USB mass storage device.
func (m *usbMassStorageImpl) setupBackingFile(ctx context.Context, sizeInMb uint64) error {
	// Create the backing file.
	if err := testexec.CommandContext(ctx, "fallocate", "--length="+strconv.FormatUint(sizeInMb, 10)+"M", backingFilePath).Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to create virtual USB file")
	}
	// Set disk label type.
	if err := testexec.CommandContext(ctx, "parted", backingFilePath, "mklabel", "gpt").Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to set disk label type")
	}
	// Create primary partition. In most case, one partition is enough.
	// For future developers: You can use "mkpart extended" to create other partition if needed.
	// But findDevicePath only reports the path of the first partition. You may also modify there.
	if err := testexec.CommandContext(ctx, "parted", "-s", backingFilePath, "mkpart", "primary", "0%", "90%").Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to create partition")
	}

	return nil
}

// PlugIn plugs in the virtual USB device by using the backing file. Once it's done, you can also observe the udev add events.
func (m *usbMassStorageImpl) PlugIn(ctx context.Context, readOnly bool) error {
	var roParam string
	if readOnly {
		roParam = "ro=1"
	}
	// It's possible to set vendor, product, serial, etc. Check https://www.kernel.org/doc/Documentation/usb/mass-storage.txt for more details.
	if err := testexec.CommandContext(ctx, "modprobe", "g_mass_storage", "file="+backingFilePath, "removable=1", roParam).Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to plug in the virtual USB device")
	}

	// Find the USB device path.
	path, err := m.findDevicePath(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to find the USB device path")
	}
	m.devicePath = path

	// Wait for the device path emerge.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		_, err := os.Stat(m.devicePath)
		return err
	}, &testing.PollOptions{Interval: time.Second, Timeout: 15 * time.Second}); err != nil {
		return errors.Wrap(err, "the device path does not exist")
	}

	return nil
}

// PlugOut plugs out the virtual USB device. You can also observe the udev remove events.
func (m *usbMassStorageImpl) PlugOut(ctx context.Context) error {
	if err := testexec.CommandContext(ctx, "modprobe", "g_mass_storage", "-r").Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to plug out the virtual USB device")
	}
	return nil
}

// FormatFileSystem formats the first partition.
// "command" for example, "mkfs.fat", "mkfs.ext4", "mkfs.ntfs".
// "arg" - arguments to pass to the command.
func (m *usbMassStorageImpl) FormatFileSystem(ctx context.Context, command string, arg ...string) error {
	arg = append(arg, m.devicePath)
	if err := testexec.CommandContext(ctx, command, arg...).Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to format the file system")
	}

	return nil
}

// CleanUp cleans the environment.
func (m *usbMassStorageImpl) CleanUp(ctx context.Context) error {
	if err := m.PlugOut(ctx); err != nil {
		return errors.Wrap(err, "failed to remove g_mass_storage module")
	}
	if err := testexec.CommandContext(ctx, "modprobe", "dummy_hcd", "-r").Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to remove dummy_hcd module")
	}
	if err := os.Remove(backingFilePath); err != nil {
		return errors.Wrap(err, "failed to remove the backing file")
	}
	return nil
}

// DevicePath returns the device path of the first partition.
func (m *usbMassStorageImpl) DevicePath() string {
	return m.devicePath
}

// findDevicePath finds the device path of the first partition.
func (m *usbMassStorageImpl) findDevicePath(ctx context.Context) (string, error) {
	partUUID, err := testexec.CommandContext(ctx, "sfdisk", "--part-uuid", backingFilePath, "1").Output(testexec.DumpLogOnError)
	if err != nil {
		return "", errors.Wrap(err, "failed to query the part-uuid")
	}

	return "/dev/disk/by-partuuid/" + strings.ToLower(strings.TrimSuffix(string(partUUID), "\n")), nil
}
