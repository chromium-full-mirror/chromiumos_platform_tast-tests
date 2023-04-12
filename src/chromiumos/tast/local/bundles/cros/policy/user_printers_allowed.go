// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"

	"chromiumos/tast/common/fixture"
	"chromiumos/tast/common/pci"
	"chromiumos/tast/common/policy"
	"chromiumos/tast/common/policy/fakedms"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/restriction"
	"chromiumos/tast/local/chrome/uiauto/role"
	"chromiumos/tast/local/policyutil"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         UserPrintersAllowed,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test behavior of UserPrintersAllowed policy: check if Add printer button is restricted based on the value of the policy",
		Contacts: []string{
			"chromeos-commercial-printing@google.com",
			"project-bolton@google.com",
			"alexanderhartl@google.com", // Test author
		},
		// ChromeOS > Software > Commercial (Enterprise) > Printing
		BugComponent: "b:1111614",
		SoftwareDeps: []string{"chrome"},
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
			"group:paper-io",
			"paper-io_printing",
		},
		Fixture: fixture.ChromePolicyLoggedIn,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.UserPrintersAllowed{}, pci.VerifiedFunctionalityUI),
			{
				Key: "feature_id",
				// Check that UserPrintersAllowed policy controls the ability to add a printer (COM_FOUND_CUJ8_TASK3_WF1).
				Value: "screenplay-82497314-906c-4db1-9d41-16d11d411050",
			},
		},
	})
}

func UserPrintersAllowed(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	for _, param := range []struct {
		name            string
		wantRestriction restriction.Restriction     // wantRestricted is the expected restriction state of the "Add printer" button.
		policy          *policy.UserPrintersAllowed // policy is the policy we test.
	}{
		{
			name:            "unset",
			wantRestriction: restriction.None,
			policy:          &policy.UserPrintersAllowed{Stat: policy.StatusUnset},
		},
		{
			name:            "not_allowed",
			wantRestriction: restriction.Disabled,
			policy:          &policy.UserPrintersAllowed{Val: false},
		},
		{
			name:            "allowed",
			wantRestriction: restriction.None,
			policy:          &policy.UserPrintersAllowed{Val: true},
		},
	} {
		s.Run(ctx, param.name, func(ctx context.Context, s *testing.State) {
			defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree_"+param.name)

			// Perform cleanup.
			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Fatal("Failed to clean up: ", err)
			}

			// Update policies.
			if err := policyutil.ServeAndVerify(ctx, fdms, cr, []policy.Policy{param.policy}); err != nil {
				s.Fatal("Failed to update policies: ", err)
			}

			// Check if the Add printer button is restricted.
			if err := policyutil.OSSettingsPage(ctx, cr, "cupsPrinters").
				SelectNode(ctx, nodewith.
					Name("Add printer").
					Role(role.Button)).
				Restriction(param.wantRestriction).
				Verify(); err != nil {
				s.Error("Unexpected OS settings state: ", err)
			}
		})
	}
}
