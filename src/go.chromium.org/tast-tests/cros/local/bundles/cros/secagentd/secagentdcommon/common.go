// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package secagentdcommon contains shared general helpers used in secagentd tast tests.
package secagentdcommon

import (
	"bufio"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	xdr "go.chromium.org/chromiumos/xdr/secagentd"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"
)

const (
	// kernelTraceFile where bpf kernel printfs are logged to.
	kernelTraceFile = "/sys/kernel/debug/tracing/trace"
	// secagentdLogFile where secagentd service sends logs to.
	secagentdLogFile = "/var/log/secagentd.log"
)

// CheckCommon verifies that the common message fields are filled with appropriate values.
func CheckCommon(common *xdr.CommonEventVariantDataFields) error {
	if common.GetCreateTimestampUs() == 0 {
		return errors.New("CreateTimestampUs field not set")
	}
	deviceUser := common.GetDeviceUser()
	if common.DeviceUser == nil || deviceUser == "Unknown" {
		return errors.Errorf("invalid username: %s", deviceUser)
	}
	return nil
}

// ClearKernelTrace clears out the kernel tracing buffer.
func ClearKernelTrace(ctx context.Context) error {
	if err := os.Truncate(kernelTraceFile, 0); err != nil {
		return errors.Wrapf(err, "failed to clear %q", kernelTraceFile)
	}
	return nil
}

// OnErrorSaveKernelTrace saves off a copy of the kernel trace on test failure.
// USAGE: defer OnErrorSaveKernelTrace(ctx, s)
func OnErrorSaveKernelTrace(ctx context.Context, outDir string, hasError func() bool) error {
	if !hasError() {
		return nil
	}
	outFile := filepath.Join(outDir, "kernel_trace")
	if err := fsutil.CopyFile(kernelTraceFile, outFile); err != nil {
		return errors.Wrapf(err, "failed to copy %q to %q:", kernelTraceFile, outFile)

	}
	return nil
}

// OnErrorSaveSecagentdLog saves off a copy of the secagentd log on test failure.
// USAGE: defer OnErrorSaveSecagentdLog(ctx, s.OutDir(), s.HasError)
func OnErrorSaveSecagentdLog(ctx context.Context, outDir string, hasError func() bool) error {
	if !hasError() {
		return nil
	}
	if _, err := os.Stat(secagentdLogFile); os.IsNotExist(err) {
		return nil
	}
	outFile := filepath.Join(outDir, "secagentd.log")
	if err := fsutil.CopyFile(secagentdLogFile, outFile); err != nil {
		return errors.Wrapf(err, "failed to copy %q to %q", secagentdLogFile, outFile)
	}
	return nil
}

// ClearSecagentdLog clears out the secagentd.log file.
func ClearSecagentdLog() error {
	err := os.Truncate(secagentdLogFile, 0)
	if err != nil {
		return errors.Wrap(err, "unable to clear "+secagentdLogFile)
	}
	return nil
}

// GetSecagentdLogSize returns the size of the logfile.
func GetSecagentdLogSize() (int64, error) {
	statInfo, err := os.Stat(secagentdLogFile)
	if err != nil {
		return -1, err
	}
	return statInfo.Size(), nil
}

// WaitForStringInLog waits for a specific string to appear in the secagentd.log
// The file should be cleared prior to restarting the daemon via ClearSecagentdLog
// prior to waiting for a string. Failure to do so means that strings from
// past runs may abort the wait prematurely.
func WaitForStringInLog(ctx context.Context, text string, startingOffset int64,
	logf func(format string, args ...interface{})) error {
	return WaitForStringInLogWithOptions(ctx, text, startingOffset,
		&testing.PollOptions{Timeout: 15 * time.Second, Interval: 2 * time.Second}, logf)
}

// WaitForStringInLogWithOptions is the same as WaitForStringInLog, but polls
// secagentd.log with the given poll options (e.g. timeout and interval).
func WaitForStringInLogWithOptions(ctx context.Context, text string, startingOffset int64,
	opts *testing.PollOptions, logf func(format string, args ...interface{})) error {
	offset := startingOffset
	return testing.Poll(ctx, func(ctx context.Context) error {
		logReader, err := os.Open(secagentdLogFile)
		if err != nil {
			return errors.Wrap(err, "failed to open "+secagentdLogFile)
		}
		if _, err := logReader.Seek(offset, io.SeekStart); err != nil {
			return errors.Wrapf(err, "Seek to %d failed on %q", offset, secagentdLogFile)
		}
		logScanner := bufio.NewScanner(logReader)
		defer logReader.Close()
		for logScanner.Scan() {
			if strings.Contains(logScanner.Text(), text) {
				foundAt, _ := logReader.Seek(0, io.SeekCurrent)
				logf("Found %q in line %q at offset %v", text, logScanner.Text(), foundAt)
				return nil
			}
		}
		offset, err = logReader.Seek(0, io.SeekCurrent)
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to calculate offset after scanning"))
		}
		return errors.New("could not find " + text + " in " + logReader.Name())
	}, opts)
}
