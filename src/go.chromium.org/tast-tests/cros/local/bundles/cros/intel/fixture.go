// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package intel

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/caller"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:            "chromeLoggedInRootfsRemoved",
		Desc:            "Removes rootfs verification, if required reboots the DUT and return logged into user session",
		Contacts:        []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		BugComponent:    "b:157291", // ChromeOS > External > Intel
		Impl:            &loggedIn{},
		Parent:          "rootfsRemovedFixture",
		SetUpTimeout:    5 * time.Minute,
		ResetTimeout:    10 * time.Second,
		TearDownTimeout: 10 * time.Second,
	})

	testing.AddFixture(&testing.Fixture{
		Name:            "chromeLoggedInPsrEnableDisable",
		Desc:            "Enables and disables PSR 2, if required reboots the DUT and return logged into user session",
		Contacts:        []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		BugComponent:    "b:157291", // ChromeOS > External > Intel
		Impl:            &loggedIn{},
		Parent:          "enableDisablePsrFixture",
		SetUpTimeout:    5 * time.Minute,
		ResetTimeout:    10 * time.Second,
		TearDownTimeout: 10 * time.Second,
	})
}

// Value holds the Chrome object.
type Value struct {
	cr *chrome.Chrome
}

// loggedIn is a fixture to start Chrome.
type loggedIn struct {
	v *Value
}

// allowedPackages lists packages that are allowed to remove rootfs verification.
var allowedRootfsPackages = []string{
	"go.chromium.org/tast-tests/cros/local/bundles/cros/intel",
}

// checkAllowedPackages checks if the caller is allowed to remove rootfs.
// Preconditions and fixtures should call this function on setup to prevent
// tests from removing rootfs by a mistake.
func checkAllowedPackages() {
	caller.Check(2, allowedRootfsPackages)
}

// This fixture will remove rootfs which is an irreversible action, which
// cannot be cleaned up in Teardown. Hence only Intel groups should use this
// fixture and with caution. The tests that are using this fixture should not
// be run with other tests in the same Tast session.
func (f *loggedIn) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	checkAllowedPackages()
	cr, err := chrome.New(ctx)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	f.v = &Value{cr: cr}
	return cr
}

func (f *loggedIn) TearDown(ctx context.Context, s *testing.FixtState) {
	if err := f.v.cr.Close(ctx); err != nil {
		s.Error("Failed to close Chrome connection: ", err)
	}
	f.v.cr = nil
}

func (f *loggedIn) Reset(ctx context.Context) error {
	if err := f.v.cr.Responded(ctx); err != nil {
		return errors.Wrap(err, "existing Chrome connection is unusable")
	}
	if err := f.v.cr.ResetState(ctx); err != nil {
		return errors.Wrap(err, "failed resetting existing Chrome session")
	}
	return nil
}

func (f *loggedIn) PreTest(ctx context.Context, s *testing.FixtTestState) {
	// No-op.
}

func (f *loggedIn) PostTest(ctx context.Context, s *testing.FixtTestState) {
	// No-op.
}
