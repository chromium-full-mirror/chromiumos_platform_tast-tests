// Copyright 2024 The ChromiumOS Authors
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
	"go.chromium.org/tast-tests/cros/local/mgs"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/policyutil/safesearch"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: DeviceLocalAccountPolicyPropagation,
		Desc: "Test for device local account policy propagation",
		Contacts: []string{
			"chromeos-commercial-remote-management@google.com",
			"vsavu@google.com", // Test owner
		},
		BugComponent: "b:1111617", // ChromeOS > Software > Commercial (Enterprise) > Remote Management > Policy Stack
		SoftwareDeps: []string{"reboot", "chrome"},
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
		},
		Fixture: fixture.FakeDMSEnrolled,
		Timeout: 2 * time.Minute,
		SearchFlags: []*testing.StringPair{
			// ForceGoogleSafeSearch policy is arbitrary, the test uses it to
			// test the device local account policy stack.
			pci.SearchFlag(&policy.ForceGoogleSafeSearch{}, pci.Served),
			pci.SearchFlag(&policy.DeviceLocalAccounts{}, pci.Served),
			pci.SearchFlag(&policy.DeviceLocalAccountAutoLoginId{}, pci.Served),
		},
		Params: []testing.Param{{
			Name: "sha256_enabled",
			Val:  true,
		}, {
			Name: "sha256_disabled",
			Val:  false,
		}},
	})
}

func DeviceLocalAccountPolicyPropagation(ctx context.Context, s *testing.State) {
	sha256Enabled := s.Param().(bool)
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 15*time.Second)
	defer cancel()

	var chromeOpts []chrome.Option
	if sha256Enabled {
		chromeOpts = append(chromeOpts, chrome.EnableFeatures("PolicyFetchWithSha256"))
	} else {
		chromeOpts = append(chromeOpts, chrome.DisableFeatures("PolicyFetchWithSha256"))
	}
	m, cr, err := mgs.New(
		ctx,
		fdms,
		mgs.DefaultAccount(),
		mgs.AutoLaunch(mgs.MgsAccountID),
		mgs.ExtraChromeOptions(chromeOpts...),
	)
	if err != nil {
		s.Fatal("Failed to start Chrome on Signin screen with default MGS account: ", err)
	}
	defer func() {
		if err := m.Close(cleanupCtx); err != nil {
			s.Fatal("Failed close MGS: ", err)
		}
	}()

	// Install initial policy.
	s.Run(ctx, "initial", func(context.Context, *testing.State) {
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

		if err := pb.AddPublicAccountPolicies(mgs.MgsAccountID, []policy.Policy{&policy.ForceGoogleSafeSearch{Val: true}}); err != nil {
			s.Fatal("Failed to add public account ForceGoogleSafeSearch policy: ", err)
		}
		if err := pb.AddPolicies(policies); err != nil {
			s.Fatal("Failed to add policies for public account setup: ", err)
		}
		if err := policyutil.ServeBlobAndRefresh(ctx, fdms, cr, pb); err != nil {
			s.Fatal("Failed to update policies: ", err)
		}
		defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree_")

		// Verify the policy value.
		if err := safesearch.TestGoogleSafeSearch(ctx, cr, true); err != nil {
			s.Error("Failed to verify state of Google safe search: ", err)
		}
	})

	s.Run(ctx, "update", func(context.Context, *testing.State) {
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

		if err := pb.AddPublicAccountPolicies(mgs.MgsAccountID, []policy.Policy{&policy.ForceGoogleSafeSearch{Val: false}}); err != nil {
			s.Fatal("Failed to add public account ForceGoogleSafeSearch policy: ", err)
		}
		if err := pb.AddPolicies(policies); err != nil {
			s.Fatal("Failed to add policies for public account setup: ", err)
		}
		if err := policyutil.ServeBlobAndRefresh(ctx, fdms, cr, pb); err != nil {
			s.Fatal("Failed to update policies: ", err)
		}
		defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree_")

		// Verify the policy value.
		if err := safesearch.TestGoogleSafeSearch(ctx, cr, false); err != nil {
			s.Error("Failed to verify state of Google safe search: ", err)
		}
	})
}
