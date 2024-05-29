// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package ptsworld contains PTSWorld access functions for crospts. The PTSworld
// is the chroot contains the phoronix-test-suite.
package ptsworld

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	// CrosChroot is the chroot directory for PTSWorld.
	CrosChroot string = WorkDir + "/chroot"
	// crosData is the data directory for PTSWorld extracted data.
	crosData string = WorkDir + "/data"
	// CrosResultsDir is the directory for test results of CrOS.
	CrosResultsDir string = ResultsDir + "/cros"
	// chrootEtc is the directory of /etc in chroot.
	chrootEtc string = CrosChroot + "/etc"
	// installed image for installing benchmark tools
	installedImageSize uint64 = 15 * 1024 * 1024 * 1024
	installedImage     string = crosData + "/installed.img"
)

const (
	// mountTimeout is the time to mount PTSWorld.
	mountTimeout = 3 * time.Second
	// unMountTimeout is the time to unmount PTSWorld.
	unMountTimeout = 3 * time.Second
)

// CrosFixture is a fixture setup for CrOS PtsWorld.
type CrosFixture struct {
	// ptsType is the host OS type for PTSWorld.
	ptsType PtsType
	// baseImage is the base image for PTSWorld.
	baseImage string
	// dataImage is the data image for PTSWorld.
	dataImage string
	// resultsDir is the directory for test results.
	resultsDir string
	// mountSequence is the file system mounting sequence.
	mountSequence []MountAttrs
}

// NewCrosFixture creates a new CrosFixture.
func NewCrosFixture(baseImage, dataImage string) *CrosFixture {
	return &CrosFixture{
		ptsType:   TypeCros,
		baseImage: baseImage,
		dataImage: dataImage,
	}
}

// MountAttrs is a structure for file system mounting attributes.
type MountAttrs struct {
	// src is the source folder or image to be mounted.
	src string
	// dst is the destination path to be mounted.
	dst string
	// fs is the file system type if required.
	fs string
	// ro is the mount as read-only.
	ro bool
	// bind is the bind mount flag.
	bind bool
	// copy is used to make a copy of the  files from dst to src before mount,
	// as ChromeOS images don't have overlayfs enabled.
	copy bool
}

// Prepare unpacks images and create mount sequence.
func (c *CrosFixture) Prepare(ctx context.Context, s *testing.FixtState) error {
	baseImageTarball := s.DataPath(c.baseImage)
	dataImageTarball := s.DataPath(c.dataImage)
	baseImage := filepath.Join(crosData, strings.TrimSuffix(c.baseImage, ".tar.xz"))
	dataImage := filepath.Join(crosData, strings.TrimSuffix(c.dataImage, ".tar.xz"))

	if err := os.MkdirAll(crosData, fs.ModeDir); err != nil {
		s.Fatalf("Failed to create directory in %v: %v", crosData, err)
		return err
	}

	if err := os.MkdirAll(CrosResultsDir, fs.ModeDir); err != nil {
		s.Fatalf("Failed to create directory in %v: %v", CrosResultsDir, err)
		return err
	}

	c.resultsDir = CrosResultsDir

	// TODO(darrenwu): check the checksum instead of file exist.
	_, err := os.Stat(baseImage)
	if os.IsNotExist(err) {
		UnpackImage(ctx, baseImageTarball, crosData)
	}
	_, err = os.Stat(dataImage)
	if os.IsNotExist(err) {
		UnpackImage(ctx, dataImageTarball, crosData)
	}

	c.mountSequence = []MountAttrs{
		{
			src: baseImage,
			dst: CrosChroot,
			fs:  "ext4",
			ro:  true,
		}, {
			src: "tmpfs",
			dst: CrosChroot + "/tmp",
			fs:  "tmpfs",
		}, {
			src: "tmpfs",
			dst: CrosChroot + "/tmp/var/lib",
			fs:  "tmpfs",
		}, {
			// The base image is mounted as read-only, but the
			// Phoronix-Test-Suite requires the var/lib as writable. Here we
			// mount var/lib as writable in tmpfs.
			src:  CrosChroot + "/tmp/var/lib",
			dst:  CrosChroot + "/var/lib",
			bind: true,
			copy: true,
		}, {
			src: "tmpfs",
			dst: CrosChroot + "/tmp/etc",
			fs:  "tmpfs",
		}, {
			// Phoronix-Test-Suite requires /etc as writable. Here we mount /etc
			// as writable in tmpfs.
			src:  CrosChroot + "/tmp/etc",
			dst:  chrootEtc,
			bind: true,
			copy: true,
		}, {
			src: "tmpfs",
			dst: CrosChroot + "/tmp/var/cache",
			fs:  "tmpfs",
		}, {
			// Phoronix-Test-Suite requires /var/cache as writable. Here we
			// mount /var/cache as writable in tmpfs.
			src:  CrosChroot + "/tmp/var/cache",
			dst:  CrosChroot + "/var/cache",
			bind: true,
			copy: true,
		}, {
			src: dataImage,
			dst: CrosChroot + PtsDir,
			fs:  "ext4",
		}, {
			src: installedImage,
			dst: CrosChroot + PtsDir + "/installed-tests",
			fs:  "ext4",
		},
		{
			src: "tmpfs",
			dst: CrosChroot + "/run",
			fs:  "tmpfs",
		}, {
			src: "devtmpfs",
			dst: CrosChroot + "/dev",
			fs:  "devtmpfs",
		}, {
			src:  DevPTS,
			dst:  CrosChroot + "/dev/pts",
			bind: true,
		}, {
			src: "proc",
			dst: CrosChroot + "/proc",
			fs:  "proc",
		}, {
			src:  "sysfs",
			dst:  CrosChroot + "/sys",
			fs:   "sysfs",
			ro:   false,
			bind: false,
		}, {
			src:  CrosResultsDir,
			dst:  CrosChroot + PtsResultsDir,
			bind: true,
		},
	}

	return nil
}

