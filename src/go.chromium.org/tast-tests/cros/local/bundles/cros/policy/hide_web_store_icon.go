// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/launcher"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         HideWebStoreIcon,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Behavior of HideWebStoreIcon policy, check if a Web Store Icon is displayed in app launcher based on the value of the policy",
		Contacts: []string{
			"cros-engprod-muc@google.com",
			"kamilszarek@google.com", // Test owner
		},
		BugComponent: "b:1263917",
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:golden_tier", "group:hw_agnostic"},
		Fixture:      fixture.ChromePolicyLoggedIn,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.PinnedLauncherApps{}, pci.VerifiedFunctionalityUI),
			pci.SearchFlag(&policy.HideWebStoreIcon{}, pci.VerifiedFunctionalityUI),
		},
	})
}

// HideWebStoreIcon tests the HideWebStoreIcon policy.
func HideWebStoreIcon(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	// Connect to Test API to use it with the UI library.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	pinWebStoreApp := &policy.PinnedLauncherApps{Val: []string{apps.WebStore.ID}}

	for _, param := range []struct {
		name     string
		wantIcon bool            // wantIcon is the expected existence of the "Web Store" icon.
		policies []policy.Policy // policies is the policies that will be set.
	}{
		{
			name:     "hide",
			wantIcon: false,
			policies: []policy.Policy{pinWebStoreApp, &policy.HideWebStoreIcon{Val: true}},
		},
		{
			name:     "show",
			wantIcon: true,
			policies: []policy.Policy{pinWebStoreApp, &policy.HideWebStoreIcon{Val: false}},
		},
		{
			name:     "unset",
			wantIcon: true,
			policies: []policy.Policy{pinWebStoreApp, &policy.HideWebStoreIcon{Stat: policy.StatusUnset}},
		},
	} {
		s.Run(ctx, param.name, func(ctx context.Context, s *testing.State) {
			defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree_"+param.name)

			// Perform cleanup.
			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Fatal("Failed to clean up: ", err)
			}

			// Update policies.
			if err := policyutil.ServeAndVerify(ctx, fdms, cr, param.policies); err != nil {
				s.Fatal("Failed to update policies: ", err)
			}

			tabletMode, err := ash.TabletModeEnabled(ctx, tconn)
			if err != nil {
				s.Fatal("Failed to get tablet mode status: ", err)
			}

			uia := uiauto.New(tconn)
			// In desktop mode user needs to bring up the application grid.
			// In tablet mode the application grid is already up.
			if !tabletMode {
				// Open Launcher and go to Apps list view page.
				// Tried to use launcher.OpenExpandedView(tconn) but it seems to be flaky, after some testing
				// it seems to be mostly flaky at ash.WaitForLauncherState(ctx, tconn, ash.FullscreenAllApps).
				if err := uiauto.Combine("Open Launcher and go to Expanded Apps list view",
					uia.WithInterval(500*time.Millisecond).LeftClickUntil(
						launcher.HomeButtonFinder,
						uia.Exists(nodewith.HasClass("AppListBubbleView")),
					),
				)(ctx); err != nil {
					s.Fatal("Failed to open Apps list view: ", err)
				}
			}

			appName := apps.WebStore.Name

			// Confirm the status of the Web Store icon in the application launcher.
			// Use First() as on the application launcher WebStore may show up twice.
			// Once on the applications grid and the second on the "Recent" section.
			if err := policyutil.WaitUntilExistsStatus(ctx, tconn, nodewith.Name(appName).HasClass("AppListItemView").First(), param.wantIcon, 15*time.Second); err != nil {
				s.Error("Could not confirm the desired status of the Web Store Icon in the application launcher: ", err)
			}

			// Confirm the status of the Web Store icon on the shelf.
			shelfAppButtonRegex := regexp.MustCompile(ash.ShelfAppButtonClassNameRegex)
			if err := policyutil.WaitUntilExistsStatus(ctx, tconn, nodewith.Name(appName).ClassNameRegex(shelfAppButtonRegex), param.wantIcon, 15*time.Second); err != nil {
				s.Error("Could not confirm the desired status of the Web Store Icon on the system shelf: ", err)
			}
		})
	}
}
