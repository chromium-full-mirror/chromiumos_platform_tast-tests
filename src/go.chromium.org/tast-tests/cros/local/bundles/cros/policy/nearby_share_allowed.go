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
	"go.chromium.org/tast-tests/cros/local/bundles/cros/policy/nearbyshare"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/policy/networkrequestmonitor"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         NearbyShareAllowed,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test NearbyShareAllowed policy",
		Contacts: []string{
			"dp-chromeos-eng@google.com",
			"chiav@google.com",
		},
		BugComponent: "b:1129862",
		Attr: []string{
			"group:golden_tier",
			"group:mainline",
			"informational",
			"group:hw_agnostic",
		},
		SoftwareDeps: []string{"chrome"},
		Fixture:      fixture.ChromePolicyRealUserLoggedIn,
		Timeout:      3 * time.Minute,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.NearbyShareAllowed{}, pci.VerifiedFunctionalityUI),
		},
	})
}

// NearbyShareAllowed tests the NearbyShareAllowed policy by verifying that the
// Nearby Share OS setting shows correctly based on policy value.
//   - When enabled: A 'Set up' button should be displayed
//   - When disabled: A on/off toggle should be set to off, and be disabled
func NearbyShareAllowed(ctx context.Context, s *testing.State) {
	// Shorten the context to make room for cleanup jobs.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree")

	// Test the 'enabled' and 'disabled' cases only, since these are the valid values in DPanel.
	// No test for 'unset' since this is not a valid value for this policy.
	for key, param := range nearbyshare.TestCases() {
		s.Run(ctx, param.Name, func(ctx context.Context, s *testing.State) {
			// Perform cleanup.
			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Fatal("Failed to clean up: ", err)
			}

			// Update policies.
			if err := policyutil.ServeAndVerify(ctx, fdms, cr, []policy.Policy{param.Policy}); err != nil {
				s.Fatal("Failed to update policies: ", err)
			}

			if err := nearbyshare.VerifyNearbySharePermissions(ctx,
				networkrequestmonitor.OptionalServiceParams{
					Chrome:        cr,
					PolicySetting: key}); err != nil {
				s.Fatal("Failed to verify Nearby Share permissions: ", err)
			}
		})
	}
}
