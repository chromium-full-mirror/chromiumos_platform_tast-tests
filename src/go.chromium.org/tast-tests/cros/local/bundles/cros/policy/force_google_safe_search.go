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
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/policyutil/safesearch"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: ForceGoogleSafeSearch,
		Desc: "Test the behavior of ForceGoogleSafeSearch policy: check if Google safe search is enabled based on the value of the policy",
		Contacts: []string{
			"cros-edu-eng@google.com",
			"snijhara@google.com", // Test author
		},
		BugComponent: "b:1363915", // ChromeOS Server Projects > Enterprise Management > Edu Features > Tast tests
		SoftwareDeps: []string{"chrome"},
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
			"group:hw_agnostic"},
		Fixture: fixture.ChromePolicyLoggedIn,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.ForceGoogleSafeSearch{}, pci.VerifiedFunctionalityJS),
		},
	})
}

func ForceGoogleSafeSearch(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	// Reserve ten seconds for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	for _, param := range []struct {
		name     string
		wantSafe bool
		value    *policy.ForceGoogleSafeSearch
	}{
		{
			name:     "enabled",
			wantSafe: true,
			value:    &policy.ForceGoogleSafeSearch{Val: true},
		},
		{
			name:     "disabled",
			wantSafe: false,
			value:    &policy.ForceGoogleSafeSearch{Val: false},
		},
		{
			name:     "unset",
			wantSafe: false,
			value:    &policy.ForceGoogleSafeSearch{Stat: policy.StatusUnset},
		},
	} {
		s.Run(ctx, param.name, func(ctx context.Context, s *testing.State) {
			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Fatal("Failed to clean up: ", err)
			}

			if err := policyutil.ServeAndVerify(ctx, fdms, cr, []policy.Policy{param.value}); err != nil {
				s.Fatal("Failed to update policies: ", err)
			}

			br, closeBrowser, err := browserfixt.SetUp(ctx, cr, browser.TypeAsh)
			if err != nil {
				s.Fatal("Failed to setup chrome: ", err)
			}
			defer closeBrowser(cleanupCtx)

			defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree_"+param.name)

			if err := safesearch.TestGoogleSafeSearch(ctx, br, param.wantSafe); err != nil {
				s.Error("Failed to verify state of Google safe search: ", err)
			}
		})
	}
}
