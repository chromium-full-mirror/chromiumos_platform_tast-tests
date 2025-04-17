// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package flex

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/policyutil/fixtures"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:            "flexARCVM",
		Desc:            "Base fixture for ChromeOS Flex ARCVM tests",
		Contacts:        []string{"chromeos-flex-eng+oncall@google.com", "josephsussman@google.com"},
		BugComponent:    "b:998633", // ChromeOS > Platform > Enablement > ChromeOS Flex
		Impl:            &flexARCVMImpl{},
		SetUpTimeout:    chrome.ManagedUserLoginTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		PostTestTimeout: chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
		Parent:          fixture.FakeDMSEnrolled,
		Vars:            []string{"ui.signinProfileTestExtensionManifestKey"},
	})
}

// FixtData is the data returned by SetUp and passed to tests.
type FixtData struct {
	// fakeDMS is an already running DMS server.
	fakeDMS *fakedms.FakeDMS
	// chrome is a connection to an already-started Chrome instance that loads policies from FakeDMS.
	chrome *chrome.Chrome
}

// flexARCVMImpl implements testing.FixtureImpl.
type flexARCVMImpl struct {
	// chrome is a connection to an already-started Chrome instance that loads policies from FakeDMS.
	cr *chrome.Chrome
	// fakeDMS is an already running DMS server.
	fdms *fakedms.FakeDMS
}

// SetUp is called once before the first test starts.
func (i *flexARCVMImpl) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	fdms, ok := s.ParentValue().(*fakedms.FakeDMS)
	if !ok {
		s.Fatal("Parent is not a FakeDMS fixture")
	}
	i.fdms = fdms

	// Start a Chrome instance and fetch policies from the login screen.
	cr, err := chrome.New(ctx,
		chrome.DMSPolicy(i.fdms.URL),
		chrome.NoLogin(),
		chrome.LoadSigninProfileExtension(s.RequiredVar("ui.signinProfileTestExtensionManifestKey")),
		chrome.KeepState(),
	)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}

	policies := []policy.Policy{
		&policy.DeviceFlexArcPreloadEnabled{Stat: policy.StatusSet, Val: true},
	}
	// Update policies from the login screen.
	if err := policyutil.ServeAndVerifyOnLoginScreen(ctx, i.fdms, cr, policies); err != nil {
		s.Fatal("Failed to serve and refresh: ", err)
	}

	// Close the previous Chrome instance.
	if err := cr.Close(ctx); err != nil {
		s.Error("Failed to close Chrome connection: ", err)
	}

	// Restart Chrome to reload policies.
	cr, err = chrome.New(ctx,
		chrome.FakeLogin(chrome.Creds{User: fixtures.Username, Pass: fixtures.Password}),
		chrome.DMSPolicy(fdms.URL),
		chrome.KeepEnrollment())
	if err != nil {
		s.Fatal("Chrome login failed: ", err)
	}
	i.cr = cr

	return &FixtData{i.fdms, i.cr}
}

// PreTest runs before every test.
func (i *flexARCVMImpl) PreTest(ctx context.Context, s *testing.FixtTestState) {
}

// PostTest runs after every test.
func (i *flexARCVMImpl) PostTest(ctx context.Context, s *testing.FixtTestState) {
	// Reset Chrome state.
	if err := i.cr.ResetState(ctx); err != nil {
		s.Fatal("Failed resetting existing Chrome session: ", err)
	}
}

// Reset runs after all but the last test to roll back changes made to the environment.
func (i *flexARCVMImpl) Reset(ctx context.Context) error {
	// Check the connection to Chrome.
	if err := i.cr.Responded(ctx); err != nil {
		return errors.Wrap(err, "existing Chrome connection is unusable")
	}
	return nil
}

// TearDown is called once just after the last test completes, unless SetUp fails.
func (i *flexARCVMImpl) TearDown(ctx context.Context, s *testing.FixtState) {
	// Fail just in case.
	if i.cr == nil {
		s.Fatal("Chrome not yet started")
	}
	if err := i.cr.Close(ctx); err != nil {
		s.Error("Failed to close Chrome connection: ", err)
	}
}
