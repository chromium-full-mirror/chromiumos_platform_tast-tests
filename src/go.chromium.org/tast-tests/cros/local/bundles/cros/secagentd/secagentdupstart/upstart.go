// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package secagentdupstart contains shared helpers for interacting with the
// secagentd upstart service.
package secagentdupstart

import (
	"context"
	"time"

	upstartcommon "go.chromium.org/tast-tests/cros/common/upstart"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/secagentd/secagentddbusmonitor"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/secagentd/secagentdprocfsscraper"
	"go.chromium.org/tast-tests/cros/local/dbusutil"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/testing"
)

// RestartSecagentd restarts the daemon with the given upstart args, verifies
// that it is running as expected and returns the daemon pid.
func RestartSecagentd(ctx context.Context, waitForAddMatchSignal bool, args ...upstart.Arg) (uint64, error) {
	const name = "secagentd"

	var addMatchWatcher *dbusutil.EventWatcher = nil
	if waitForAddMatchSignal {
		// Monitor for the AddMatch signal for UserDataAuth which signifies secagentd listening for AuthenticateAuthFactorCompleted signals from cryptohome.
		watcher, addMatchCancel, err := secagentddbusmonitor.SetupAddMatchDbusWatcherWithTimeout(ctx, 15*time.Second)
		addMatchWatcher = watcher
		if err != nil {
			addMatchCancel()
			return 0, err
		}
		defer addMatchCancel()
	}

	if err := upstart.RestartJob(ctx, name, args...); err != nil {
		return 0, err
	}
	if err := upstart.WaitForJobStatus(ctx, name, upstartcommon.StartGoal, upstartcommon.RunningState, upstart.RejectWrongGoal, 5*time.Second); err != nil {
		return 0, err
	}
	_, _, mjPid, err := upstart.JobStatus(ctx, name)
	if err != nil {
		return 0, err
	}
	// Upstart returns the minijail0 pid. secagentd is its child. Poll briefly
	// to let minijail do its thing and start secagentd.
	pid := uint64(0)
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		pid, err = secagentdprocfsscraper.GetOnlyChildPid(uint64(mjPid))
		return err
	}, &testing.PollOptions{Timeout: 5 * time.Second}); err != nil {
		return 0, err
	}

	if waitForAddMatchSignal {
		if err := secagentddbusmonitor.WaitForAddMatchSignal(addMatchWatcher); err != nil {
			return 0, err
		}
		// TODO(b/329819143): Temp solution until we had a secagentd ready signal emitted from client.
		// GoBigSleepLint: Once the correct dbus signal is sent wait some time to ensure
		// secagentd is ready to receive cryptohome signals.
		testing.Sleep(ctx, 5*time.Second)
	}

	return pid, nil
}
