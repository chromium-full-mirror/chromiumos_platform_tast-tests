// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// This file provides small helpers, packaging shill APIs in ways to ease their
// use by others.

package shill

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func waitForOnline(ctx context.Context, timeout time.Duration) error {
	m, err := NewManager(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to connect to shill's Manager")
	}

	expectProps := map[string]interface{}{
		shillconst.ServicePropertyState: shillconst.ServiceStateOnline,
	}
	if _, err := m.WaitForServiceProperties(ctx, expectProps, timeout); err != nil {
		return errors.Wrap(err, "network did not come back online")
	}

	return nil
}

// WaitForOnline waits for Internet connectivity, a shorthand which is useful so external packages don't have to worry
// about Shill details (e.g., Service, Manager). Tests that require Internet connectivity (e.g., for a real GAIA login)
// need to ensure that before trying to perform Internet requests. This function is one way to do that.
// Returns an error if we don't come back online within a reasonable amount of time.
func WaitForOnline(ctx context.Context) error {
	return waitForOnline(ctx, 15*time.Second)
}

// WaitForOnlineAfterResume uses a longer timeout than WaitForOnline. This is due to how long network issues in the lab
// can take to resolve. If the ethernet dongle does not resume properly, it can take up to 3 minutes for the test hooks
// to reset the device and connect to the network.
func WaitForOnlineAfterResume(ctx context.Context) error {
	return waitForOnline(ctx, 3*time.Minute)
}

// LogOutUserAndPushTestProfile will log out user if the user is logged in, and
// push a test profile to shill and return a clean up function to pop up this
// profile. This function is helpful if the test may pollute the profile and not
// easy to do the cleanup at the end of the test. Caveats:
//   - This function won't help if the property is written to default profile
//     (platform2/shill/default_profile.h).
//   - This function may restart Chrome.
func LogOutUserAndPushTestProfile(ctx context.Context) (func(context.Context), error) {
	m, err := NewManager(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to shill manager")
	}

	// If there are still multiple profiles, it very likely means that there is a
	// user logged in. In this case, the following PushProfile() will fail since a
	// non-default global profile cannot be pushed on top of a user profile. Do a
	// UI restart here to make sure that there is no user profile.
	paths, err := m.ProfilePaths(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get current profiles")
	}
	if len(paths) > 1 {
		testing.ContextLog(ctx, "Restarting UI since PushTestProfile requires that Chrome is not logged in")
		if err := upstart.RestartJob(ctx, "ui"); err != nil {
			return nil, errors.Wrap(err, "failed to restart ui")
		}
	}

	return m.PushTestProfile(ctx)
}
