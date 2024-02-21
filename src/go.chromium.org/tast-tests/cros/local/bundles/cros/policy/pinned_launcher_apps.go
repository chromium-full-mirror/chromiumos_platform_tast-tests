// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PinnedLauncherApps,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test the behavior of PinnedLauncherApps policy: apps in the policy are pinned on the shelf and cannot be unpinned",
		Contacts: []string{
			"cros-engprod-muc@google.com",
			"snijhara@google.com", // Test author
		},
		BugComponent: "b:1263917",
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:golden_tier", "group:hw_agnostic"},
		Fixture:      fixture.ChromePolicyLoggedIn,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.PinnedLauncherApps{}, pci.VerifiedFunctionalityUI),
		},
	})
}

func PinnedLauncherApps(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	// Connect to Test API to use it with the UI library.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)

	// Perform cleanup.
	if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
		s.Fatal("Failed to clean up: ", err)
	}

	// Update the policy to pin the files app.
	policyValue := policy.PinnedLauncherApps{Val: []string{apps.Files.ID}}
	if err := policyutil.ServeAndVerify(ctx, fdms, cr, []policy.Policy{&policyValue}); err != nil {
		s.Fatal("Failed to update policies: ", err)
	}

	ui := uiauto.New(tconn)
	filesAppShelfButton := nodewith.Name(apps.Files.Name).ClassName(ash.ShelfAppButtonClassName)
	unpinContextMenuItem := nodewith.Name("Unpin").ClassName("MenuItemView")
	if err := uiauto.Combine("check unpin option is not present for pinned app",
		ui.RightClick(filesAppShelfButton),
		ui.WaitUntilExists(nodewith.Name("New window").ClassName("MenuItemView")),
		ui.WaitUntilGone(unpinContextMenuItem),
		// This extra left click is needed to dismiss the context menu.
		ui.LeftClick(filesAppShelfButton),
	)(ctx); err != nil {
		s.Fatal("Failed to check unpin option in context menu: ", err)
	}

	// Reset the policy so that files app is no longer pinned.
	if err := policyutil.ServeAndVerify(ctx, fdms, cr, []policy.Policy{}); err != nil {
		s.Fatal("Failed to update policies: ", err)
	}

	if err := uiauto.Combine("check app can be unpinned after policy reset",
		ui.RightClick(filesAppShelfButton),
		ui.WaitUntilExists(unpinContextMenuItem),
		ui.LeftClick(unpinContextMenuItem),
		ui.WaitUntilExists(nodewith.Name(apps.Files.Name+" was un-pinned")),
	)(ctx); err != nil {
		s.Error("Failed to unpin the app after policy reset: ", err)
	}
}
