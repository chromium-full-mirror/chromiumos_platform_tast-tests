// Copyright 2022 The ChromiumOS Authors
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
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/policyutil/safesearch"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: ForceSafeSearch,
		Desc: "Test the behavior of deprecated ForceSafeSearch policy: check if Google and YouTube safe search is enabled based on the value of the policy",
		Contacts: []string{
			"cros-engprod-muc@google.com",
		},
		BugComponent: "b:1263917",
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:golden_tier", "group:hw_agnostic"},
		Fixture:      fixture.ChromePolicyLoggedIn,
		// Loading two YouTube videos on slower devices can take a while (we observed subtests that took up to 40 seconds), thus give every subtest 1 minute to run.
		Timeout: 9 * time.Minute,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.ForceYouTubeRestrict{}, pci.VerifiedFunctionalityJS),
			pci.SearchFlag(&policy.ForceSafeSearch{}, pci.VerifiedFunctionalityJS),
			pci.SearchFlag(&policy.ForceYouTubeSafetyMode{}, pci.VerifiedFunctionalityJS),
			pci.SearchFlag(&policy.ForceGoogleSafeSearch{}, pci.VerifiedFunctionalityJS),
		},
	})
}

func ForceSafeSearch(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	// Reserve ten seconds for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	for _, param := range []struct {
		name            string
		wantGoogleSafe  bool
		wantYouTubeSafe bool
		value           []policy.Policy
	}{
		{
			name:            "enabled",
			wantGoogleSafe:  true,
			wantYouTubeSafe: true,
			value:           []policy.Policy{&policy.ForceSafeSearch{Val: true}},
		},
		{
			name: "enabled_overwritten_by_ForceGoogleSafeSearch",
			// ForceSafeSearch is ignored entirely if ForceGoogleSafeSearch is set.
			wantGoogleSafe:  false,
			wantYouTubeSafe: false,
			value: []policy.Policy{
				&policy.ForceSafeSearch{Val: true},
				&policy.ForceGoogleSafeSearch{Val: false}},
		},
		{
			name: "enabled_overwritten_by_ForceYouTubeSafetyMode",
			// ForceSafeSearch is ignored entirely if ForceYouTubeSafetyMode is set.
			wantGoogleSafe:  false,
			wantYouTubeSafe: false,
			value: []policy.Policy{
				&policy.ForceSafeSearch{Val: true},
				&policy.ForceYouTubeSafetyMode{Val: false}},
		},
		{
			name: "enabled_overwritten_by_ForceYouTubeRestrict",
			// ForceSafeSearch is ignored entirely if ForceYouTubeRestrict is set.
			wantGoogleSafe:  false,
			wantYouTubeSafe: false,
			value: []policy.Policy{
				&policy.ForceSafeSearch{Val: true},
				&policy.ForceYouTubeRestrict{Val: safesearch.ForceYouTubeRestrictDisabled}},
		},
		{
			name:            "disabled",
			wantGoogleSafe:  false,
			wantYouTubeSafe: false,
			value:           []policy.Policy{&policy.ForceSafeSearch{Val: false}},
		},
		{
			name:            "disabled_overwritten_by_ForceGoogleSafeSearch",
			wantGoogleSafe:  true,
			wantYouTubeSafe: false,
			value: []policy.Policy{
				&policy.ForceSafeSearch{Val: false},
				&policy.ForceGoogleSafeSearch{Val: true}},
		},
		{
			name:            "disabled_overwritten_by_ForceYouTubeSafetyMode",
			wantGoogleSafe:  false,
			wantYouTubeSafe: true,
			value: []policy.Policy{
				&policy.ForceSafeSearch{Val: false},
				&policy.ForceYouTubeSafetyMode{Val: true}},
		},
		{
			name:            "disabled_overwritten_by_ForceYouTubeRestrict",
			wantGoogleSafe:  false,
			wantYouTubeSafe: true,
			value: []policy.Policy{
				&policy.ForceSafeSearch{Val: false},
				&policy.ForceYouTubeRestrict{Val: safesearch.ForceYouTubeRestrictModerate}},
		},
		{
			name:            "unset",
			wantGoogleSafe:  false,
			wantYouTubeSafe: false,
			value:           []policy.Policy{&policy.ForceSafeSearch{Stat: policy.StatusUnset}},
		},
	} {
		s.Run(ctx, param.name, func(ctx context.Context, s *testing.State) {
			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Fatal("Failed to clean up: ", err)
			}

			if err := policyutil.ServeAndVerify(ctx, fdms, cr, param.value); err != nil {
				s.Fatal("Failed to update policies: ", err)
			}

			defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree_"+param.name)

			if err := safesearch.TestGoogleSafeSearch(ctx, cr, param.wantGoogleSafe); err != nil {
				s.Error("Failed to verify state of Google safe search: ", err)
			}

			if err := safesearch.TestYouTubeRestrictedMode(ctx, cr, param.wantYouTubeSafe, false); err != nil {
				s.Error("Failed to verify YouTube content restriction: ", err)
			}
		})
	}
}
