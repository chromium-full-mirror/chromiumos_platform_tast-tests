// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/network/shillscript"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ShillInitLogoutScript,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test that shill init logout script perform as expected",
		Contacts:     []string{"cros-networking@google.com", "hugobenichi@google.com"},
		BugComponent: "b:156085",
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:network", "network_platform"},
	})
}

func ShillInitLogoutScript(ctx context.Context, s *testing.State) {
	if err := shillscript.RunTest(ctx, testLogout, false); err != nil {
		s.Fatal("Failed running testLogout: ", err)
	}
}

// testLogout tests the logout process.
func testLogout(ctx context.Context, env *shillscript.TestEnv) error {
	if err := upstart.StartJob(ctx, shill.JobName); err != nil {
		return errors.Wrap(err, "failed starting shill")
	}

	cr, err := chrome.New(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to start Chrome")
	}
	defer cr.Close(ctx)

	timeoutCtx, cancel := context.WithTimeout(ctx, shillscript.DbusMonitorTimeout)
	defer cancel()

	stop, err := shillscript.DbusEventMonitor(timeoutCtx)
	if err != nil {
		return err
	}

	// TODO (b:159063029) Add a logout function in chrome.go and use it here
	// instead of Restatring the ui.
	// Emulate logout.
	if err := upstart.RestartJob(ctx, "ui"); err != nil {
		stop()
		return errors.Wrap(err, "Chrome failed to log out")
	}

	calledMethods, err := stop()
	if err != nil {
		return err
	}

	expectedCalls := []string{shillscript.PopAllUserProfiles}
	if err := shillscript.AssureMethodCalls(ctx, expectedCalls, calledMethods); err != nil {
		return err
	}

	if err := shillscript.AssureNotExists(shillscript.ShillUserProfilesDir); err != nil {
		return err
	}

	profiles, err := shillscript.GetProfileList(ctx)
	if err != nil {
		return err
	}

	if len(profiles) > 1 {
		return errors.Wrapf(err, "found unexpected number of profiles in the profile stack: got %d, want 1", len(profiles))
	}

	return nil
}