// getPtsSystemProperties gets the system properties by PTS command
// `/phoronix-test-suite/phoronix-test-suite system-properties`.
func getPtsSystemProperties(ctx context.Context) (string, string, error) {
	ptsWorldCmd := testexec.CommandContext(ctx, "env", "-i", "/usr/bin/chroot", CrosChroot, "/usr/bin/bash", "-c", "/phoronix-test-suite/phoronix-test-suite system-properties")
	stdoutBuf := new(bytes.Buffer)
	stderrBuf := new(bytes.Buffer)
	ptsWorldCmd.Stdout = stdoutBuf
	ptsWorldCmd.Stderr = stderrBuf

	if err := ptsWorldCmd.Run(); err != nil {
		return stdoutBuf.String(), stderrBuf.String(), errors.Wrap(err, "failed to start PTSWorld shell command to get system-properties")
	}

	return stdoutBuf.String(), stderrBuf.String(), nil
}

// blockTestsReinstall updates the system hash in pts-install.json to block the
// test packages reinstall for the case that system hash ID string is not match
func blockTestsReinstall(ctx context.Context, testsList []string) error {
	// Get the system properties by running PTS system-properties command.
	systemProperties, stderr, err := getPtsSystemProperties(ctx)
	if err != nil {
		return errors.Wrapf(err, "failed to get PTS system properties: %s", stderr)
	}

	// Get the system id string in based64 encoded format.
	systemIDStr := GetPtsSystemIDString(systemProperties)

	for _, test := range testsList {
		file := CrosChroot + "/var/lib/phoronix-test-suite/installed-tests/pts/" + test + "/pts-install.json"
		if _, err := os.Stat(file); err == nil {
			content, err := os.ReadFile(file)
			if err != nil {
				return errors.Wrapf(err, "failed to read: %s", file)
			}

			var data map[string]interface{}
			err = json.Unmarshal(content, &data)
			if err != nil {
				return errors.Wrapf(err, "failed to unmarshal: %s", file)
			}

			// Update system_hash by system ID
			environment := data["test_installation"].(map[string]interface{})["environment"].(map[string]interface{})
			environment["system_hash"] = systemIDStr

			updatedJSON, err := json.MarshalIndent(data, "", "    ") // Indent for readability
			if err != nil {
				return errors.Wrapf(err, "failed to marshal indent: %s", file)
			}

			err = os.WriteFile(file, updatedJSON, 0644)
			if err != nil {
				return errors.Wrapf(err, "failed to write back: %s", file)
			}
		}
	}

	return nil
}

// allocateInstalledImageFile allocates a image file for installing benchmark tools
func allocateInstalledImageFile(ctx context.Context, imageFile string) error {
	// check imageFile exist
	if _, err := os.Stat(imageFile); err == nil {
		return nil
	}

	// Create the directory if not exist
	if _, err := os.Create(imageFile); err != nil {
		return errors.Wrapf(err, "failed to create file: %s", imageFile)
	}

	// Create the image file
	if err := os.Truncate(imageFile, int64(installedImageSize)); err != nil {
		return errors.Wrapf(err, "failed to truncate image file: %s", imageFile)
	}
	// Format the image file with ext4 file system
	if err := testexec.CommandContext(ctx, "mkfs.ext4", "-F", imageFile).Run(); err != nil {
		return errors.Wrapf(err, "failed to format image file: %s", imageFile)
	}
	return nil
}

