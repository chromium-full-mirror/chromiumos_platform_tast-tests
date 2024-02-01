// Copyright 2018 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/vm"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	// TestBinaryDirPath is the directory to store test binaries which run inside ARC.
	TestBinaryDirPath = "/usr/local/libexec/arc-binary-tests"
	// ProjectIDExtMediaImage is the quota project ID for image files in the external storage.
	// Taken from android_projectid_config.h.
	ProjectIDExtMediaImage = 1003
)

// PullFile copies a file in Android to ChromeOS with adb pull.
func (a *ARC) PullFile(ctx context.Context, src, dst string) error {
	return a.device.PullFile(ctx, src, dst)
}

// PushFile copies a file in ChromeOS to Android with adb push.
func (a *ARC) PushFile(ctx context.Context, src, dst string) error {
	return a.device.PushFile(ctx, src, dst)
}

// PushFileToTmpDir copies a file in ChromeOS to Android temp directory.
// The destination path within the ARC container is returned.
func (a *ARC) PushFileToTmpDir(ctx context.Context, src string) (string, error) {
	return a.device.PushFileToTmpDir(ctx, src)
}

// AndroidDataDir returns the ChromeOS path from which /data/ can be accessed (/home/root/${USER_HASH}/android-data).
func AndroidDataDir(ctx context.Context, user string) (string, error) {
	// Cryptohome dir for the current user.
	rootCryptDir, err := cryptohome.SystemPath(ctx, user)
	if err != nil {
		return "", errors.Wrapf(err, "failed to get the cryptohome directory for user: %s", user)
	}

	// android-data dir under the cryptohome dir (/home/root/${USER_HASH}/android-data)
	return filepath.Join(rootCryptDir, "android-data"), nil
}

// PkgDataDir returns the ChromeOS path of the directory that contains user files of a given Android package (/home/root/${USER_HASH}/android-data/data/media/0/Android/data/${PKG}).
func PkgDataDir(ctx context.Context, user, pkg string) (string, error) {
	andrDataDir, err := AndroidDataDir(ctx, user)
	if err != nil {
		return "", errors.Wrap(err, "failed to get android-data path")
	}

	dataDir := filepath.Join(andrDataDir, "data/media/0/Android/data")
	if _, err := os.Stat(dataDir); err != nil {
		return "", errors.Wrapf(err, "cannot access Android data directory: %s", dataDir)
	}

	return filepath.Join(dataDir, pkg), nil
}

// ReadFile reads a file in Android file system with adb pull.
func (a *ARC) ReadFile(ctx context.Context, filename string) ([]byte, error) {
	return a.device.ReadFile(ctx, filename)
}

// WriteFile writes to a file in Android file system with adb push.
func (a *ARC) WriteFile(ctx context.Context, filename string, data []byte) error {
	return a.device.WriteFile(ctx, filename, data)
}

// FileSize returns the size of the specified file in bytes. Returns an error if the file does not exist.
// Note: In contrast to PkgFileSize, FileSize accesses files via adb commands.
func (a *ARC) FileSize(ctx context.Context, filename string) (int64, error) {
	return a.device.FileSize(ctx, filename)
}

// PkgFileSize returns the size of a specified file that belongs to a specified Android package in bytes. Returns an error if the file does not exist.
func PkgFileSize(ctx context.Context, user, pkg, filename string) (int64, error) {
	pkgDir, err := PkgDataDir(ctx, user, pkg)
	if err != nil {
		return 0, errors.Wrapf(err, "failed to get package directory for %s", pkg)
	}

	fullPath := filepath.Join(pkgDir, filename)
	info, err := os.Stat(fullPath)
	if err != nil {
		return 0, errors.Wrapf(err, "unable to access file: %s", fullPath)
	}
	return info.Size(), nil
}

