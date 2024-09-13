// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package faillog provides helper functions for dumping UI data on test failures.
package faillog

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/testing"
)

func deviceEventLogName(logType string) string {
	if logType != "" {
		return fmt.Sprintf("device_event_log_%s.txt", logType)
	}
	return "device_event_log_all.txt"
}

// DumpDeviceEventLogOnError checks hasError to see if the test failed and saves the logType subset
// of chrome://device-log into a device_event_log file in outDir.  It does nothing if the test succeeds.
func DumpDeviceEventLogOnError(ctx context.Context, outDir string, hasError func() bool, tconn *chrome.TestConn, logType string) {
	if !hasError() {
		return
	}

	fileName := deviceEventLogName(logType)
	testing.ContextLog(ctx, "Test failed. Dumping the device event log into ", fileName)
	DumpDeviceEventLogToFile(ctx, outDir, tconn, logType, fileName)
}

// DumpDeviceEventLogToFile saves the logType subset of chrome://device-log into fileName in outDir.
func DumpDeviceEventLogToFile(ctx context.Context, outDir string, tconn *chrome.TestConn, logType, fileName string) {
	dir := filepath.Join(outDir, faillogDir)
	if err := os.MkdirAll(dir, 0777); err != nil {
		testing.ContextLogf(ctx, "Failed to create directory %s: %v", dir, err)
		return
	}

	var logs string
	if err := tconn.Call(ctx, &logs, "tast.promisify(chrome.autotestPrivate.getDeviceEventLog)", logType); err != nil {
		testing.ContextLog(ctx, "Failed to get device event log from Chrome: ", err)
		return
	}

	filePath := filepath.Join(dir, fileName)
	if err := os.WriteFile(filePath, []byte(logs), 0644); err != nil {
		testing.ContextLogf(ctx, "Failed to save device event to %s: %v", filePath, err)
	}
}
