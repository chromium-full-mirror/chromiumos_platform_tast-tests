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
	"go.chromium.org/tast-tests/cros/local/bundles/cros/policy/developertools"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DeveloperToolsAvailability,
		LacrosStatus: testing.LacrosVariantExists,
		// TODO(crbug/1125548): add functionality to verify policy with
		// force installed extension.
		Desc: "Behavior of the DeveloperToolsAvailability policy, check whether developer tools can be opened on chrome://user-actions page",
		Contacts: []string{
			"cros-engprod-muc@google.com",
			"pmarko@google.com", // Test author
		},
		SoftwareDeps: []string{"chrome"},
		BugComponent: "b:1263917", // ChromeOS > Software > Commercial (Enterprise) > Testing
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
		// This test
		// - starts 16 subtests, each of which can realistically take up to 20 seconds and
		// - reserves 10 seconds for cleanup.
		// -> at least 5.5min should be allocated as timeout.
		Timeout: 6 * time.Minute,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DeveloperToolsAvailability{}, pci.VerifiedFunctionalityUI),
		},
	})
}

func DeveloperToolsAvailability(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	// Reserve ten seconds for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Connect to Test API.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	for _, tc := range []developertools.TestCase{
		{
			Name:        "disallowed_for_force_installed_extensions",
			Value:       &policy.DeveloperToolsAvailability{Val: 0},
			WantAllowed: true,
		},
		{
			Name:        "allowed",
			Value:       &policy.DeveloperToolsAvailability{Val: 1},
			WantAllowed: true,
		},
		{
			Name:        "disallowed",
			Value:       &policy.DeveloperToolsAvailability{Val: 2},
			WantAllowed: false,
		},
		{
			Name:        "unset",
			Value:       &policy.DeveloperToolsAvailability{Stat: policy.StatusUnset},
			WantAllowed: true,
		},
	} {
		s.Run(ctx, tc.Name, func(ctx context.Context, s *testing.State) {
			// Perform cleanup.
			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Fatal("Failed to clean up: ", err)
			}

			// Update policies.
			if err := policyutil.ServeAndVerify(ctx, fdms, cr, []policy.Policy{tc.Value}); err != nil {
				s.Fatal("Failed to update policies: ", err)
			}

			_, closeBrowser, err := browserfixt.SetUp(ctx, cr, s.Param().(browser.Type))
			if err != nil {
				s.Fatal("Failed to setup chrome: ", err)
			}
			defer closeBrowser(cleanupCtx)

			if err := developertools.TriggerDeveloperToolsAvailability(ctx, tc, tconn, s); err != nil {
				s.Fatal("Failed to trigger and verify developer tools availability: ", err)
			}
		})
	}
}