// TempDir creates a temporary directory under AndroidTmpDirPath in Android,
// then returns its absolute path.
// It is caller's responsibility to remove all the contents in the directory
// after its use. One of the typical use cases will be as follows:
//
//	tmpdir, err := a.TempDir(ctx)
//	if err != nil {
//	  ... // error handling
//	}
//	defer a.RemoveAll(ctx, tmpdir)
//	... // Main code using tmpdir.
func (a *ARC) TempDir(ctx context.Context) (string, error) {
	return a.device.TempDir(ctx)
}

// RemoveAll removes all files and directories under the path in Android.
// The path must be abspath.
func (a *ARC) RemoveAll(ctx context.Context, path string) error {
	return a.device.RemoveAll(ctx, path)
}

// getARCVMCID returns the CID of ARCVM.
func getARCVMCID(ctx context.Context, user string) (int, error) {
	// Create a stub "ARCVM" object to get its metadata from Concierge.
	arcVM, err := vm.GetRunningVM(ctx, user, vm.ARC)
	if err != nil {
		return 0, err
	}

	// Safe to convert as for all intents and purposes the CID would fit within the minimum size of
	// an int.
	return int(arcVM.ContextID), nil
}

// MountSDCardPartitionOnHostWithSSHFS mounts Android's SDCard partition /storage/emulated/0
// on the host's /home/root/<hash>/android-data/data/media/0 using SSHFS.
func MountSDCardPartitionOnHostWithSSHFS(ctx context.Context, user string) error {
	androidDataDir, err := AndroidDataDir(ctx, user)
	if err != nil {
		return errors.Wrap(err, "failed to get Android data dir")
	}
	cid, err := getARCVMCID(ctx, user)
	if err != nil {
		return errors.Wrap(err, "failed to get ARCVM CID")
	}
	// On the guest side, arc-sftp-server-launcher starts SFTP server for /storage/emulated/0 at
	// port 7780 on ARC startup.
	cmd := testexec.CommandContext(
		// Use nonempty option since /home/root/<hash>/android-data/data/media/0 usually has
		// an empty Download directory.
		ctx, "sshfs", "-o", fmt.Sprintf("nonempty,vsock=%d:7780", cid), "unused:",
		filepath.Join(androidDataDir, "/data/media/0"))
	return cmd.Run(testexec.DumpLogOnError)
}

// UnmountSDCardPartitionFromHost unmounts Android's SDCard partition from the host's
// /home/root/<hash>/android-data/data/media/0.
func UnmountSDCardPartitionFromHost(ctx context.Context, user string) error {
	androidDataDir, err := AndroidDataDir(ctx, user)
	if err != nil {
		return errors.Wrap(err, "failed to get Android data dir")
	}
	cmd := testexec.CommandContext(ctx, "umount", filepath.Join(androidDataDir, "/data/media/0"))
	return cmd.Run(testexec.DumpLogOnError)
}

// PollBreakIfNotEUCLEANOnVirtioBlkData takes an error and wraps the error with testing.PollBreak
// except for when virtio-blk /data is used and the given error is EUCLEAN (structure needs
// cleaning). This is intended to be used in the closure provided to PollWithReadOnlyAndroidData.
func PollBreakIfNotEUCLEANOnVirtioBlkData(ctx context.Context, inErr error) error {
	if !errors.Is(inErr, unix.EUCLEAN) {
		return testing.PollBreak(inErr)
	}
	virtioBlkDataEnabled, err := IsVirtioBlkDataEnabled(ctx)
	if err != nil {
		// Return the given error as-is without wrapping in testing.PollBreak.
		testing.ContextLog(ctx, "Failed to check if virtio-blk /data is enabled: ", err)
		return inErr
	}
	if virtioBlkDataEnabled {
		return inErr
	}
	return testing.PollBreak(inErr)
}

