// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package usb

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/lsbrelease"
	"go.chromium.org/tast/core/testing"
)

// CommandRunner runs commands on a servo host or a dut.
type CommandRunner interface {
	// RunCommand execs a command, possibly as the root user.
	RunCommand(ctx context.Context, asRoot bool, name string, args ...string) error
	// RunCommandQuiet execs a command, possibly as the root user, but without logging errors.
	RunCommandQuiet(ctx context.Context, asRoot bool, name string, args ...string) error
	// OutputCommand execs a command, possibly as the root user, and returns stdout
	OutputCommand(ctx context.Context, asRoot bool, name string, args ...string) ([]byte, error)
}

// ValidateUSBImage verifies that the usb drive is mountable, and returns the version of the ChromeOS test image.
// A blank version means that the ChromeOS image on the USB drive is invalid.
func ValidateUSBImage(ctx context.Context, usbdev, mountPath string, runner CommandRunner) (version, milestone string, err error) {
	testing.ContextLog(ctx, "Checking ChromeOS image name on usbkey ", usbdev)
	// Unmount whatever might be mounted.
	runner.RunCommandQuiet(ctx, true, "umount", "-q", mountPath)

	// ChromeOS kernel is at /dev/sdx2.
	kernelPart := usbdev + "2"
	// ChromeOS root fs is in /dev/sdx3.
	mountSrc := usbdev + "3"
	if err := runner.RunCommand(ctx, true, "mkdir", "-p", mountPath); err != nil {
		return "", "", errors.Wrapf(err, "mkdir failed at %q", mountPath)
	}

	var lsb map[string]string
	// Failures here are a bad USB image, so don't fail, return false.
	err = func() error {
		if output, err := runner.OutputCommand(ctx, true, "dd", fmt.Sprintf("if=%s", kernelPart), "bs=8", "count=1"); err != nil {
			return errors.Wrap(err, "failed to read kernel magic")
		} else if bytes.Compare(output, []byte("CHROMEOS")) != 0 {
			return errors.Errorf("incorrect kernel magic string got %v want %v", output, []byte("CHROMEOS"))
		}
		if err = runner.RunCommand(ctx, true, "mount", "-o", "ro", mountSrc, mountPath); err != nil {
			return errors.Wrapf(err, "failed to mount %q at %q", mountSrc, mountPath)
		}
		defer runner.RunCommand(ctx, true, "umount", mountPath)
		output, err := runner.OutputCommand(ctx, true, "cat", fmt.Sprintf("%s/etc/lsb-release", mountPath))
		if err != nil {
			return errors.Wrap(err, "failed to read lsb-release")
		}
		lsb, err = lsbrelease.Parse(bytes.NewReader(output))
		if err != nil {
			return errors.Wrap(err, "failed to parse lsb-release")
		}
		return nil
	}()
	if err != nil {
		testing.ContextLog(ctx, "Bad USB image: ", err)
		return "", "", nil
	}
	if !strings.Contains(lsb[lsbrelease.ReleaseTrack], "test") {
		testing.ContextLog(ctx, "The image on usbkey is not a test image")
		return "", "", nil
	}

	return lsb[lsbrelease.BuilderPath], lsb[lsbrelease.Milestone], nil
}
