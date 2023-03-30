// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"time"

	"chromiumos/tast/common/fixture"
	"chromiumos/tast/common/pci"
	"chromiumos/tast/common/policy"
	"chromiumos/tast/common/policy/fakedms"
	"chromiumos/tast/ctxutil"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/ash"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/role"
	"chromiumos/tast/local/personalization"
	"chromiumos/tast/local/policyutil"
	"chromiumos/tast/testing"
)

const (
	chooseFromFileButtonName = "Choose a file"
	takePhotoButtonName      = "Take a photo"
	takeVideoButtonName      = "Create a looping video"
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
		Attr:         []string{"group:commercial_limited"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      3 * time.Minute,
		Fixture:      fixture.ChromePolicyLoggedIn,
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
//
// Note: Google profile image selector is not tested here since it requires a
// real GAIA user.
func UserAvatarCustomization(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

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

	for _, param := range []struct {
		name                      string
		shouldFindCustomSelectors bool
		policy                    *policy.UserAvatarCustomizationSelectorsEnabled
	}{
		{
			name:                      "unset",
			shouldFindCustomSelectors: true,
			policy:                    &policy.UserAvatarCustomizationSelectorsEnabled{Stat: policy.StatusUnset},
		},
		{
			name:                      "enabled",
			shouldFindCustomSelectors: true,
			policy:                    &policy.UserAvatarCustomizationSelectorsEnabled{Val: true},
		},
		{
			name:                      "disabled",
			shouldFindCustomSelectors: false,
			policy:                    &policy.UserAvatarCustomizationSelectorsEnabled{Val: false},
		},
	} {
		s.Run(ctx, param.name, func(ctx context.Context, s *testing.State) {
			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Fatal("Failed to clean up: ", err)
			}

			if err := policyutil.ServeAndVerify(ctx, fdms, cr, []policy.Policy{param.policy}); err != nil {
				s.Fatal("Failed to update policies: ", err)
			}

			ui := uiauto.New(tconn).WithTimeout(30 * time.Second)

			// Open avatar subpage.
			if err := uiauto.Combine("open avatar subpage",
				personalization.OpenPersonalizationHub(ui),
				personalization.OpenAvatarSubpage(ui),
			)(ctx); err != nil {
				s.Fatal("Failed to open avatar subpage: ", err)
			}

			chooseFromFileSelector := selectorFinder(chooseFromFileButtonName)
			takePhotoSelector := selectorFinder(takePhotoButtonName)
			takeVideoSelector := selectorFinder(takeVideoButtonName)

			if param.shouldFindCustomSelectors {
				if err := uiauto.Combine("Verify custom selectors are shown",
					ui.WaitUntilExists(chooseFromFileSelector),
					ui.WaitUntilExists(takePhotoSelector),
					ui.WaitUntilExists(takeVideoSelector),
				)(ctx); err != nil {
					s.Fatal("Failed to verify custom selectors are shown: ", err)
				}
			} else {
				if err := uiauto.Combine("Verify custom selectors are not shown",
					ui.WaitUntilGone(chooseFromFileSelector),
					ui.WaitUntilGone(takePhotoSelector),
					ui.WaitUntilGone(takeVideoSelector),
				)(ctx); err != nil {
					s.Fatal("Failed to verify custom selectors are not shown: ", err)
				}
			}
		})
	}
}

func selectorFinder(name string) *nodewith.Finder {
	return nodewith.
		Role(role.ListBoxOption).
		Name(name).
		HasClass("avatar-button-container")
}
