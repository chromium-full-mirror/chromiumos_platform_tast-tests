// Copyright 2023 The ChromiumOS Authors
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
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/network/ping"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/strcmp"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DevicePrinters,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify behavior of printer configuration device policies",
		Contacts: []string{
			"chromeos-commercial-printing@google.com",
		},
		BugComponent: "b:1111614", // ChromeOS > Software > Commercial (Enterprise) > Printing
		SoftwareDeps: []string{"reboot", "chrome"},
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
		},
		Fixture: fixture.ChromeEnrolledLoggedIn,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DevicePrintersAccessMode{}, pci.VerifiedFunctionalityJS),
			pci.SearchFlag(&policy.DevicePrintersAllowlist{}, pci.VerifiedFunctionalityJS),
			pci.SearchFlag(&policy.DevicePrintersBlocklist{}, pci.VerifiedFunctionalityJS),
			pci.SearchFlag(&policy.DevicePrinters{}, pci.VerifiedFunctionalityJS),
			pci.SearchFlag(&policy.UserPrintersAllowed{}, pci.VerifiedFunctionalityJS),
			{
				Key: "feature_id",
				// Test printer configuration device policies (COM_FOUND_CUJ7_TASK3_WF1).
				Value: "screenplay-87696fca-4b8c-410d-a5f7-b2b5f1391eb3",
			},
		},
	})
}

func DevicePrinters(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	// Connect to Test API to use it with the UI library.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	// Internet connectivity needed because DevicePrinters is an external data policy.
	if err := ping.VerifyInternetConnectivity(ctx, 10*time.Second); err != nil {
		s.Fatal("Cannot fetch external data policy, no internet connectivity: ", err)
	}

	// All the common policies that define the printers configuration, allowlist and blocklist.
	// DevicePrinters configures 4 printers with the following names: "wl", "bl", "both", "other".
	commonPolicies := []policy.Policy{
		&policy.DevicePrintersAllowlist{Val: []string{"both", "wl"}},
		&policy.DevicePrintersBlocklist{Val: []string{"both", "bl"}},
		&policy.DevicePrinters{Val: &policy.DevicePrintersValue{
			Url:  "https://storage.googleapis.com/chromiumos-test-assets-public/enterprise/printers.json",
			Hash: "7a052c5e4f23c159668148df2a3c202bed4d65749cab5ecd0fa7db211c12a3b8",
		}},
		// UserPrintersAllowed is set to mitigate against finding other printers in the lab network (see b/284300892).
		&policy.UserPrintersAllowed{Val: false},
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
				&policy.DevicePrintersAccessMode{Val: 0},
			),
		},
		{
			name:        "allowlist",
			expectedIDs: []string{"wl", "both"},
			policies: append(
				commonPolicies,
				&policy.DevicePrintersAccessMode{Val: 1},
			),
		},
		{
			name:        "all",
			expectedIDs: []string{"bl", "wl", "other", "both"},
			policies: append(
				commonPolicies,
				&policy.DevicePrintersAccessMode{Val: 2},
			),
		},
		{
			name:        "unset",
			expectedIDs: []string{"bl", "wl", "other", "both"},
			policies: append(
				commonPolicies,
				&policy.DevicePrintersAccessMode{Stat: policy.StatusUnset},
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
