// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package lockscreen

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/lockscreen"
	"go.chromium.org/tast-tests/cros/local/chrome/userutil"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/vdi/fixtures"
	"go.chromium.org/tast/core/testing"
)

const testTimeout = 2*chrome.LoginTimeout + userutil.TakingOwnershipTimeout + 2*time.Minute

func init() {
	testing.AddTest(&testing.Test{
		Func:         EphemeralPasswordUnlock,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test checks if a user can unlock screen when DeviceEphemeralUsersEnabled policy is used",
		Contacts: []string{
			"cros-lurs@google.com",
			"kamilszarek@google.com",
		},
		Timeout:      testTimeout,
		BugComponent: "b:1207311", // ChromeOS > Software > Commercial (Enterprise) > Identity > LURS
		SoftwareDeps: []string{"chrome"},
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
			"group:release-health",
			"release-health_enterprise",
		},
		Fixture: fixture.FakeDMSEnrolled,
		// It is needed as serving policies on SignIn screen will talk to
		// testing extension on the SignIn screen.
		VarDeps: []string{"ui.signinProfileTestExtensionManifestKey"},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DeviceEphemeralUsersEnabled{}, pci.VerifiedFunctionalityOS),
		},
	})
}

func EphemeralPasswordUnlock(ctx context.Context, s *testing.State) {
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	// Login to later load the policies.
	cr, err := chrome.New(
		ctx,
		chrome.NoLogin(),
		chrome.KeepEnrollment(),
		chrome.DMSPolicy(fdms.URL),
		chrome.LoadSigninProfileExtension(s.RequiredVar("ui.signinProfileTestExtensionManifestKey")),
	)
	if err != nil {
		s.Fatal("Chrome login failed: ", err)
	}

	// Set the DeviceEphemeralUsersEnabled device policy.
	pb := policy.NewBlob()
	pb.AddPolicies([]policy.Policy{&policy.DeviceEphemeralUsersEnabled{Val: true}})
	if err := policyutil.ServeBlobAndRefreshOnLoginScreen(ctx, fdms, cr, pb); err != nil {
		s.Fatal("Failed to server policy blob and refresh: ", err)
	}

	// Restart Chrome as device policies are not dynamically refreshed.
	cr, err = chrome.New(
		ctx,
		chrome.FakeLogin(chrome.Creds{User: fixtures.Username, Pass: fixtures.Password}),
		chrome.DMSPolicy(fdms.URL),
		chrome.KeepEnrollment(),
		chrome.EphemeralUser(),
	)
	if err != nil {
		s.Fatal("Failed to start chrome after applying DeviceEphemeralUsersEnabled policy: ", err)
	}

	defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree")

	// Connect to Test API.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	// Open a keyboard device.
	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to open keyboard device: ", err)
	}
	defer kb.Close(ctx)

	// Lock the screen.
	if err := lockscreen.Lock(ctx, tconn); err != nil {
		s.Fatal("Failed to lock the screen: ", err)
	}

	if st, err := lockscreen.WaitState(ctx, tconn, func(st lockscreen.State) bool { return st.Locked && st.ReadyForPassword }, 30*time.Second); err != nil {
		s.Fatalf("Waiting for screen to be locked failed: %v (last status %+v)", err, st)
	}

	// Enter and submit the local password to unlock the DUT.
	if err := lockscreen.EnterPassword(ctx, tconn, fixtures.Username, fixtures.Password, kb); err != nil {
		s.Fatal("Failed to enter password: ", err)
	}

	if st, err := lockscreen.WaitState(ctx, tconn, func(st lockscreen.State) bool { return !st.Locked }, 30*time.Second); err != nil {
		s.Fatalf("Waiting for screen to be unlocked failed: %v (last status %+v)", err, st)
	}
}
