// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ptsworld

import (
	"context"
	"os"
	"strings"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// PtsType indicates the type of host OS for crospts.
type PtsType string

const (
	// TypeCros refers to the crospts is running on top of CrOS.
	TypeCros PtsType = "cros"
)
const (
	// VarLibDir is the directory of /var/lib in chroot.
	VarLibDir string = "/var/lib"
	// PtsDir is the directory of phoronix-test-suite under /var/lib.
	PtsDir string = VarLibDir + "/phoronix-test-suite"
	// PtsResultsDir is the directory of test results folder.
	PtsResultsDir string = PtsDir + "/test-results"
	// WorkDir is the working directory for crospts.
	WorkDir string = "/usr/local/crospts"
	// ResultsDir is the directory for test results.
	ResultsDir string = WorkDir + "/test_results"
	// DevPTS is the device node of pseudo terminal.
	DevPTS string = "/dev/pts"
)

// Fixture is the interface for tast fixture.
type Fixture interface {
	// Prepare prepares the fixture.
	// It unpacks images and create mount sequence.
	Prepare(ctx context.Context, s *testing.FixtState) error
	// Mount mounts the PTSWorld as chroot.
	Mount(ctx context.Context, s *testing.FixtState) error
	// Unmount unmounts the PTSWorld chroot.
	Unmount(ctx context.Context, s *testing.FixtState) error
}

// UnpackImage unpacks the given image to the given imagePath.
func UnpackImage(ctx context.Context, image, imagePath string) error {
	testing.ContextLogf(ctx, "Unpacking %v to %v", image, imagePath)
	if err := testexec.CommandContext(ctx, "tar", "-C", imagePath, "-xvf", image).Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to unpack image")
	}
	return nil
}

// Runner is the interface for crospts runner.
type Runner interface {
	// RunTestSuite runs the given test suite in PTSWorld.
	RunTestSuite(ctx context.Context, s *testing.State, cmd string)
	// PtsWorldShell runs the given command in PTSWorld.
	PtsWorldShell(ctx context.Context, s *testing.State, cmd string)
}

// GetMountPoints returns the mount points in /proc/mounts with mount-point as
// key and true as value.
func GetMountPoints(ctx context.Context) (map[string]bool, error) {
	mountPoints := make(map[string]bool)

	mounts, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return nil, errors.Wrap(err, "failed to read /proc/mounts")
	}

	for _, line := range strings.Split(string(mounts), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		// The /proc/mounts file has the following format:
		// device mount-point type flags options dump pass
		// Example:
		// /dev/sda1 / ext4 rw,relatime,discard,errors=remount-ro 0 0
		// We put mount-point in key which can be efficiently used to check the
		// path has been mounted, the value is not used and is set to true.
		mountPoints[fields[1]] = true
	}

	return mountPoints, nil
}
