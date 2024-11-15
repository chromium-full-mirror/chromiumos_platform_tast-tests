// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"os"
	"path/filepath"
	"time"
	"unsafe"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"

	"golang.org/x/exp/slices"
	"golang.org/x/sys/unix"
)

// This list should be in sync with the chrome side list which is defined in
// chromeos/ash/components/login/readahead/login_readahead_performer.cc.
var readaheadTargets = []string{
	"/opt/google/chrome/chrome",
	"/opt/google/chrome/resources.pak",
	"/opt/google/chrome/chrome_100_percent.pak",
	"/opt/google/chrome/chrome_200_percent.pak",
	"/usr/share/fonts/noto/NotoSans-Regular.ttf",
	"/usr/share/fonts/roboto/Roboto-Medium.ttf",
	"/usr/share/fonts/roboto/Roboto-Regular.ttf",
}

const (
	// We expect the readahead size is at least 128KiB in
	// chromeos/ash/components/login/readahead/login_readahead_performer.cc.
	expectedReadaheadWindowSizeKb = 128
)

func init() {
	testing.AddTest(&testing.Test{
		Func: LoginPerfReadahead,
		Desc: "Verify readahead for ChromeOS login is working as expected",
		Contacts: []string{
			"cros-sw-perf@google.com",
			"xiyuan@google.com",
			"junis@google.com",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Attr:         []string{"group:mainline", "informational"},
		Timeout:      1 * time.Minute,
	})
}

func LoginPerfReadahead(ctx context.Context, s *testing.State) {
	if err := checkReadaheadList(ctx); err != nil {
		s.Error("Some files in the readahead list are missing: ", err)
	}

	if err := checkReadaheadSize(ctx); err != nil {
		s.Error("Failed to verify readahead size: ", err)
	}
}

// checkReadaheadList verifies that all expected files exist on the device.
func checkReadaheadList(ctx context.Context) error {
	for _, path := range readaheadTargets {
		info, err := os.Stat(path)
		if err != nil {
			return err
		}

		if !info.Mode().IsRegular() {
			return errors.Errorf("%s is not regular file", path)
		}

		// Files must be readable by chronos.
		if info.Mode().Perm()&0004 == 0 {
			return errors.Errorf("%s is not readable", path)
		}
	}
	return nil
}

type deviceID struct {
	major uint32
	minor uint32
}

// checkReadaheadSize verifies that the read ahead window is not smaller than our expectation.
func checkReadaheadSize(ctx context.Context) error {
	var deviceIds []deviceID
	for _, path := range readaheadTargets {
		id, err := findContainingDevice(path)
		if err != nil {
			return errors.Wrapf(err, "failed to identify block device for %s", path)
		}
		if !slices.Contains(deviceIds, id) {
			deviceIds = append(deviceIds, id)
		}
	}

	for _, id := range deviceIds {
		devicePath, err := findDevicePath(id)
		if err != nil {
			return errors.Wrapf(err, "failed to find device with major:minor=%d:%d", id.major, id.minor)
		}

		sizeKb, err := readaheadSizeKbForDevice(devicePath)
		if err != nil {
			return errors.Wrapf(err, "failed to get readahead size for device=%s", devicePath)
		}

		testing.ContextLogf(ctx, "readahead size for %s: %d KiB", devicePath, sizeKb)

		// We use `expectedReadaheadWindowSizeKb` for readahead on chrome side. If
		// that is bigger than the actual size, the last part of the request will be
		// silently skipped by the kernel. So to keep readahead effective, we
		// should use a smaller size than `sizeKb` on chrome side.
		if sizeKb < expectedReadaheadWindowSizeKb {
			return errors.Errorf("readahead window is smaller than expected: got %d; want >= %d", sizeKb, expectedReadaheadWindowSizeKb)
		}
	}
	return nil
}

func findContainingDevice(path string) (deviceID, error) {
	var stbuf unix.Stat_t
	if err := unix.Stat(path, &stbuf); err != nil {
		return deviceID{0, 0}, err
	}

	major := unix.Major(stbuf.Dev)
	minor := unix.Minor(stbuf.Dev)
	return deviceID{major, minor}, nil
}

func findDevicePath(id deviceID) (string, error) {
	const devFs = "/dev"

	devices, err := os.ReadDir(devFs)
	if err != nil {
		return "", err
	}

	for _, dev := range devices {
		path := filepath.Join(devFs, dev.Name())

		var stbuf unix.Stat_t
		if err := unix.Stat(path, &stbuf); err != nil {
			return "", err
		}
		if unix.Major(stbuf.Rdev) == id.major && unix.Minor(stbuf.Rdev) == id.minor {
			return path, nil
		}
	}

	return "", errors.New("failed to identify device")
}

func readaheadSizeKbForDevice(path string) (int64, error) {
	dev, err := os.Open(path)
	if err != nil {
		return -1, err
	}

	var buf uint64 = 0
	_, _, errno := unix.Syscall(
		unix.SYS_IOCTL,
		uintptr(dev.Fd()),
		uintptr(unix.BLKRAGET),
		uintptr(unsafe.Pointer(&buf)),
	)
	if errno != 0 {
		return -1, errors.Errorf("failed to perform ioctl BLKRAGET: errno=%d", errno)
	}

	// BLKRAGET returns the size in sector units.
	readAheadSizeKb := buf * 512 / 1024
	return int64(readAheadSizeKb), nil
}