// PollWithReadOnlyAndroidData is a utility function for waiting on a change inside Android /data.
// When virtio-blk /data is used, the disk image is mounted before executing the given function on
// every iteration of testing.Poll. The mount and unmount are done on every iteration because the
// content of mounted /data will not reflect the guest-side change to /data made after the mount.
// Mounting virtio-blk /data and accessing files in it from the host side is prone to fail with
// "structure needs cleaning" error (EUCLEAN) when the mount raced with some guest-side change to
// the file. In such cases, the caller should remount /data and retry the operation by returning a
// non-PollBreak error in the closure. Use PollBreakIfNotEUCLEANOnVirtioBlkData to wrap an error
// that should result in a testing.PollBreak but could be an EUCLEAN for virtio-blk /data.
//
// Example:
//
//	// Wait until |path| (in Android /data) is created, and read the content.
//	err := PollWithReadOnlyAndroidData(ctx, username, func(ctx context.Context) error {
//		if _, err := os.Stat(path); err != nil {
//			if os.IsNotExist(err) {
//				return errors.Wrap(err, "the file does not exist yet")
//			}
//			// This could be EUCLEAN if virtio-blk /data is used, but otherwise it is a fatal error.
//			return PollBreakIfNotEUCLEANOnVirtioBlkData(err)
//		}
//		// Read the content of |path| (at this point, EUCLEAN should not be observed)
//		// NOTE: Do this inside PollWithReadOnlyAndroidData rather than after the poll
//		// (with a remount), as the accesses to |path| might fail after the remount if
//		// the file could still be changed from the guest side.
//		if _, err = ReadFile(path); err != nil {
//			return testing.PollBreak(err)
//		}
//		return nil
//	}, pollOptions)
func PollWithReadOnlyAndroidData(ctx context.Context, user string, fn func(context.Context) error, pollopt *testing.PollOptions) error {
	diskPath, err := GetVirtioBlkDataDiskPath(ctx, user)
	if err != nil {
		return errors.Wrap(err, "failed to get disk path of virtio-blk /data")
	}

	return testing.Poll(ctx, func(ctx context.Context) error {
		// virtio-blk /data should be enabled if |diskPath| is not empty.
		if diskPath != "" {
			// Before mounting the virtio-blk disk image, run sync on the Android side to ensure
			// that the disk image is up-to-date.
			if err := BootstrapCommand(ctx, "/system/bin/sync").Run(testexec.DumpLogOnError); err != nil {
				// Retry on sync errors.
				return errors.Wrap(err, "failed to call sync on guest")
			}

			// Mount and unmount the disk image on every iteration of testing.Poll to ensure that
			// the Android-side changes are reflected on the host side.
			cleanupFunc, err := MountVirtioBlkDataDiskImageReadOnlyWithoutSync(ctx, user, diskPath)
			if err != nil {
				return testing.PollBreak(err)
			}
			defer cleanupFunc(ctx)
		}

		return fn(ctx)
	}, pollopt)
}

// GetVirtioBlkDataDiskPath returns an existing path to the disk image of virtio-blk /data
// (crosvm disk image or LVM volume) for the given user. Returns an empty string with a nil error
// if no disk image is found in the candidate paths.
func GetVirtioBlkDataDiskPath(ctx context.Context, user string) (string, error) {
	rootCryptDir, err := cryptohome.SystemPath(ctx, user)
	if err != nil {
		return "", errors.Wrap(err, "failed to get cryptohome root dir")
	}

	userHash, err := cryptohome.UserHash(ctx, user)
	if err != nil {
		return "", errors.Wrap(err, "failed to get user hash")
	}

	// virtio-blk disk exists at one of these paths.
	crosvmDiskPath := filepath.Join(rootCryptDir, "crosvm/YXJjdm0=.img")
	lvmVolumePath := filepath.Join("/dev/mapper/vm", fmt.Sprintf("dmcrypt-%s-arcvm", userHash[0:8]))

	diskPath := ""
	for _, path := range []string{crosvmDiskPath, lvmVolumePath} {
		if _, err := os.Stat(path); err == nil {
			diskPath = path
			break
		} else if !os.IsNotExist(err) {
			return "", errors.Wrapf(err, "failed to stat %q", path)
		}
	}

	return diskPath, nil
}

