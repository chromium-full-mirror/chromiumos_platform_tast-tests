// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package util

import (
	"context"
	"strconv"
	"strings"

	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	// TrimChunkSize is the size (in bytes) of a data chunk to trim.
	TrimChunkSize = 192 * 1024

	// TrimFileSize is the size (in bytes) of the file we want to trim.
	TrimFileSize = 4 * 1024 * 1024 * 1024 // 4 GiB
)

// TrimComparisonFunc is the code used to compare trimmed data and return whether it matches the expected value.
type TrimComparisonFunc func(context.Context, *dut.DUT, string, uint64) (bool, error)

// RunTrim calls the ioctl trim command.
func RunTrim(ctx context.Context, dut *dut.DUT, diskPath string, offset, size uint64) error {
	_, err := RunCmdWithOutputSilent(ctx, dut, "blkdiscard", "-f", "-o", strconv.FormatUint(offset*TrimChunkSize, 10), "-l", strconv.FormatUint(size, 10), diskPath)
	if err != nil {
		return errors.Wrap(err, "failed to execute trim")
	}
	return nil
}

// RunTrimForRatio runs trim for the specified ratio of the dut.
func RunTrimForRatio(ctx context.Context, dut *dut.DUT, diskpath string, chunkCount uint64, ratio float64) ([]bool, int, error) {
	verifyCount := 0
	trimmed := make([]bool, chunkCount)
	for i := uint64(0); i < chunkCount; i++ {
		if float64(i%4)/4 < ratio {
			if err := RunTrim(ctx, dut, diskpath, i, TrimChunkSize); err != nil {
				return nil, 0, errors.Wrap(err, "error running trim command")
			}
			trimmed[i] = true
			verifyCount++
		}
	}
	return trimmed, verifyCount, nil
}

// WriteRandomData writes random data to a specified file.
func WriteRandomData(ctx context.Context, dut *dut.DUT, filepath string, chunkCount uint64) error {
	cmd := "dd if=/dev/urandom of=" + filepath + " bs=" + strconv.FormatUint(TrimChunkSize, 10) + " count=" + strconv.FormatUint(chunkCount, 10)
	out, err := RunCmdWithStringOutput(ctx, dut, "bash", "-c", cmd)
	if err != nil {
		return errors.Wrap(err, "failed to write random data")
	}
	testing.ContextLogf(ctx, "Writing random data, output: %s", out)
	return nil
}

// GetCurrentHashes calculates hash values for all chunks of the specified file.
func GetCurrentHashes(ctx context.Context, dut *dut.DUT, filename string, chunkCount uint64) (hashes []string, err error) {
	for i := uint64(0); i < chunkCount; i++ {
		buf, err := GetCurrentHash(ctx, dut, filename, i)
		if err != nil {
			return hashes, errors.Wrapf(err, " error reading from: %s", filename)
		}
		hashes = append(hashes, buf)
	}
	return hashes, nil
}

// GetCurrentHash calculates hash values for the chunk of the specified file.
func GetCurrentHash(ctx context.Context, dut *dut.DUT, filename string, chunk uint64) (hash string, err error) {
	cmd := "dd if=" + filename + " bs=" + strconv.FormatUint(TrimChunkSize, 10) + " count=1 skip=" + strconv.FormatUint(chunk, 10) + " status=none | sha256sum"
	buf, err := RunCmdWithStringOutputSilent(ctx, dut, "bash", "-c", cmd)
	if err != nil {
		return "", errors.Wrapf(err, " error reading from: %s", filename)
	}
	return buf, nil
}

