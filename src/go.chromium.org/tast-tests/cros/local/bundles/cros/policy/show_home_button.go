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
		Func: ShowHomeButton,
		Desc: "Test the behavior of ShowHomeButton policy: check if a home button is shown based on the value of the policy",
		Contacts: []string{
			"chrome-desktop-ui-sea@google.com",
			"dpenning@google.com",
			"alexanderhartl@google.com", // Test author
		},
		BugComponent: "b:1457393",
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:golden_tier", "group:hw_agnostic"},
		Fixture:      fixture.ChromePolicyLoggedIn,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.ShowHomeButton{}, pci.VerifiedFunctionalityUI),
		},
	})
}

func ShowHomeButton(ctx context.Context, s *testing.State) {
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

	for _, param := range []struct {
		name       string
		wantButton bool                   // wantButton is the expected existence of the "Home" button.
		policy     *policy.ShowHomeButton // policy is the policy we test.
	}{
		{
			name:       "unset",
			wantButton: false,
			policy:     &policy.ShowHomeButton{Stat: policy.StatusUnset},
		},
		{
			name:       "no_show",
			wantButton: false,
			policy:     &policy.ShowHomeButton{Val: false},
		},
		{
			name:       "show",
			wantButton: true,
			policy:     &policy.ShowHomeButton{Val: true},
		},
	} {
		s.Run(ctx, param.name, func(ctx context.Context, s *testing.State) {
			// Perform cleanup.
			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Fatal("Failed to clean up: ", err)
			}

			// Update policies.
			if err := policyutil.ServeAndVerify(ctx, fdms, cr, []policy.Policy{param.policy}); err != nil {
				s.Fatal("Failed to update policies: ", err)
			}

			defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree_"+param.name)

			conn, err := cr.NewConn(ctx, "")
			if err != nil {
				s.Fatal("Failed to connect to chrome: ", err)
			}
			defer conn.Close()

			// Confirm the status of the Home button node.
			ui := uiauto.New(tconn)
			homeButton := nodewith.Name("Home").Role(role.Button).First()
			if err = ui.WaitUntilExists(homeButton)(ctx); err != nil {
				if !nodewith.IsNodeNotFoundErr(err) {
					s.Fatal("Failed to wait for 'Home' button: ", err)
				}
				if param.wantButton {
					s.Error("'Home' button not found: ", err)
				}
			} else if !param.wantButton {
				s.Error("Unexpected 'Home' button found: ", err)
			}
		})
	}
}
