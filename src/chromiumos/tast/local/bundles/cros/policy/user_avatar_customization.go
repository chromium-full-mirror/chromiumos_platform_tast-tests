// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"time"

	"chromiumos/tast/common/chrome/credconfig"
	"chromiumos/tast/common/fixture"
	"chromiumos/tast/common/pci"
	"chromiumos/tast/common/policy"
	"chromiumos/tast/common/policy/fakedms"
	"chromiumos/tast/local/annotations"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/ash"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/role"
	"chromiumos/tast/local/personalization"
	"chromiumos/tast/local/policyutil"

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
	const (
		chooseFromFileButtonName = "Choose a file"
		takePhotoButtonName      = "Take a photo"
		takeVideoButtonName      = "Create a looping video"
		profileImageName         = "Google profile photo"

		avatarButtonContainerClass = "avatar-button-container"
		imageContainerClass        = "image-container"

		annotationID = "108903331" // signed_in_profile_avatar
	)

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
		chrome.DMSPolicy(fdms.URL),        // FakeDMS for setting policies
		chrome.GAIALogin(gaiaCreds),       // Real GAIA to enable calendar_get_events call
		chrome.ExtraArgs("--log-net-log"), // Enable netlog on startup
	}

	for _, param := range []struct {
		name                      string
		shouldFindAnnotation      bool
		shouldFindCustomSelectors bool
		policy                    *policy.UserAvatarCustomizationSelectorsEnabled
	}{
		{
			name:                      "unset",
			shouldFindAnnotation:      true,
			shouldFindCustomSelectors: true,
			policy:                    &policy.UserAvatarCustomizationSelectorsEnabled{Stat: policy.StatusUnset},
		},
		{
			name:                      "enabled",
			shouldFindAnnotation:      true,
			shouldFindCustomSelectors: true,
			policy:                    &policy.UserAvatarCustomizationSelectorsEnabled{Val: true},
		},
		{
			name:                      "disabled",
			shouldFindAnnotation:      false,
			shouldFindCustomSelectors: false,
			policy:                    &policy.UserAvatarCustomizationSelectorsEnabled{Val: false},
		},
	} {
		s.Run(ctx, param.name, func(ctx context.Context, s *testing.State) {
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

			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Fatal("Failed to clean up: ", err)
			}

			// Update policies.
			policies := []policy.Policy{param.policy}
			policyBlob := policy.NewBlob()
			policyBlob.PolicyUser = gaiaCreds.User
			policyBlob.AddPolicies(policies)
			if err := policyutil.ServeBlobAndRefresh(ctx, fdms, cr, policyBlob); err != nil {
				s.Fatal("Failed to update policies: ", err)
			}
			if err := policyutil.Verify(ctx, tconn, policies); err != nil {
				s.Fatal("Failed to verify updated policies: ", err)
			}

			ui := uiauto.New(tconn).WithTimeout(30 * time.Second)

			// Open personalization hub.
			if err := personalization.OpenPersonalizationHub(ui)(ctx); err != nil {
				s.Fatal("Failed to open personalization hub: ", err)
			}
			// Wait for personalization hub to finish loading.
			if err := tconn.WaitForExpr(ctx, "document.readyState === 'complete'"); err != nil {
				s.Fatal("Failed to wait for personalization hub to load: ", err)
			}
			// Open avatar subpage.
			if err := personalization.OpenAvatarSubpage(ui)(ctx); err != nil {
				s.Fatal("Failed to open avatar subpage: ", err)
			}

			chooseFromFileSelector := selectorFinder(chooseFromFileButtonName, avatarButtonContainerClass)
			takePhotoSelector := selectorFinder(takePhotoButtonName, avatarButtonContainerClass)
			takeVideoSelector := selectorFinder(takeVideoButtonName, avatarButtonContainerClass)
			profileImageSelector := selectorFinder(profileImageName, imageContainerClass)

			if param.shouldFindCustomSelectors {
				if err := uiauto.Combine("Verify custom selectors are shown",
					ui.WaitUntilExists(chooseFromFileSelector),
					ui.WaitUntilExists(takePhotoSelector),
					ui.WaitUntilExists(takeVideoSelector),
					ui.WaitUntilExists(profileImageSelector),
				)(ctx); err != nil {
					s.Fatal("Failed to verify custom selectors are shown: ", err)
				}
			} else {
				if err := uiauto.Combine("Verify custom selectors are not shown",
					ui.WaitUntilGone(chooseFromFileSelector),
					ui.WaitUntilGone(takePhotoSelector),
					ui.WaitUntilGone(takeVideoSelector),
					ui.WaitUntilGone(profileImageSelector),
				)(ctx); err != nil {
					s.Fatal("Failed to verify custom selectors are not shown: ", err)
				}
			}

			// Check netlog for network annotation.
			foundAnnotation, err := annotations.CheckLogsFromFile(ctx, cr, annotationID, annotations.UserDirNetLogFile)
			if param.shouldFindAnnotation != foundAnnotation {
				s.Fatalf("Annotation mismatch. Expected: %t. Actual: %t", param.shouldFindAnnotation, foundAnnotation)
			}
		})
	}
}

func selectorFinder(name, class string) *nodewith.Finder {
	return nodewith.Role(role.ListBoxOption).Name(name).HasClass(class)
}