// Mount mounts PTSWorld for CrOS. It mounts base image to sysroot of chroot in
// read-only, and data image to a /var/lib/phoronix-test-suite. The chroot
// mounting sequence follows the mountSequence.
func (c *CrosFixture) Mount(ctx context.Context, s *testing.FixtState) error {
	shortCtx, cancel := ctxutil.Shorten(ctx, mountTimeout)
	defer cancel()

	mountPoints, err := GetMountPoints(ctx)
	if err != nil {
		s.Fatal("Failed to get mount points: ", err)
	}

	err = allocateInstalledImageFile(shortCtx, installedImage)
	if err != nil {
		s.Fatal("Failed to allocate image file: ", err)
		return err
	}

	for _, mount := range c.mountSequence {
		if _, exist := mountPoints[mount.dst]; exist {
			s.Log("Destination has been mounted: ", mount.dst)
			continue
		}
		if err := mountFS(shortCtx, s, mount.src, mount.dst, mount.fs, mount.ro, mount.bind, mount.copy); err != nil {
			s.Fatalf("Failed to mount %v to %v (ro:%v bind:%v copy:%v): %v", mount.src, mount.dst, mount.ro, mount.bind, mount.copy, err)
			return err
		}
	}

	// Write resolv.conf to enable the internet connectivity
	if err := os.WriteFile(filepath.Join(chrootEtc, "resolv.conf"), []byte("nameserver 8.8.8.8"), 0644); err != nil {
		s.Fatal("Failed to write ", filepath.Join(chrootEtc, "resolv.conf"))
		return err
	}

	// The large disk tests are the tests require more disk space which causes
	// low disk DUTs test fail, e.g. octopus.
	// Update the system hash to block test packages reinstall.
	largeDiskTests := []string{"openssl-3.1.0", "compress-lz4-1.0.0", "vpxenc-3.2.0", "tensorflow-lite-1.1.0"}
	// TODO(darrenwu): Remove the blockTestsReinstall() when split PTSWorld
	// implemented.
	// The split PTSWorld can help to customize the test packages,
	// and we can reduce the required disk size. But for now, we can block the
	// reinstallation for workaround.
	if err := blockTestsReinstall(ctx, largeDiskTests); err != nil {
		s.Fatal("Failed to update PTS system hash: ", err)
	}
	return nil
}

// Unmount unmounts PTSWorld for CrOS. It unmounts the chroot and tmp
// directories and follows the mountSequence in reverse order.
func (c *CrosFixture) Unmount(ctx context.Context, s *testing.FixtState) error {
	shortCtx, cancel := ctxutil.Shorten(ctx, unMountTimeout)
	defer cancel()

	mountPoints, err := GetMountPoints(ctx)
	if err != nil {
		s.Fatal("Failed to get mount points: ", err)
	}

	for i := len(c.mountSequence) - 1; i >= 0; i-- {
		if _, exist := mountPoints[c.mountSequence[i].dst]; !exist {
			s.Log("Destination is not mounted: ", c.mountSequence[i].dst)
			continue
		}
		if err := unmountFS(shortCtx, s, c.mountSequence[i].dst); err != nil {
			s.Fatalf("Failed to unmount %v: %v", c.mountSequence[i].dst, err)
			return err
		}
	}

	// Remove the installed image file
	if err := os.Remove(installedImage); err != nil {
		s.Fatal("Failed to remove image file: ", err)
		return err
	}

	return nil
}

// mountFS mounts a file system with options.
func mountFS(ctx context.Context, s *testing.FixtState, src, dst, filesystem string, ro, bind, copy bool) error {
	var arg []string

	if copy {
		if err := testexec.CommandContext(ctx, "sh", "-c", fmt.Sprintf("cp -rf %s/* %s", dst, src)).Run(); err != nil {
			s.Fatalf("Failed to copy files from %v to %v: %v", dst, src, err)
			return err
		}
	}

	if filesystem != "" {
		arg = append(arg, "-t", filesystem)
	}
	if ro {
		arg = append(arg, "-o", "ro")
	}
	if bind {
		arg = append(arg, "--bind")
	}
	arg = append(arg, src, dst)

	if err := os.MkdirAll(dst, fs.ModeDir); err != nil {
		s.Fatalf("Failed to create directory in %v: %v", dst, err)
		return err
	}
	s.Logf("Mounting %v to %v", src, dst)
	return testexec.CommandContext(ctx, "mount", arg...).Run()
}

// unmountFS mounts a destination file system.
func unmountFS(ctx context.Context, s *testing.FixtState, dst string) error {
	s.Log("Unmounting ", dst)
	return testexec.CommandContext(ctx, "umount", dst).Run()
}
