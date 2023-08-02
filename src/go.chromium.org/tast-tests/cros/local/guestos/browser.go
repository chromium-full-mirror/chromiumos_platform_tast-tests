// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package guestos

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/vm"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// LaunchBrowser runs a command that is expected to launch the host system browser.
func LaunchBrowser(ctx context.Context, tconn *chrome.TestConn, bt browser.Type, guest vm.Guest, title string, command []string) error {
	var err error
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	cmd := guest.Command(ctx, command...)
	testing.ContextLogf(ctx, "Running: %q", strings.Join(cmd.Args, " "))
	if err = cmd.Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to launch browser from container")
	}

	pollOptions := &testing.PollOptions{Timeout: time.Minute, Interval: time.Second}
	if err = ash.WaitForCondition(ctx, tconn, ash.BrowserTitleMatch(bt, title), pollOptions); err != nil {
		return errors.Wrapf(err, "failed to wait for the window to be open, browser: %v", bt)
	}

	return nil
}
