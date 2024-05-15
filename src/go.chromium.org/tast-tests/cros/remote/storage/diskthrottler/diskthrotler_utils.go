// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package diskthrottler contains utils for disk_throttler global hook
package diskthrottler

import (
	"context"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func getStatHex(ctx context.Context, dut *dut.DUT, rootdev, format string) (string, error) {
	statCmd := "stat " + strings.TrimSpace(string(rootdev)) + " --format=\"" + format + "\""
	rootdevNumBytes, err := dut.Conn().CommandContext(ctx, "bash", "-c", statCmd).Output(testexec.DumpLogOnError)
	if err != nil {
		return "", errors.Wrap(err, "failed to stat root device")
	}
	rootdevNum := strings.TrimSpace(string(rootdevNumBytes))
	val, err := strconv.ParseInt(rootdevNum, 16, 64)
	if err != nil {
		return "", errors.Wrapf(err, "failed to parse int64 for argument %q", rootdevNum)
	}
	return strconv.FormatInt(val, 10), nil
}

func setEphemeralLimit(ctx context.Context, dut *dut.DUT, limit, rootdev string, val int64) error {
	argCmd := "echo \"" + rootdev + " " + strconv.FormatInt(val, 10) + "\" > /sys/fs/cgroup/blkio/blkio.throttle." + limit
	testing.ContextLog(ctx, argCmd)
	_, err := dut.Conn().CommandContext(ctx, "bash", "-c", argCmd).Output(testexec.DumpLogOnError)
	return err
}

func activeKernelPartition(ctx context.Context, dut *dut.DUT) (int64, error) {
	activeRootfsBytes, err := dut.Conn().CommandContext(ctx, "rootdev", "-s").Output(testexec.DumpLogOnError)
	if err != nil {
		return -1, errors.Wrap(err, "failed to get active rootfs")
	}

	activeRootfs := string(activeRootfsBytes)

	lastChar := activeRootfs[len(activeRootfs)-2 : len(activeRootfs)-1]
	val, err := strconv.ParseInt(lastChar, 10, 64)
	if err != nil {
		return -1, errors.Wrapf(err, "failed to get partition number from: %s", activeRootfs)
	}

	return val - 1, nil
}

func kernelArguments(ctx context.Context, dut *dut.DUT, partNum int64) (string, error) {
	testing.ContextLog(ctx, "Retrieving kernel arguments")
	partStr := strconv.FormatInt(partNum, 10)
	prefix := "/tmp/kargs" + uuid.New().String()
	err := dut.Conn().CommandContext(ctx, "/usr/share/vboot/bin/make_dev_ssd.sh", "--save_config", prefix, "--partitions", partStr).Run()
	if err != nil {
		return "", errors.Wrap(err, "failed to save boot config")
	}

	savedKArgsFile := prefix + "." + partStr
	defer dut.Conn().CommandContext(ctx, "rm", savedKArgsFile)

	argBytes, err := dut.Conn().CommandContext(ctx, "cat", savedKArgsFile).Output(testexec.DumpLogOnError)
	if err != nil {
		return "", errors.Wrap(err, "failed to read saved kernel config")
	}

	args := string(argBytes)

	if strings.Index(args, "init") == -1 {
		return "", errors.Wrapf(err, "failed validity check for kernel argumets: %q", args)
	}

	return args, nil
}

func setKernelArguments(ctx context.Context, dut *dut.DUT, partNum int64, args string) error {
	testing.ContextLog(ctx, "Setting kernel arguments")
	partStr := strconv.FormatInt(partNum, 10)
	prefix := "/tmp/kargs" + uuid.New().String()
	savedKArgsFile := prefix + "." + partStr
	defer dut.Conn().CommandContext(ctx, "rm", savedKArgsFile)

	echoCmd := "echo -n '" + args + "' > " + savedKArgsFile
	if err := dut.Conn().CommandContext(ctx, "bash", "-c", echoCmd).Run(); err != nil {
		return errors.Wrap(err, "failed to write kernel arguments tmp file")
	}

	if out, err := dut.Conn().CommandContext(ctx, "/usr/share/vboot/bin/make_dev_ssd.sh", "--set_config", prefix, "--partitions", partStr).Output(testexec.DumpLogOnError); err != nil {
		return errors.Wrapf(err, "failed to set kernel arguments file: %q", string(out))
	}

	return nil
}

func removeLimitsFromKernelArguments(args string) string {
	limitKArgs := []string{"storage_limit_device", "storage_limit_read_iops", "storage_limit_write_iops", "storage_limit_read_bps", "storage_limit_write_bps"}
	for _, arg := range limitKArgs {
		re := regexp.MustCompile(`\s` + arg + `=[^\s]+`)
		args = re.ReplaceAllString(args, "")
	}
	return args
}

func addLimitsToKernelArguments(args, dev string, readIopsLimit, writeIopsLimit, readBpsLimit, writeBpsLimit int64) string {
	devArg := " storage_limit_device=" + dev
	riopsArg := " storage_limit_read_iops=" + strconv.FormatInt(readIopsLimit, 10)
	wiopsArg := " storage_limit_write_iops=" + strconv.FormatInt(writeIopsLimit, 10)
	rbpsArg := " storage_limit_read_bps=" + strconv.FormatInt(readBpsLimit, 10)
	wbpsArg := " storage_limit_write_bps=" + strconv.FormatInt(writeBpsLimit, 10)

	return args + devArg + riopsArg + wiopsArg + rbpsArg + wbpsArg
}

// ParseLimit converts string limit into a numeric value and verifies it is within expected range.
func ParseLimit(limit string) (int64, error) {
	val, err := strconv.ParseInt(limit, 10, 64)
	if err != nil {
		return 0, errors.Wrapf(err, "failed to parse int64 for limit %q", limit)
	}
	if val < 0 {
		return 0, errors.Errorf("limits must be non-negative, found %d", val)
	}

	return val, nil
}

// RootdevPath returns path to the root device.
func RootdevPath(ctx context.Context, dut *dut.DUT) (string, error) {
	rootdevBytes, err := dut.Conn().CommandContext(ctx, "rootdev", "-s", "-d").Output(testexec.DumpLogOnError)
	if err != nil {
		return "", errors.Wrap(err, "failed to get root device")
	}
	return strings.TrimSpace(string(rootdevBytes)), nil
}

// DevMajorMinor returns devices "major:minor" string (base-10 encoding)
func DevMajorMinor(ctx context.Context, dut *dut.DUT, dev string) (string, error) {
	major, err := getStatHex(ctx, dut, dev, "%t")
	if err != nil {
		return "", errors.Wrapf(err, "failed to stat device %q", dev)
	}
	minor, err := getStatHex(ctx, dut, dev, "%T")
	if err != nil {
		return "", errors.Wrapf(err, "failed to stat device %q", dev)
	}

	return major + ":" + minor, nil
}

// SetEphemeralLimits sets non-boot persistent throttling limits for the supplied device.
func SetEphemeralLimits(ctx context.Context, dut *dut.DUT, devMajorMinor string, readIopsLimit, writeIopsLimit, readBpsLimit, writeBpsLimit int64) error {
	var firstError error

	if err := setEphemeralLimit(ctx, dut, "read_iops_device", devMajorMinor, readIopsLimit); err != nil && firstError == nil {
		firstError = err
	}
	if err := setEphemeralLimit(ctx, dut, "write_iops_device", devMajorMinor, writeIopsLimit); err != nil && firstError == nil {
		firstError = err
	}
	if err := setEphemeralLimit(ctx, dut, "read_bps_device", devMajorMinor, readBpsLimit); err != nil && firstError == nil {
		firstError = err
	}
	if err := setEphemeralLimit(ctx, dut, "write_bps_device", devMajorMinor, writeBpsLimit); err != nil && firstError == nil {
		firstError = err
	}

	return firstError
}

// ResetEphemeralLimits removes non-boot persistent throttling limits for the supplied device.
func ResetEphemeralLimits(ctx context.Context, dut *dut.DUT, rootdevMajorMinor string) error {
	return SetEphemeralLimits(ctx, dut, rootdevMajorMinor, 0, 0, 0, 0)
}

// SetPersistentLimits modifies kernel argument to throttle storage device at boot.
func SetPersistentLimits(ctx context.Context, dut *dut.DUT, dev string, readIopsLimit, writeIopsLimit, readBpsLimit, writeBpsLimit int64) error {
	partNum, err := activeKernelPartition(ctx, dut)
	if err != nil {
		return errors.Wrap(err, "failed to get active kernel partition")
	}

	args, err := kernelArguments(ctx, dut, partNum)
	if err != nil {
		return errors.Wrap(err, "failed to get kernel arguments")
	}

	args = removeLimitsFromKernelArguments(args)
	args = addLimitsToKernelArguments(args, dev, readIopsLimit, writeIopsLimit, readBpsLimit, writeBpsLimit)
	if err := setKernelArguments(ctx, dut, partNum, args); err != nil {
		return errors.Wrap(err, "failed to set kernel argument with limits removed")
	}

	if err := dut.Reboot(ctx); err != nil {
		return errors.Wrap(err, "failed to reboot the DUT after setting kernel arguments")
	}

	return nil
}

// ResetPersistentLimits removes storage throttling kernel arguments.
func ResetPersistentLimits(ctx context.Context, dut *dut.DUT, dev string) error {
	partNum, err := activeKernelPartition(ctx, dut)
	if err != nil {
		return errors.Wrap(err, "failed to get active kernel partition")
	}

	args, err := kernelArguments(ctx, dut, partNum)
	if err != nil {
		return errors.Wrap(err, "failed to get kernel arguments")
	}

	args = removeLimitsFromKernelArguments(args)
	if err := setKernelArguments(ctx, dut, partNum, args); err != nil {
		return errors.Wrap(err, "failed to set kernel argument with limits removed")
	}

	if err := dut.Reboot(ctx); err != nil {
		return errors.Wrap(err, "failed to reboot the DUT after setting kernel arguments")
	}

	return nil
}
