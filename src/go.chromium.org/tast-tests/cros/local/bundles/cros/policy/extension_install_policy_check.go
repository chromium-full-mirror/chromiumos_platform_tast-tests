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
	"go.chromium.org/tast-tests/cros/local/annotations"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/policy/extensioninstall"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ExtensionInstallPolicyCheck,
		LacrosStatus: testing.LacrosVariantNeeded,
		Desc:         "Checks the behavior of ExtensionInstallAllowlist, ExtensionInstallBlocklist policies",
		Contacts: []string{
			"cros-commercial-chromeapps-eng@google.com",
			"dp-chromeos-eng@google.com",
			"giovax@google.com",
		},
		BugComponent: "b:1253865",
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:golden_tier"},
		Fixture:      fixture.ChromePolicyLoggedInDevToolsAvailable,
		Timeout:      4 * time.Minute, // There is a longer wait when installing the extension.
		Params: []testing.Param{
			{
				Name: "blocklist_wildcard",
				Val: []extensioninstall.TestCase{
					{
						Name:         "allowlist_set",
						AllowInstall: true,
						Policies: []policy.Policy{
							// Test API extension should be specified in allow list, otherwise it would get disabled automatically.
							&policy.ExtensionInstallAllowlist{Val: []string{extensioninstall.ExtensionID, chrome.TestExtensionID}},
							&policy.ExtensionInstallBlocklist{Val: []string{"*"}},
						},
						ShouldFindAnnotation: false,
					},
					{
						Name:         "allowlist_set_with_test_api_extension",
						AllowInstall: false,
						Policies: []policy.Policy{
							&policy.ExtensionInstallAllowlist{Val: []string{chrome.TestExtensionID}},
							&policy.ExtensionInstallBlocklist{Val: []string{"*"}},
						},
						ShouldFindAnnotation: false,
					},
				},
			},
			{
				Name: "blocklist_unset",
				Val: []extensioninstall.TestCase{
					{
						Name:         "allowlist_set",
						AllowInstall: true,
						Policies: []policy.Policy{
							&policy.ExtensionInstallAllowlist{Val: []string{extensioninstall.ExtensionID}},
						},
						ShouldFindAnnotation: false,
					},
					{
						Name:                 "allowlist_unset",
						AllowInstall:         true,
						Policies:             []policy.Policy{},
						ShouldFindAnnotation: false,
					},
				},
			},
			{
				Name: "blocklist_set",
				Val: []extensioninstall.TestCase{
					{
						Name:         "allowlist_set",
						AllowInstall: false,
						Policies: []policy.Policy{
							&policy.ExtensionInstallAllowlist{Val: []string{extensioninstall.ExtensionID}},
							&policy.ExtensionInstallBlocklist{Val: []string{extensioninstall.ExtensionID}},
						},
						ShouldFindAnnotation: false,
					},
					{
						Name:         "allowlist_unset",
						AllowInstall: false,
						Policies: []policy.Policy{
							&policy.ExtensionInstallBlocklist{Val: []string{extensioninstall.ExtensionID}},
						},
						ShouldFindAnnotation: false,
					},
				},
			},
		},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.ExtensionInstallBlocklist{}, pci.VerifiedFunctionalityUI),
			pci.SearchFlag(&policy.ExtensionInstallAllowlist{}, pci.VerifiedFunctionalityUI),
		},
	})
}

func ExtensionInstallPolicyCheck(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	// Connect to Test API to use it with the UI library.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	tcs, ok := s.Param().([]extensioninstall.TestCase)
	if !ok {
		s.Fatal("Failed to convert test cases to the desired type")
	}

	for _, tc := range tcs {
		s.Run(ctx, tc.Name, func(ctx context.Context, s *testing.State) {
			defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree_"+tc.Name)

			// Perform cleanup.
			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Fatal("Failed to clean up: ", err)
			}

			// Update policies.
			if err := policyutil.ServeAndVerify(ctx, fdms, cr, tc.Policies); err != nil {
				s.Fatal("Failed to update policies: ", err)
			}

			// Open the net-export page and start logging.
			if err := annotations.StartLogging(ctx, cr, cr.Browser()); err != nil {
				s.Fatal("Failed to start logging: ", err)
			}

			// Run actual test.
			if err := extensioninstall.TriggerExtensionInstall(ctx, tc, tconn, cr.Browser()); err != nil {
				s.Fatal("Test case failed: ", err)
			}

			// Stop logging and check the logs to verify if all/none annotations
			// are found.
			foundAnnotation, err := annotations.StopLoggingVerifyAnnotationSet(ctx, cr, cr.Browser(), tc.ShouldFindAnnotation, extensioninstall.GetAnnotations())
			if err != nil {
				s.Fatal("Unexpected error when verifying logs: Got: ", foundAnnotation, " Want:", tc.ShouldFindAnnotation, " Error:", err)
			}
			if foundAnnotation != tc.ShouldFindAnnotation {
				s.Fatal("Unexpected outcome when verifying logs: Got: ", foundAnnotation, " Want:", tc.ShouldFindAnnotation, " Error:", err)
			}
		})
	}
}
