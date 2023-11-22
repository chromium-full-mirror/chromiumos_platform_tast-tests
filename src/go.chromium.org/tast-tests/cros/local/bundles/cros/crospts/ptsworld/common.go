// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ptsworld

import (
	"context"

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
