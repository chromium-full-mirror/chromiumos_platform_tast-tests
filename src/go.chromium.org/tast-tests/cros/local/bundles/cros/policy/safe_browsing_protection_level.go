// Copyright 2020 The ChromiumOS Authors
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
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/checked"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/restriction"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SafeBrowsingProtectionLevel,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks if Google Chrome's Safe Browsing feature is enabled and the mode it operates in",
		Contacts: []string{
			"cros-engprod-muc@google.com",
			"mohamedaomar@google.com", // Test author
		},
		BugComponent: "b:1263917",
		Attr:         []string{"group:golden_tier", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{{
			Fixture: fixture.ChromePolicyLoggedIn,
			Val:     browser.TypeAsh,
		}, {
			Name:              "lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Fixture:           fixture.LacrosPolicyLoggedIn,
			Val:               browser.TypeLacros,
		}},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.SafeBrowsingProtectionLevel{}, pci.VerifiedFunctionalityUI),
		},
	})
}

func SafeBrowsingProtectionLevel(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	// Reserve ten seconds for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	for _, param := range []struct {
		name            string
		wantRestriction restriction.Restriction // wantRestriction is the wanted restriction state of the radio buttons in Safe Browsing settings page.
		selectedOption  string                  // selectedOption is the selected safety level in Safe Browsing settings page.
		value           *policy.SafeBrowsingProtectionLevel
	}{
		{
			name:            "unset",
			wantRestriction: restriction.None,
			selectedOption:  "Standard protection",
			value:           &policy.SafeBrowsingProtectionLevel{Stat: policy.StatusUnset},
		},
		{
			name:            "no_protection",
			wantRestriction: restriction.Disabled,
			selectedOption:  "No protection (not recommended)",
			value:           &policy.SafeBrowsingProtectionLevel{Val: 0},
		},
		{
			name:            "standard_protection",
			wantRestriction: restriction.Disabled,
			selectedOption:  "Standard protection",
			value:           &policy.SafeBrowsingProtectionLevel{Val: 1},
		},
		{
			name:            "enhanced_protection",
			wantRestriction: restriction.Disabled,
			selectedOption:  "Enhanced protection",
			value:           &policy.SafeBrowsingProtectionLevel{Val: 2},
		},
	} {
		s.Run(ctx, param.name, func(ctx context.Context, s *testing.State) {
			// Perform cleanup.
			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Fatal("Failed to clean up: ", err)
			}

			// Update policies.
			if err := policyutil.ServeAndVerify(ctx, fdms, cr, []policy.Policy{param.value}); err != nil {
				s.Fatal("Failed to update policies: ", err)
			}

			// Setup browser based on the chrome type.
			br, closeBrowser, err := browserfixt.SetUp(ctx, cr, s.Param().(browser.Type))
			if err != nil {
				s.Fatal("Failed to open the browser: ", err)
			}
			defer closeBrowser(cleanupCtx)

			defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree_"+param.name)

			// Open the security settings page.
			if err := policyutil.SettingsPage(ctx, cr, br, "security").
				SelectNode(ctx, nodewith.
					Role(role.RadioButton).
					Name(param.selectedOption)).
				Checked(checked.True).
				Restriction(param.wantRestriction).
				Verify(); err != nil {
				s.Error("Unexpected OS settings state: ", err)
			}
		})
	}
}
