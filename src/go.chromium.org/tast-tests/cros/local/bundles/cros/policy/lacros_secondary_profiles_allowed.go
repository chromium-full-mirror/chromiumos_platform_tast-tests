// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         LacrosSecondaryProfilesAllowed,
		Desc:         "Behavior of LacrosSecondaryProfilesAllowed policy",
		LacrosStatus: testing.LacrosVariantExists,
		Contacts: []string{
			"lacros-team@google.com",
			"anastasiian@chromium.org", // Test author
		},
		BugComponent: "crbug:OS>LaCrOS",
		SoftwareDeps: []string{"chrome", "lacros"},
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		Fixture:      fixture.LacrosPolicyLoggedIn,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.LacrosSecondaryProfilesAllowed{}, pci.VerifiedFunctionalityUI),
		},
	})
}

func LacrosSecondaryProfilesAllowed(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	// Browser controls to open a profile:
	profileToolbarButton := nodewith.ClassName("AvatarToolbarButton").Role(role.Button).Focusable()
	profileMenu := nodewith.NameStartingWith("Accounts and sync").Role(role.Menu)
	otherProfilesLabel := nodewith.Name("Other profiles").Role(role.StaticText).Ancestor(profileMenu)

	// 'Add' and 'Guest' profile buttons.
	addProfileButton := nodewith.Name("Add").Role(role.Button).Focusable().Ancestor(profileMenu)
	guestProfileButton := nodewith.Name("Guest").Role(role.Button).Focusable().Ancestor(profileMenu)

	// Reserve ten seconds for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	for _, param := range []struct {
		// name is the subtest name.
		name string
		// value is the policy value.
		value *policy.LacrosSecondaryProfilesAllowed
		// newProfileOrGuestAllowed is the policy result
		newProfileOrGuestAllowed bool
	}{
		{
			name:                     "true",
			value:                    &policy.LacrosSecondaryProfilesAllowed{Val: true},
			newProfileOrGuestAllowed: true,
		},
		{
			name:                     "false",
			value:                    &policy.LacrosSecondaryProfilesAllowed{Val: false},
			newProfileOrGuestAllowed: false,
		},
		{
			name:                     "unset",
			value:                    &policy.LacrosSecondaryProfilesAllowed{Stat: policy.StatusUnset},
			newProfileOrGuestAllowed: false,
		},
	} {
		s.Run(ctx, param.name, func(ctx context.Context, s *testing.State) {
			// Perform cleanup.
			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Fatal("Failed to clean up: ", err)
			}

			// Update policies.
			if err := policyutil.ServeAndRefresh(ctx, fdms, cr, []policy.Policy{param.value}); err != nil {
				s.Fatal("Failed to update policies: ", err)
			}

			// Setup the browser.
			cr, l, _, err := lacros.Setup(ctx, s.FixtValue(), browser.TypeLacros)
			if err != nil {
				s.Fatal("Failed to initialize test: ", err)
			}
			defer lacros.CloseLacros(cleanupCtx, l)

			// Connect to Test API to use it with the UI library.
			tconn, err := cr.TestAPIConn(ctx)
			if err != nil {
				s.Fatal("Failed to connect Test API: ", err)
			}
			defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree_"+param.name)

			ui := uiauto.New(tconn)

			if err := uiauto.Combine("Open profile toolbar",
				ui.WaitUntilExists(profileToolbarButton),
				ui.LeftClick(profileToolbarButton),
				ui.WaitUntilExists(otherProfilesLabel),
			)(ctx); err != nil {
				s.Fatal("Failed to open profile toolbar: ", err)
			}

			// Test 'Add profile' button.
			newProfileEnabled := true
			if err := ui.Exists(addProfileButton)(ctx); err != nil {
				newProfileEnabled = false
			}
			if newProfileEnabled != param.newProfileOrGuestAllowed {
				s.Errorf("Unexpected new profile behavior: got %t; want %t", newProfileEnabled, param.newProfileOrGuestAllowed)
			}

			// Test 'Guest' button.
			guestProfileEnabled := true
			if err := ui.Exists(guestProfileButton)(ctx); err != nil {
				guestProfileEnabled = false
			}
			if guestProfileEnabled != param.newProfileOrGuestAllowed {
				s.Fatalf("Unexpected guest profile behavior: got %t; want %t", guestProfileEnabled, param.newProfileOrGuestAllowed)
			}
		})
	}
}
