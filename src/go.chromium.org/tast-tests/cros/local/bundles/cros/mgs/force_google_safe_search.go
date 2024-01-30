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
		Func:         ForceGoogleSafeSearch,
		LacrosStatus: testing.LacrosVariantNeeded,
		Desc:         "Verify behavior of ForceGoogleSafeSearch policy on Managed Guest Session",
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
		// Give each subtest 1 minute to run
		Timeout: 3 * time.Minute,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.ForceGoogleSafeSearch{}, pci.VerifiedFunctionalityJS),
			pci.SearchFlag(&policy.DeviceLocalAccounts{}, pci.VerifiedFunctionalityOS),
			pci.SearchFlag(&policy.DeviceLocalAccountAutoLoginId{}, pci.VerifiedFunctionalityOS),
		},
	})
}

func ForceGoogleSafeSearch(ctx context.Context, s *testing.State) {
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
		name     string
		wantSafe bool
		value    *policy.ForceGoogleSafeSearch
	}{
		{
			name:     "unset",
			wantSafe: false,
			value:    &policy.ForceGoogleSafeSearch{Stat: policy.StatusUnset},
		},
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
				s.Fatal("Failed to add public account ForceGoogleSafeSearch policy: ", err)
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
			if err := safesearch.TestGoogleSafeSearch(ctx, br, param.wantSafe); err != nil {
				s.Error("Failed to verify state of Google safe search: ", err)
			}
		})
	}
}
