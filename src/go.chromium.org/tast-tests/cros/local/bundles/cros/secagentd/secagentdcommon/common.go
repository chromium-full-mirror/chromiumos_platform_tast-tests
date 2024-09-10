// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package secagentdcommon contains shared general helpers used in secagentd tast tests.
package secagentdcommon

import (
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
func WaitForStringInLog(ctx context.Context, text string, startingOffset int64) error {
	offset := startingOffset
	return testing.Poll(ctx, func(ctx context.Context) error {
		loginfo, err := os.Stat(secagentdLogFile)
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to stat "+secagentdLogFile))
		}
		currentSize := loginfo.Size()
		buff := make([]byte, currentSize-offset)
		logFile, err := os.Open(secagentdLogFile)
		defer logFile.Close()
		bytesRead, err := logFile.ReadAt(buff, offset)
		if err != nil && err != io.EOF {
			return testing.PollBreak(errors.Wrap(err, "failed to read "+secagentdLogFile))
		}
		offset += int64(bytesRead)
		if !strings.Contains(string(buff), text) {
			return errors.New("could not find " + text + " in " + logFile.Name())
		}
		return nil
	}, &testing.PollOptions{Timeout: 15 * time.Second, Interval: 2 * time.Second})
}
