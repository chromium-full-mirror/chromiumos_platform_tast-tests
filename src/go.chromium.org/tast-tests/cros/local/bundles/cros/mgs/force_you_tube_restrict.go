// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package mgs

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/mgs"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/policyutil/safesearch"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ForceYouTubeRestrict,
		LacrosStatus: testing.LacrosVariantNeeded,
		Desc:         "Verify behavior of ForceYouTubeRestrict policy on Managed Guest Session",
		Contacts: []string{
			"cros-edu-eng@google.com",
		},
		BugComponent: "b:1363915", // ChromeOS Server Projects > Enterprise Management > Edu Features > Tast tests
		SoftwareDeps: []string{"reboot", "chrome"},
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
		},
		Fixture: fixture.FakeDMSEnrolled,
		// Loading two YouTube videos on slower devices can take a while (we observed subtests that took up to 40 seconds), thus give every subtest 1 minute to run.
		Timeout: 4 * time.Minute,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.ForceYouTubeRestrict{}, pci.VerifiedFunctionalityJS),
			pci.SearchFlag(&policy.DeviceLocalAccounts{}, pci.VerifiedFunctionalityOS),
			pci.SearchFlag(&policy.DeviceLocalAccountAutoLoginId{}, pci.VerifiedFunctionalityOS),
		},
	})
}

// ForceYouTubeRestrict tests the behavior of the ForceYouTubeRestrict Enterprise policy.
func ForceYouTubeRestrict(ctx context.Context, s *testing.State) {
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()
	// Launch a new MGS with default account.
	m, cr, err := mgs.New(
		ctx,
		fdms,
		mgs.DefaultAccount(),
		mgs.AutoLaunch(mgs.MgsAccountID),
	)
	if err != nil {
		s.Fatal("Failed to start Chrome on Signin screen with default MGS account: ", err)
	}
	defer func() {
		if err := m.Close(ctx); err != nil {
			s.Fatal("Failed close MGS: ", err)
		}
	}()
	for _, param := range []struct {
		// name is the subtest name.
		name string
		// value is the policy value.
		value *policy.ForceYouTubeRestrict
		// stringContentRestricted is whether strong content is expected to be restricted.
		strongContentRestricted bool
		// mildContentRestricted is whether mild content is expected to be restricted.
		mildContentRestricted bool
	}{
		{
			name:                    "disabled",
			value:                   &policy.ForceYouTubeRestrict{Val: safesearch.ForceYouTubeRestrictDisabled},
			strongContentRestricted: false,
			mildContentRestricted:   false,
		},
		{
			name:                    "moderate",
			value:                   &policy.ForceYouTubeRestrict{Val: safesearch.ForceYouTubeRestrictModerate},
			strongContentRestricted: true,
			mildContentRestricted:   false,
		},
		{
			name:                    "strict",
			value:                   &policy.ForceYouTubeRestrict{Val: safesearch.ForceYouTubeRestrictStrict},
			strongContentRestricted: true,
			mildContentRestricted:   true,
		},
		{
			name:                    "unset",
			value:                   &policy.ForceYouTubeRestrict{Stat: policy.StatusUnset},
			strongContentRestricted: false,
			mildContentRestricted:   false,
		},
	} {
		s.Run(ctx, param.name, func(ctx context.Context, s *testing.State) {
			mgsAccountPolicy := policy.DeviceLocalAccountInfo{
				AccountID:   &mgs.MgsAccountID,
				AccountType: &mgs.AccountType,
			}
			policies := []policy.Policy{
				&policy.DeviceLocalAccounts{
					Val: []policy.DeviceLocalAccountInfo{
						mgsAccountPolicy,
					},
				},
				&policy.DeviceLocalAccountAutoLoginId{
					Val: mgs.MgsAccountID,
				},
			}
			pb := policy.NewBlob()
			if err := pb.AddPublicAccountPolicies(mgs.MgsAccountID, []policy.Policy{param.value}); err != nil {
				s.Fatal("Failed to add public account ForceYouTubeRestrict policy: ", err)
			}
			if err := pb.AddPolicies(policies); err != nil {
				s.Fatal("Failed to add policies for public account setup: ", err)
			}
			if err := policyutil.ServeBlobAndRefresh(ctx, fdms, cr, pb); err != nil {
				s.Fatal("Failed to update policies: ", err)
			}
			defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree_")
			br := cr.Browser()

			// Run actual test.
			if err := safesearch.TestYouTubeRestrictedMode(ctx, br, param.strongContentRestricted, param.mildContentRestricted); err != nil {
				s.Error("Failed to verify YouTube content restriction: ", err)
			}
		})
	}
}
