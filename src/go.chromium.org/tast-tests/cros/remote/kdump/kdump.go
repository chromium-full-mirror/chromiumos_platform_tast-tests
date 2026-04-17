// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package kdump provides common utilities for kdump tests.
package kdump

import (
	"context"
	"regexp"
	"strconv"
	"strings"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/exec"
	"go.chromium.org/tast/core/testing"
)

// KexecCrashLoadedPath is the path to the file that indicates whether a crash
// kernel is loaded.
const KexecCrashLoadedPath = "/sys/kernel/kexec_crash_loaded"

const (
	// KdumpDir is the directory that contains kdump artifacts.
	KdumpDir                           = "/var/spool/kdump"
	procCmdlinePath                    = "/proc/cmdline"
	cmdlineParamCrashKernel            = "crashkernel"
	cmdlineParamCrashKexecPostNotifier = "crash_kexec_post_notifiers"
)

// ListKdumpFiles lists the files in the kdump directory.
func ListKdumpFiles(ctx context.Context, d *dut.DUT) ([]string, error) {
	out, err := d.Conn().CommandContext(ctx, "ls", "-1", KdumpDir).CombinedOutput()
	if err != nil {
		// It's okay if the directory doesn't exist.
		if strings.Contains(string(out), "No such file or directory") {
			return nil, nil
		}
		return nil, errors.Wrap(err, "failed to list kdump files")
	}
	return strings.Split(strings.TrimSpace(string(out)), "\n"), nil
}

// getRootPartition returns root partition index by running `rootdev -s` and
// converting the last digit to partition index.
// For example, "/dev/mmcblk0p3" corresponds to root partition index 2 that is
// used in modifying the kernel args.
func getRootPartition(ctx context.Context, d *dut.DUT) (string, error) {
	out, err := d.Conn().CommandContext(ctx, "/usr/bin/rootdev", "-s").Output()
	if err != nil {
		return "", errors.Wrap(err, "failed in getting the root partition")
	}

	// Sample output of "rootdev -s": /dev/nvme0p3, /dev/mmcblk0p3, /dev/sda3
	// Capture the ending numerical part, followed by a newline, of the output.
	// Reference: init of $ROOTDEV_PARTITION in
	// src/platform/vboot_reference/scripts/image_signing/make_dev_ssd.sh.
	re := regexp.MustCompile(`.*\D(\d+)\s*$`)
	groups := re.FindStringSubmatch(string(out))
	if len(groups) != 2 {
		return "", errors.Errorf("failed to parse root partition from %s", out)
	}

	i, err := strconv.Atoi(groups[1])
	if err != nil {
		return "", errors.Errorf("failed to get partition index from %s", groups[1])
	}

	return strconv.Itoa(i - 1), nil
}

// checkKdumpEnabledInCmdline checks if kdump is enabled in the given kernel
// command line.
func checkKdumpEnabledInCmdline(cmdline string) error {
	if !strings.Contains(cmdline, cmdlineParamCrashKernel) {
		return errors.Errorf("current params: `%s`, want `%s` included", cmdline, cmdlineParamCrashKernel)
	}
	if !strings.Contains(cmdline, cmdlineParamCrashKexecPostNotifier) {
		return errors.Errorf("current params: `%s`, want `%s` included", cmdline, cmdlineParamCrashKexecPostNotifier)

	}
	return nil
}

// getKernelCmdline returns the kernel command line.
func getKernelCmdline(ctx context.Context, d *dut.DUT) (string, error) {
	out, err := d.Conn().CommandContext(ctx, "cat", procCmdlinePath).Output(exec.DumpLogOnError)
	return string(out), err
}

// isKdumpEnabled checks if kdump is enabled on the DUT.
func isKdumpEnabled(ctx context.Context, d *dut.DUT) (bool, error) {
	cmdline, err := getKernelCmdline(ctx, d)
	if err != nil {
		return false, errors.Wrap(err, "failed to get kernel command line")
	}
	checkErr := checkKdumpEnabledInCmdline(cmdline)
	return checkErr == nil, err
}

// checkKdumpState checks if the kdump state on the DUT is the same as want.
func checkKdumpState(ctx context.Context, d *dut.DUT, want bool) error {
	cmdline, err := getKernelCmdline(ctx, d)
	if err != nil {
		return errors.Wrap(err, "failed to get kernel command line")
	}
	checkErr := checkKdumpEnabledInCmdline(cmdline)
	if want && checkErr != nil {
		return errors.Wrap(checkErr, "kdump is not enabled, want enabled")
	}
	if !want && checkErr == nil {
		return errors.New("kdump is enabled, want disabled")
	}
	return nil
}

// setKdumpState sets the kdump state on the DUT.
func setKdumpState(ctx context.Context, d *dut.DUT, state bool) error {
	// Get the kernel partition.
	part, err := getRootPartition(ctx, d)
	if err != nil {
		return errors.Wrap(err, "failed to get the root partition index")
	}

	// Modify the kernel boot parameters.
	args := []string{"--partitions", part}
	if state {
		args = append(args, "--enable_kdump")
	} else {
		args = append(args, "--disable_kdump")
	}
	if err := d.Conn().CommandContext(ctx, "/usr/share/vboot/bin/make_dev_ssd.sh", args...).Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to modify kernel params")
	}

	// Reboot the device.
	if err := d.Reboot(ctx); err != nil {
		return errors.Wrap(err, "failed to reboot DUT")
	}

	// Check the new kernel param.
	if err := checkKdumpState(ctx, d, state); err != nil {
		return errors.Wrap(err, "failed to verify kdump state after reboot")
	}

	return nil
}

// EnableKdump enables kdump on the DUT and returns a function to disable it. If
// kdump is already enabled, this function will do nothing. Note that this
// function may reboot the DUT in the middle.
func EnableKdump(ctx context.Context, d *dut.DUT) (func(ctx context.Context) error, error) {
	// Check if Kdump is already enabled.
	if isEnabled, err := isKdumpEnabled(ctx, d); err != nil {
		return nil, errors.Wrap(err, "failed to check kdump state before enablement")
	} else if isEnabled {
		testing.ContextLog(ctx, "Kdump is already enabled")
		// No cleanup is needed.
		return func(ctx context.Context) error { return nil }, nil
	}

	testing.ContextLog(ctx, "Enable kdump and reboot")
	if err := setKdumpState(ctx, d, true /*state*/); err != nil {
		return nil, errors.Wrap(err, "failed to enable kdump")
	}

	return func(ctx context.Context) error {
		testing.ContextLog(ctx, "Disable kdump and reboot")
		return setKdumpState(ctx, d, false /*state*/)
	}, nil
}
