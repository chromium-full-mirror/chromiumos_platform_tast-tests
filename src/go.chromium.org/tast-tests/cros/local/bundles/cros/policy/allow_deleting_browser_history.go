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
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
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
		Func:         AllowDeletingBrowserHistory,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Behavior of AllowDeletingBrowserHistory policy, checking the correspoding checkbox states (restriction and checked) after setting the policy",
		Contacts: []string{
			"chrome-enterprise-team@google.com",
		},
		BugComponent: "b:322532",
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:golden_tier", "group:hw_agnostic"},
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
			pci.SearchFlag(&policy.AllowDeletingBrowserHistory{}, pci.VerifiedFunctionalityUI),
		},
	})
}

// AllowDeletingBrowserHistory tests the AllowDeletingBrowserHistory policy.
func AllowDeletingBrowserHistory(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	// Reserve ten seconds for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Connect to Test API to use it with the UI library.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}
	uia := uiauto.New(tconn)

	for _, param := range []struct {
		name            string
		wantRestriction restriction.Restriction             // wantRestriction is the wanted restriction state of the checkboxes in Browsing history.
		wantChecked     checked.Checked                     // wantChecked is the wanted checked state of the checkboxes in Browsing history.
		value           *policy.AllowDeletingBrowserHistory // value is the value of the policy.
	}{
		{
			name:            "unset",
			wantRestriction: restriction.None,
			wantChecked:     checked.True,
			value:           &policy.AllowDeletingBrowserHistory{Stat: policy.StatusUnset},
		},
		{
			name:            "allow",
			wantRestriction: restriction.None,
			wantChecked:     checked.True,
			value:           &policy.AllowDeletingBrowserHistory{Val: true},
		},
		{
			name:            "deny",
			wantRestriction: restriction.Disabled,
			wantChecked:     checked.False,
			value:           &policy.AllowDeletingBrowserHistory{Val: false},
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

			defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree_"+param.name)

			// Open settings page where the affected checkboxes can be found.
			conn, err := br.NewConn(ctx, "chrome://settings/clearBrowserData")
			if err != nil {
				s.Fatal("Failed to connect to the settings page: ", err)
			}
			defer conn.Close()

			// Loop for different checkboxes.
			for _, checkbox := range []struct {
				ref string // ref is the shortened name of the checkbox that can be used in logging.
				tab string // tab is the name of the tab in the UI tree that should be selected to find the checkbox.
			}{
				{
					ref: "Browsing history",
					tab: "Basic",
				},
				{
					ref: "Browsing history",
					tab: "Advanced",
				},
				{
					ref: "Download history",
					tab: "Advanced",
				},
			} {
				// Select the tab if it is not selected already.
				tabNode := nodewith.Name(checkbox.tab).Role(role.Tab)
				tabInfo, err := uia.Info(ctx, tabNode)
				if err != nil {
					s.Fatalf("Failed to find the %s tab: %v", checkbox.tab, err)
				}

				if tabInfo.ClassName != "tab selected" {
					if err := uiauto.Combine("select tab",
						uia.LeftClick(tabNode),
						uia.WaitUntilExists(tabNode.ClassName("tab selected")),
					)(ctx); err != nil {
						s.Fatalf("Failed to select %s tab: %v", checkbox.tab, err)
					}
				}

				if err := policyutil.CurrentPage(cr).
					SelectNode(ctx, nodewith.
						NameStartingWith(checkbox.ref).
						Role(role.CheckBox)).
					Restriction(param.wantRestriction).
					Checked(param.wantChecked).
					Verify(); err != nil {
					s.Errorf("Unexpected settings state for %q: %v", checkbox.ref, err)
				}
			}
		})
	}
}
