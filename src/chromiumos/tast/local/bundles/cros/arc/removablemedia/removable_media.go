// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package removablemedia implements the testing sceanrio of arc.RemovableMedia
// test and its utilities.
package removablemedia

import (
	"context"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"chromiumos/tast/common/android/ui"
	"chromiumos/tast/common/testexec"
	"chromiumos/tast/local/arc"
	"chromiumos/tast/local/bundles/cros/arc/storage"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/crosdisks"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// createZeroFile creates a file filled with size bytes of 0.
func createZeroFile(size int64, name string) (string, error) {
	f, err := ioutil.TempFile("", name)
	if err != nil {
		return "", errors.Wrap(err, "failed to create an image file")
	}
	defer f.Close()

	if err := f.Truncate(size); err != nil {
		os.Remove(f.Name()) // Ignore error.
		return "", errors.Wrap(err, "failed to resize the image file")
	}

	return f.Name(), nil
}

// attachLoopDevice attaches to loop device.
func attachLoopDevice(ctx context.Context, path string) (string, error) {
	b, err := testexec.CommandContext(ctx, "losetup", "-f", path, "--show").Output(testexec.DumpLogOnError)
	if err != nil {
		return "", errors.Wrap(err, "failed to attach to loop device")
	}
	return strings.TrimSpace(string(b)), nil
}

// detachLoopDevice detaches from loop device.
func detachLoopDevice(ctx context.Context, devLoop string) error {
	if err := testexec.CommandContext(ctx, "losetup", "-d", devLoop).Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrapf(err, "failed to detach from loop device at %s", devLoop)
	}

	return nil
}

// formatVFAT formats the vfat file system.
func formatVFAT(ctx context.Context, devLoop string) error {
	if err := testexec.CommandContext(ctx, "mkfs", "-t", "vfat", "-F", "32", "-n", "MyDisk", devLoop).Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to format vfat file system")
	}
	return nil
}

// mount adds device to allowlist and mounts it.
func mount(ctx context.Context, cd *crosdisks.CrosDisks, devLoop, name string) (mountPath string, retErr error) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Second)
	defer cancel()
	sysPath := path.Join("/sys/devices/virtual/block", path.Base(devLoop))
	if err := cd.AddDeviceToAllowlist(ctx, sysPath); err != nil {
		return "", errors.Wrapf(err, "failed to add device %s to allowlist", sysPath)
	}
	defer func() {
		if err := cd.RemoveDeviceFromAllowlist(cleanupCtx, sysPath); err != nil && retErr != nil {
			retErr = errors.Wrapf(err, "failed to remove device %s from allowlist", sysPath)
		}
	}()
	w, err := cd.WatchMountCompleted(ctx)
	if err != nil {
		return "", err
	}
	defer w.Close(ctx)

	if err := cd.Mount(ctx, devLoop, "", []string{"rw", "nodev", "noexec", "nosuid", "sync"}); err != nil {
		return "", errors.Wrap(err, "failed to mount")
	}

	path := filepath.Join("/media/removable", name)
	for {
		m, err := w.Wait(ctx)
		if err != nil {
			return "", errors.Wrap(err, "failed to mount")
		}
		if m.SourcePath == devLoop && m.MountPath == path {
			// Target mount point is found.
			if m.Status != crosdisks.MountErrorNone {
				return "", errors.Wrap(m.Status, "failed to mount")
			}
			return path, nil
		}
	}
}

// unmount unmounts the disk.
func unmount(ctx context.Context, cd *crosdisks.CrosDisks, devLoop string) error {
	if err := cd.Unmount(ctx, devLoop, []string{"lazy"}); err != nil {
		return errors.Wrap(err, "failed to unmount")
	}
	return nil
}

// CreateAndMountImage sets up a filesystem image and mounts it via CrosDisks.
func CreateAndMountImage(ctx context.Context, imageSize int64, diskName string) (mountDir string, cleanupFunc func(context.Context), retErr error) {
	// Set up a filesystem image.
	image, err := createZeroFile(imageSize, "vfat.img")
	if err != nil {
		return "", nil, errors.Wrap(err, "failed to create image")
	}
	defer func() {
		if retErr != nil {
			os.Remove(image)
		}
	}()

	devLoop, err := attachLoopDevice(ctx, image)
	if err != nil {
		os.Remove(image)
		return "", nil, errors.Wrap(err, "failed to attach loop device")
	}
	defer func() {
		if retErr != nil {
			if err := detachLoopDevice(ctx, devLoop); err != nil {
				testing.ContextLog(ctx, "Failed to detach from loop device: ", err)
			}
		}
	}()

	if err := formatVFAT(ctx, devLoop); err != nil {
		return "", nil, errors.Wrap(err, "failed to format VFAT file system")
	}

	// Mount the image via CrosDisks.
	cd, err := crosdisks.New(ctx)
	if err != nil {
		return "", nil, errors.Wrap(err, "failed to find crosdisks D-Bus service")
	}
	mountDir, err = mount(ctx, cd, devLoop, diskName)
	if err != nil {
		return "", nil, errors.Wrap(err, "failed to mount file system")
	}

	cleanupFunc = func(ctx context.Context) {
		if err := unmount(ctx, cd, devLoop); err != nil {
			testing.ContextLog(ctx, "Failed to unmount VFAT image: ", err)
		}
		if err := detachLoopDevice(ctx, devLoop); err != nil {
			testing.ContextLog(ctx, "Failed to detach from loop device: ", err)
		}
		os.Remove(image)
	}
	return mountDir, cleanupFunc, nil
}

// RunTest executes the testing scenario of arc.RemovableMedia.
func RunTest(ctx context.Context, s *testing.State, a *arc.ARC, cr *chrome.Chrome, d *ui.Device, testFile string) {
	const (
		imageSize = 64 * 1024 * 1024
		diskName  = "MyDisk"
	)

	expected := []byte(storage.ExpectedFileContent)

	mountDir, cleanupFunc, err := CreateAndMountImage(ctx, imageSize, diskName)
	if err != nil {
		s.Fatal("Failed to set up image: ", err)
	}
	defer cleanupFunc(ctx)

	if err := arc.WaitForARCRemovableMediaVolumeMount(ctx, a); err != nil {
		s.Fatal("Failed to wait for the volume to be mounted in ARC: ", err)
	}

	// Create a text file in the removable media.
	tpath := filepath.Join(mountDir, testFile)
	if err := ioutil.WriteFile(tpath, expected, 0644); err != nil {
		s.Fatal("Failed to write a data file: ", err)
	}

	config := storage.TestConfig{DirName: diskName, SubDirectories: []string{}, FileName: testFile}
	expectations := []storage.Expectation{
		{LabelID: storage.ActionID, Value: storage.ExpectedAction},
		{LabelID: storage.URIID, Predicate: arc.VerifyContentURIForArcVolumeProviderPath(filepath.Join(arc.RemovableMediaUUID, testFile))},
		{LabelID: storage.FileContentID, Value: storage.ExpectedFileContent}}

	if err := storage.TestOpenWithAndroidApp(ctx, a, cr, d, config, expectations); err != nil {
		s.Fatal("Failed to open file with Android app: ", err)
	}
}
