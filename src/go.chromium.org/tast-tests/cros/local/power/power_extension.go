// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"fmt"
	"io/ioutil"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// Level describes the power level management to use.
type Level int

const (
	// Display keeps the screen and system active.
	Display Level = iota
	// System keeps the system active but allows the screen to be dimmed or turned off.
	System
)

// RequestKeepAwake sends a request to keep the system awake. A function is
// returned to restore the previous state. Full reference can be found here:
// https://developer.chrome.com/docs/extensions/reference/power/.
func RequestKeepAwake(ctx context.Context, tconn *chrome.TestConn, level Level) (func(ctx context.Context, tconn *chrome.TestConn) error, error) {
	// Convert the level to a string for the API call.
	var levelVal string
	switch level {
	case Display:
		levelVal = "display"
	case System:
		levelVal = "system"
	default:
		return nil, errors.Errorf("invalid level provided: %v", level)
	}

	// Make a call to the requestKeepAwake API. Wrapping in a promisify
	// incorrectly sends the arguments which is why a custom handler is made.
	if err := tconn.Call(ctx, nil, `(level) => new Promise((resolve, reject) => {
		chrome.power.requestKeepAwake(level);
		if (chrome.runtime.lastError) {
			reject(new Error(chrome.runtime.lastError.message));
			return;
		}
		resolve();
	})`, levelVal); err != nil {
		return nil, errors.Wrap(err, "failed to call requestKeepAwake")
	}

	return func(ctx context.Context, tconn *chrome.TestConn) error {
		return releaseKeepAwake(ctx, tconn)
	}, nil
}

// releaseKeepAwake restores the previous power state. Full reference can be
// found here: https://developer.chrome.com/docs/extensions/reference/power/.
func releaseKeepAwake(ctx context.Context, tconn *chrome.TestConn) error {
	if err := tconn.Call(ctx, nil, `() => new Promise((resolve, reject) => {
			chrome.power.releaseKeepAwake();
			if (chrome.runtime.lastError) {
				reject(new Error(chrome.runtime.lastError.message));
				return;
			}
			resolve();
		})`); err != nil {
		return errors.Wrap(err, "failed to call releaseKeepAwake")
	}

	return nil
}

// SuspendAndResume calls powerd_dbus_suspend command to suspend the system
// and lets it stay sleep for the given duration and then wake up.
func SuspendAndResume(ctx context.Context, cr *chrome.Chrome, timeout time.Duration) error {
	// Read wakeup count here to prevent suspend retries, which happens without
	// user input.
	wakeupCount, err := ioutil.ReadFile("/sys/power/wakeup_count")
	if err != nil {
		return errors.Wrap(err, "failed to read wakeup count before suspend")
	}

	timeoutSec := int(timeout.Round(time.Second).Seconds())
	cmd := testexec.CommandContext(
		ctx,
		"powerd_dbus_suspend",
		"--delay=0",
		fmt.Sprintf("--wakeup_timeout=%d", timeoutSec),
		fmt.Sprintf("--wakeup_count=%s", strings.Trim(string(wakeupCount), "\n")),
		"--timeout=30",
	)
	testing.ContextLogf(ctx, "Suspend DUT for %d seconds: %s", timeoutSec, cmd.Args)

	if err := cmd.Run(); err != nil {
		return errors.Wrap(err, "powerd_dbus_suspend failed to properly suspend")
	}

	testing.ContextLog(ctx, "DUT resumes from suspend")
	return cr.Reconnect(ctx)
}