// MountVirtioBlkDataDiskImageReadOnlyIfUsed first checks if ARCVM virtio-blk /data is used
// on the device, and if that is the case, finds the path to the virtio-blk disk image
// and mounts the disk on the host's /home/root/<hash>/android-data/data as read-only.
// NOTE: This should be used with care, since mounting Android's /data on host and accessing files
// in it while ARCVM is still running has various problems; see the comment of
// PollWithReadOnlyAndroidData.
func MountVirtioBlkDataDiskImageReadOnlyIfUsed(ctx context.Context, user string) (func(context.Context), error) {
	virtioBlkDataEnabled, err := IsVirtioBlkDataEnabled(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to check if virtio-blk /data is enabled")
	}
	if !virtioBlkDataEnabled {
		// If ARCVM virtio-blk /data is not enabled, Android's /data directory is already
		// available at the host's /home/root/<hash>/android-data/data.
		return func(context.Context) {}, nil
	}

	// Before mounting the virtio-blk disk image, run sync on the Android side to ensure that
	// the disk image up-to-date.
	if err := BootstrapCommand(ctx, "/system/bin/sync").Run(testexec.DumpLogOnError); err != nil {
		return nil, errors.Wrap(err, "failed to call sync on guest")
	}

	diskPath, err := GetVirtioBlkDataDiskPath(ctx, user)
	if err != nil || diskPath == "" {
		return nil, errors.Wrap(err, "failed to get disk image path of /data")
	}
	return MountVirtioBlkDataDiskImageReadOnlyWithoutSync(ctx, user, diskPath)
}

// MountVirtioBlkDataDiskImageReadOnlyWithoutSync mounts |diskPath|, a disk image of virtio-blk
// /data, on the host's /home/root/<hash>/android-data/data as read-only.
func MountVirtioBlkDataDiskImageReadOnlyWithoutSync(ctx context.Context, user, diskPath string) (func(context.Context), error) {
	rootCryptDir, err := cryptohome.SystemPath(ctx, user)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get cryptohome root dir")
	}

	// Mount virtio-blk disk image.
	hostMountPath := filepath.Join(rootCryptDir, "/android-data/data")
	mountCmd := testexec.CommandContext(ctx, "mount", "-o", "loop,ro,noload", diskPath, hostMountPath)
	if err := mountCmd.Run(testexec.DumpLogOnError); err != nil {
		return nil, errors.Wrap(err, "failed to mount virtio-blk Android /data disk image on host")
	}
	// Return a function to unmount the image.
	cleanupFunc := func(ctx context.Context) {
		if err := testexec.CommandContext(ctx, "umount", hostMountPath).Run(testexec.DumpLogOnError); err != nil {
			testing.ContextLog(ctx, "Failed to unmount virtio-blk Android /data disk image from host: ", err)
		}
	}
	return cleanupFunc, nil
}

// MountSDCardPartitionOnHostWithSSHFSIfVirtioBlkDataEnabled first checks if virtio-blk /data is
// used on the device, and if that is the case, mounts Android's SDCard partition
// /storage/emulated/0 on the host's /home/root/<hash>/android-data/data/media/0 using SSHFS.
func MountSDCardPartitionOnHostWithSSHFSIfVirtioBlkDataEnabled(ctx context.Context, user string) (func(context.Context), error) {
	virtioBlkDataEnabled, err := IsVirtioBlkDataEnabled(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to check if virtio-blk /data is enabled")
	}
	if !virtioBlkDataEnabled {
		// If ARCVM virtio-blk /data is not enabled, Android's /data directory is already
		// available at the host's /home/root/<hash>/android-data/data.
		return func(context.Context) {}, nil
	}

	// Mount SDCard partition on the host side with SSHFS.
	if err := MountSDCardPartitionOnHostWithSSHFS(ctx, user); err != nil {
		return nil, errors.Wrap(err, "failed to mount Android's SDCard partition on host")
	}
	cleanupFunc := func(ctx context.Context) {
		if err := UnmountSDCardPartitionFromHost(ctx, user); err != nil {
			testing.ContextLog(ctx, "Failed to unmount Android's SDCard partition from host: ", err)
		}
	}
	return cleanupFunc, nil
}