// CompareToZero compares the chunk of the specified file to all zeroes.
func CompareToZero(ctx context.Context, dut *dut.DUT, filename string, chunk uint64) (bool, error) {
	cmd := "dd if=" + filename + " bs=" + strconv.FormatUint(TrimChunkSize, 10) + " count=1 skip=" + strconv.FormatUint(chunk, 10) + " status=none | cmp - /dev/zero 2>&1"
	out, err := RunCmdWithStringOutputSilent(ctx, dut, "bash", "-c", cmd)
	if err != nil {
		return false, errors.Wrapf(err, " error reading from: %s", filename)
	}

	if strings.Contains(out, "differ") {
		return false, nil
	}

	if !strings.Contains(out, "cmp: EOF on -") {
		return false, errors.Wrapf(err, "unexpected comparison termination: %s", out)
	}
	return true, nil
}

// CompareToOne compares the chunk of the specified file to 0xff.
func CompareToOne(ctx context.Context, dut *dut.DUT, filename string, chunk uint64) (bool, error) {
	cmd := "dd if=" + filename + " bs=" + strconv.FormatUint(TrimChunkSize, 10) + " count=1 skip=" + strconv.FormatUint(chunk, 10) + " status=none | cmp - <(tr '\000' '\377' < /dev/zero) 2>&1"
	out, err := RunCmdWithStringOutputSilent(ctx, dut, "bash", "-c", cmd)
	if err != nil {
		return false, errors.Wrapf(err, " error reading from: %s", filename)
	}

	if strings.Contains(out, "differ") {
		return false, nil
	}

	if !strings.Contains(out, "cmp: EOF on -") {
		return false, errors.Wrapf(err, "unexpected comparison termination: %s", out)
	}
	return true, nil
}

// ExpectedTrimValueNvme returns the value we expect to read after trim on an nvme device.
func ExpectedTrimValueNvme(ctx context.Context, dut *dut.DUT, diskPath string) (TrimComparisonFunc, error) {
	identity, err := RunCmdWithStringOutput(ctx, dut, "nvme", "id-ns", "-n", "1", diskPath)
	if err != nil {
		return nil, errors.Wrap(err, "could not read namespace identity")
	}

	dlfeat, ok := FindSingleHexInt64NvmeRegs(ctx, identity, "dlfeat")
	if !ok {
		return nil, errors.Wrap(err, "could not get dlfeat value")
	}
	if dlfeat == 1 {
		testing.ContextLog(ctx, "Expecting read zero after trim for NVMe")
		return CompareToZero, nil
	} else if dlfeat == 2 {
		testing.ContextLog(ctx, "Expecting read one after trim for NVMe")
		return CompareToOne, nil
	} else {
		testing.ContextLog(ctx, "Expected values for trimmed data not specified")
		return nil, nil
	}
}

// ExpectedTrimValueEmmc returns the value we expect to read after trim on emmc devices.
func ExpectedTrimValueEmmc(ctx context.Context, dut *dut.DUT, diskPath string) (TrimComparisonFunc, error) {
	cmd := "mmc extcsd read " + diskPath + " | grep 'Erased memory content' | sed -n 's/.*: //p' | sed 's/]//'"
	out, err := RunCmdWithStringOutput(ctx, dut, "bash", "-c", cmd)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get erased memory content value")
	}
	if out[0:2] == "0x" {
		out = out[2:]
	}
	value, err := strconv.ParseInt(out, 16, 64)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse %q as int64", out)
	}
	if value == 0 {
		return CompareToZero, nil
	} else if value == 1 {
		return CompareToOne, nil
	}
	testing.ContextLog(ctx, "Expected values for trimmed data not recognized")
	return nil, nil
}

// ExpectedTrimValueUfs returns the value we expect to read after trim on ufs devices.
func ExpectedTrimValueUfs(ctx context.Context, dut *dut.DUT, diskPath string) (TrimComparisonFunc, error) {
	testing.ContextLog(ctx, "Expecting to read zero after trim for UFS")
	return CompareToZero, nil
}

// ExpectedTrimValueEmmcBridge returns the value we expect to read after trim on emmc bridge devices.
func ExpectedTrimValueEmmcBridge(ctx context.Context, dut *dut.DUT, diskPath string) (TrimComparisonFunc, error) {
	testing.ContextLog(ctx, "Expected values for trimmed data not specified")
	return nil, nil
}
