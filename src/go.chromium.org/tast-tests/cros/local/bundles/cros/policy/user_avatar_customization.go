// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/chrome/credconfig"
	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/policy/useravatar"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/netexport"
	"go.chromium.org/tast-tests/cros/local/policyutil"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         UserAvatarCustomization,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies behavior of UserAvatarCustomizationSelectorsEnabled policy",
		Contacts: []string{
			"dp-chromeos-eng@google.com",
			"chiav@google.com",
		},
		BugComponent: "b:1129862",
		Attr:         []string{"group:golden_tier"},
		SoftwareDeps: []string{"chrome"},
		VarDeps:      []string{"policy.managedUserAccountPool"},
		Timeout:      5 * time.Minute,
		Fixture:      fixture.FakeDMS,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.UserAvatarCustomizationSelectorsEnabled{},
				pci.VerifiedFunctionalityUI),
		},
	})
}

// UserAvatarCustomization verifies policy behavior:
//  1. Set UserAvatarCustomizationSelectorsEnabled policy value
//  2. Open user avatar settings
//  3. Verify that custom selectors are shown or not shown, based on policy
//  4. Verify network annotation for profile image fetch is controlled by policy
func UserAvatarCustomization(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	gaiaCreds, err := credconfig.PickRandomCreds(
		s.RequiredVar("policy.managedUserAccountPool"))
	if err != nil {
		s.Fatal("Failed to parse managed user creds: ", err)
	}

	policyBlob := policy.NewBlob()
	policyBlob.PolicyUser = gaiaCreds.User
	if err := fdms.WritePolicyBlob(policyBlob); err != nil {
		s.Fatal("Failed to write policies to FakeDMS: ", err)
	}

	opts := []chrome.Option{
		chrome.DMSPolicy(fdms.URL),  // FakeDMS for setting policies
		chrome.GAIALogin(gaiaCreds), // Real GAIA to enable calendar_get_events call
	}
	// Enable netlog on startup, since the profile image is fetched on startup.
	opts = append(opts, netexport.CommandLineArgs(browser.TypeAsh)...)

	for index, param := range useravatar.TestCases() {
		s.Run(ctx, param.Name, func(ctx context.Context, s *testing.State) {
			// Start Chrome. Note that we restart Chrome for each test case to reset the netlog.
			cr, err := chrome.New(ctx, opts...)
			if err != nil {
				s.Fatal("Failed to start Chrome: ", err)
			}
			defer cr.Close(ctx)

			// Create API connection.
			tconn, err := cr.TestAPIConn(ctx)
			if err != nil {
				s.Fatal("Failed to create Test API connection: ", err)
			}

			// Force Chrome to be in clamshell mode to make sure it's possible to close
			// the personalization hub.
			cleanup, err := ash.EnsureTabletModeEnabled(ctx, tconn, false)
			if err != nil {
				s.Fatal("Failed to ensure DUT is not in tablet mode: ", err)
			}
			defer cleanup(cleanupCtx)

			defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(),
				s.HasError, cr, "ui_tree")

			netExport, err := netexport.FromCommandLineArg(browser.TypeAsh)
			if err != nil {
				s.Fatal("Failed to get net export session: ", err)
			}

			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Fatal("Failed to clean up: ", err)
			}

			// Update policies.
			policies := []policy.Policy{param.Policy}
			policyBlob := policy.NewBlob()
			policyBlob.PolicyUser = gaiaCreds.User
			policyBlob.AddPolicies(policies)
			if err := policyutil.ServeBlobAndRefresh(ctx, fdms, cr, policyBlob); err != nil {
				s.Fatal("Failed to update policies: ", err)
			}
			if err := policyutil.Verify(ctx, tconn, policies); err != nil {
				s.Fatal("Failed to verify updated policies: ", err)
			}

			if err := useravatar.TriggerUserAvatarCustomization(ctx, nil, nil, nil, tconn, index); err != nil {
				s.Fatal("Failed to trigger and verify user avatar customization: ", err)
			}

			// Check netlog for network annotation.
			foundAnnotation, err := netExport.Find(useravatar.AnnotationHashCode)
			if err != nil {
				s.Fatal("Failed to search net log file for annotation: ", err)
			}
			if param.ShouldFindAnnotation != foundAnnotation {
				s.Fatalf("Annotation mismatch. Expected: %t. Actual: %t", param.ShouldFindAnnotation, foundAnnotation)
			}
		})
	}
}
