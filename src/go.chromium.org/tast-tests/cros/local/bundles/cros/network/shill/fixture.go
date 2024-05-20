// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package shill contains library code for interacting with shill that is
// specific to the network testing package.
package shill

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/network"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: "shillReset",
		Desc: "A fixture that ensures shill is in a default state with no user profiles when the test starts and will reset any shill modifications after the test",
		Contacts: []string{
			"khegde@chromium.org",                 // fixture maintainer
			"stevenjb@chromium.org",               // fixture maintainer
			"cros-network-health-team@google.com", // Network Health team
		},
		BugComponent:    "b:1166446", // ChromeOS > Platform > Connectivity > NetworkHealth
		PreTestTimeout:  shill.ResetShillTimeout + 5*time.Second,
		PostTestTimeout: 5 * time.Second,
		TearDownTimeout: shill.ResetShillTimeout + 5*time.Second,
		Impl:            &shillFixture{},
		Params: []testing.FixtureParam{
			// The default fixture using no param.
			{},
			// The fixture using Ethernet-hide.
			{
				Name:   "ehide",
				Parent: "ehide",
			},
		},
	})
}

// shillFixture implements testing.FixtureImpl.
type shillFixture struct {
	netUnlock func()
}

func (f *shillFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	return nil
}

func (f *shillFixture) Reset(ctx context.Context) error {
	return nil
}

func (f *shillFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
	// We lose connectivity along the way here, and if that races with the
	// recover_duts network-recovery hooks, it may interrupt us. This is
	// automatically unlocked after 30 minutes, so unlock and lock it between
	// each test.
	success := false
	unlock, err := network.LockCheckNetworkHook(ctx)
	if err != nil {
		s.Fatal("Failed to lock the check network hook: ", err)
	}
	defer func() {
		if !success {
			unlock()
		}
	}()

	if errs := shill.ResetShill(ctx); len(errs) != 0 {
		for _, err := range errs {
			s.Error("ResetShill error: ", err)
		}
		s.Fatal("Failed resetting shill in PreTest")
	}

	success = true
	f.netUnlock = unlock
}

func (f *shillFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
	f.netUnlock()
}

func (f *shillFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	// Restart ui so that cryptohome unmounts all user mounts before shill is
	// restarted so that shill does not keep the mounts open perpetually.
	// TODO(b/205726835): Remove once the mount propagation for shill is fixed.
	if err := upstart.RestartJob(ctx, "ui"); err != nil {
		s.Error("Failed to restart ui: ", err)
	}

	if errs := shill.ResetShill(ctx); len(errs) != 0 {
		for _, err := range errs {
			s.Error("ResetShill error: ", err)
		}
		s.Error("Failed resetting shill in TearDown")
	}
}
