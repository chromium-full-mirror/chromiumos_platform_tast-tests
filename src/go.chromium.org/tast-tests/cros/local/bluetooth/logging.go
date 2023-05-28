// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package bluetooth contains helpers to interact with the system's bluetooth
// adapters.
package bluetooth

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// LogVerbosity indicates whether or not to enable verbose logging for the different bluetooth modules.
type LogVerbosity struct {
	Bluez  bool
	Kernel bool
}

// SetDebugLogLevels sets the logging level for Bluetooth debug logs.
func SetDebugLogLevels(ctx context.Context, levels LogVerbosity) error {
	// First check if we are using Floss; if so, skip adjusting the log level.
	cmd := testexec.CommandContext(ctx, "dbus-send", "--system", "--print-reply",
		"--dest=org.chromium.bluetooth.Manager",
		"/org/chromium/bluetooth/Manager",
		"org.chromium.bluetooth.Manager.GetFlossEnabled",
	)
	var stderr bytes.Buffer
	var stdout bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = &stdout
	if err := cmd.Run(testexec.DumpLogOnError); err != nil {
		// On systems that don't support Floss, the manager daemon will not be running by default.
		// If the manager is not found, we know we're on a bluez system and can proceed.
		testing.ContextLog(ctx, stderr.String())
		if !strings.Contains(stderr.String(), "org.chromium.bluetooth.Manager was not provided by any .service file") {
			return errors.Wrap(err, "failed to query floss status")
		}
	}
	testing.ContextLog(ctx, stdout.String())
	if strings.Contains(stdout.String(), "boolean true") {
		// Floss is enabled, so don't set bluez debug log levels.
		return nil
	}
	btoi := map[bool]int{
		false: 0,
		true:  1,
	}
	if err := testexec.CommandContext(ctx, "dbus-send", "--system", "--print-reply",
		"--dest=org.bluez", "/org/chromium/Bluetooth", "org.chromium.Bluetooth.Debug.SetLevels",
		fmt.Sprintf("byte:%v", btoi[levels.Bluez]),
		fmt.Sprintf("byte:%v", btoi[levels.Kernel]),
	).Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to set bluetooth log levels")
	}
	return nil
}

// StartBTSnoopLogging starts capturing Bluetooth HCI "btsnoop" logs in a file at the specified path.
// Call Start on the returned command to start log collection, and call Kill when finished to end btmon.
func StartBTSnoopLogging(ctx context.Context, path string) *testexec.Cmd {
	return testexec.CommandContext(ctx, "/usr/bin/btmon", "-w", path)
}
