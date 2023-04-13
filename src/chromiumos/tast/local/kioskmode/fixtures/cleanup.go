// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package fixtures contains fixtures useful for Kiosk mode tests.
package fixtures

import (
	"context"

	"chromiumos/tast/common/fixture"
	"chromiumos/tast/common/policy"
	"chromiumos/tast/common/policy/fakedms"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/policyutil"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: fixture.KioskAutoLaunchCleanup,
		Desc: "Fixture should be used when kioksmode.AutoLaunch() option is passed when creating Kiosk session using kioskmode.New()",
		Contacts: []string{
			"kamilszarek@google.com",
			"alt-modalities-stability@google.com",
		},
		Impl:            &kiosk{},
		TearDownTimeout: chrome.ManagedUserLoginTimeout,
		ResetTimeout:    chrome.ManagedUserLoginTimeout,
		Parent:          fixture.FakeDMSEnrolled,
		Vars: []string{
			"ui.signinProfileTestExtensionManifestKey",
		},
	})
}

type kiosk struct {
	fakeDMS *fakedms.FakeDMS

	signinTestExtensionManifestKey string
}

// FixtData is fixture return data.
type FixtData struct {
	fakeDMS *fakedms.FakeDMS
}

// FakeDMS implements the HasFakeDMS interface.
func (fd FixtData) FakeDMS() *fakedms.FakeDMS {
	if fd.fakeDMS == nil {
		panic("FakeDMS is called with nil fakeDMS instance")
	}
	return fd.fakeDMS
}

func (k *kiosk) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	fdms, ok := s.ParentValue().(*fakedms.FakeDMS)
	if !ok {
		s.Fatal("Parent is not a fakeDMSEnrolled fixture")
	}
	k.fakeDMS = fdms
	k.signinTestExtensionManifestKey = s.RequiredVar("ui.signinProfileTestExtensionManifestKey")
	return FixtData{fakeDMS: fdms}
}

func (k *kiosk) TearDown(ctx context.Context, s *testing.FixtState) {
	s.Log("Kiosk clean up fixture tear down: Starting Chrome to clean policies")
	defer s.Log("Kiosk clean up fixture tear down finished")

	if err := k.CleanupKioskPolicies(ctx); err != nil {
		s.Fatal("Failed to cleaup kiosk policies: ", err)
	}
}

func (k *kiosk) Reset(ctx context.Context) error {
	testing.ContextLog(ctx, "Kiosk clean up fixture reset: Starting Chrome to clean policies")
	defer testing.ContextLog(ctx, "Kiosk clean up fixture reset finished")

	return k.CleanupKioskPolicies(ctx)
}

func (k *kiosk) PreTest(ctx context.Context, s *testing.FixtTestState)  {}
func (k *kiosk) PostTest(ctx context.Context, s *testing.FixtTestState) {}

func (k *kiosk) CleanupKioskPolicies(ctx context.Context) (retErr error) {
	cr, err := chrome.New(
		ctx,
		chrome.NoLogin(),
		chrome.DMSPolicy(k.fakeDMS.URL),
		chrome.KeepEnrollment(),
		chrome.LoadSigninProfileExtension(k.signinTestExtensionManifestKey),
		// Use a designated test-only command line switch to prevent kiosk
		// autolaunch which could be enabled by a policy from some test.
		//
		// Chrome makes a decision about kiosk autolaunch very early at startup,
		// so the most reliable way to do it is to stop kiosk autolaunch and reset
		// policies on the login screen.
		chrome.ExtraArgs("--prevent-kiosk-autolaunch-for-testing"),
	)
	if err != nil {
		return errors.Wrap(err, "failed to start Chrome to stay on the login screen and prevent kiosk autolaunch")
	}
	defer func(ctx context.Context) {
		if err := cr.Close(ctx); err != nil {
			if retErr != nil {
				testing.ContextLog(ctx, "Failed to close Chrome started by CleanupKioskPolicies function: ", err)
			} else {
				retErr = errors.Wrap(err, "failed to close Chrome ")
			}
		}
	}(ctx)

	// Some tests may setup kiosk autolaunch policy which affects follow-up
	// tests, so we have to apply empty policies to perform proper cleanup.
	if err := policyutil.ServeAndVerifyOnLoginScreen(ctx, k.fakeDMS, cr, []policy.Policy{}); err != nil {
		return errors.Wrap(err, "could not serve and verify empty policies on login screen")
	}
	return nil
}
