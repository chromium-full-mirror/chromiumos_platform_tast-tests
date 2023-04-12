// Copyright 2021 The ChromiumOS Authors
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
	"chromiumos/tast/errors"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/policyutil"
	"chromiumos/tast/local/strcmp"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PrintersBulkConfiguration,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify behavior of printer bulk configuration user policies",
		Contacts: []string{
			"chromeos-commercial-printing@google.com",
		},
		BugComponent: "b:1111614", // ChromeOS > Software > Commercial (Enterprise) > Printing
		SoftwareDeps: []string{"chrome"},
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
		},
		Fixture: fixture.ChromePolicyLoggedIn,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.PrintersBulkAccessMode{}, pci.VerifiedFunctionalityJS),
			pci.SearchFlag(&policy.PrintersBulkAllowlist{}, pci.VerifiedFunctionalityJS),
			pci.SearchFlag(&policy.PrintersBulkBlocklist{}, pci.VerifiedFunctionalityJS),
			pci.SearchFlag(&policy.PrintersBulkConfiguration{}, pci.VerifiedFunctionalityJS),
			{
				Key: "feature_id",
				// Test printer configuration user policies (COM_FOUND_CUJ7_TASK3_WF1).
				Value: "screenplay-87696fca-4b8c-410d-a5f7-b2b5f1391eb3",
			},
		},
	})
}

func PrintersBulkConfiguration(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	// Connect to Test API to use it with the UI library.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	// All the common policies that define the printers configuration, allowlist and blocklist.
	// DevicePrinters configures 4 printers with the following names: "wl", "bl", "both", "other".
	commonPolicies := []policy.Policy{
		&policy.PrintersBulkAllowlist{Val: []string{"both", "wl"}},
		&policy.PrintersBulkBlocklist{Val: []string{"both", "bl"}},
		&policy.PrintersBulkConfiguration{Val: &policy.PrintersBulkConfigurationValue{
			Url:  "https://storage.googleapis.com/chromiumos-test-assets-public/enterprise/printers.json",
			Hash: "7a052c5e4f23c159668148df2a3c202bed4d65749cab5ecd0fa7db211c12a3b8",
		}},
	}

	for _, param := range []struct {
		name        string
		expectedIDs []string // expectedIDs is the expected list of ids for each configuration.
		policies    []policy.Policy
	}{
		{
			name:        "all_except_blocklist",
			expectedIDs: []string{"wl", "other"},
			policies: append(
				commonPolicies,
				&policy.PrintersBulkAccessMode{Val: 0},
			),
		},
		{
			name:        "allowlist",
			expectedIDs: []string{"wl", "both"},
			policies: append(
				commonPolicies,
				&policy.PrintersBulkAccessMode{Val: 1},
			),
		},
		{
			name:        "all",
			expectedIDs: []string{"bl", "wl", "other", "both"},
			policies: append(
				commonPolicies,
				&policy.PrintersBulkAccessMode{Val: 2},
			),
		},
		{
			name:        "unset",
			expectedIDs: []string{"bl", "wl", "other", "both"},
			policies: append(
				commonPolicies,
				&policy.PrintersBulkAccessMode{Stat: policy.StatusUnset},
			),
		},
	} {
		s.Run(ctx, param.name, func(ctx context.Context, s *testing.State) {
			// Reserve ten seconds for cleanup.
			cleanupCtx := ctx
			ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
			defer cancel()

			defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree_"+param.name)

			// Perform cleanup.
			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Fatal("Failed to clean up: ", err)
			}

			// Update policies.
			if err := policyutil.ServeAndVerify(ctx, fdms, cr, param.policies); err != nil {
				s.Fatal("Failed to update policies: ", err)
			}

			// Retrieve Printers seen by user.
			printers := make([]map[string]string, 0)
			if err := tconn.Call(ctx, &printers, `tast.promisify(chrome.autotestPrivate.getPrinterList)`); err != nil {
				s.Fatal("Failed to evaluate JS expression and get printers: ", err)
			}

			// Get Printers IDs.
			foundIDs := make(map[string]bool)
			ids := make([]string, 0)
			for _, printer := range printers {
				if id, ok := printer["printerId"]; ok {
					foundIDs[id] = true
					ids = append(ids, id)
				} else {
					s.Fatal("Missing printerId field")
				}
			}
			if len(foundIDs) < len(printers) {
				s.Fatal("Received response contains duplicates")
			}

			if diff := strcmp.SameList(param.expectedIDs, ids); diff != "" {
				s.Error(errors.Errorf("unexpected IDs (-want +got): %v", diff))
			}
		})
	}
}
