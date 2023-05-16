// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"time"

	"chromiumos/tast/common/chrome/credconfig"
	"chromiumos/tast/common/fixture"
	"chromiumos/tast/common/pci"
	"chromiumos/tast/common/policy"
	"chromiumos/tast/common/policy/fakedms"
	"chromiumos/tast/local/annotations"
	"chromiumos/tast/local/bundles/cros/policy/calendarintegration"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/browser/browserfixt"
	"chromiumos/tast/local/chrome/lacros/lacrosfixt"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/policyutil"

	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CalendarIntegrationEnabled,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks behavior of CalendarIntegrationEnabled policy, check if event list is shown based on value of the policy",
		BugComponent: "b:1129862",
		Contacts: []string{
			"dp-chromeos-eng@google.com",
			"crmullins@google.com",
		},
		Attr:         []string{"group:commercial_limited"},
		SoftwareDeps: []string{"chrome"},
		Vars:         []string{"policy.managedUserAccountPool"},
		Timeout:      3 * time.Minute,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.CalendarIntegrationEnabled{}, pci.VerifiedFunctionalityUI),
			pci.SearchFlag(&policy.CalendarIntegrationEnabled{}, pci.VerifiedFunctionalityOS),
		},
		Params: []testing.Param{{
			Val:     browser.TypeAsh,
			Fixture: fixture.FakeDMS,
		}, {
			Name:              "lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Fixture:           fixture.PersistentLacros, // FakeDMS with lacros policy
			Val:               browser.TypeLacros,
		}},
	})
}

func CalendarIntegrationEnabled(ctx context.Context, s *testing.State) {
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	gaiaCreds, err := credconfig.PickRandomCreds(
		s.RequiredVar("policy.managedUserAccountPool"))
	if err != nil {
		s.Fatal("Failed to parse managed user creds: ", err)
	}

	policyBlob := policy.NewBlob()
	policyBlob.PolicyUser = gaiaCreds.User
	if err := fdms.WritePolicyBlob(policyBlob); err != nil {
		s.Fatal("Failed to write policies to FakeDMS: ", err)
	}

	opts := []chrome.Option{
		chrome.DMSPolicy(fdms.URL),  // FakeDMS for setting policies
		chrome.GAIALogin(gaiaCreds), // Real GAIA to enable calendar_get_events call
	}
	if isLacros(s) {
		opts, err = lacrosfixt.NewConfig(lacrosfixt.ChromeOptions(opts...)).Opts()
		if err != nil {
			s.Fatal("Failed to compute lacros chrome options: ", err)
		}
	}

	cr, err := chrome.New(ctx, opts...)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(ctx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}
	defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)

	for _, param := range []calendarintegration.TestCase{
		{
			Name:                    "unset",
			ShouldFindEventListView: true,
			ShouldFindManagedIcon:   false,
			ShouldFindAnnotation:    true,
			Policy:                  &policy.CalendarIntegrationEnabled{Stat: policy.StatusUnset},
		},
		{
			Name:                    "enabled",
			ShouldFindEventListView: true,
			ShouldFindManagedIcon:   false,
			ShouldFindAnnotation:    true,
			Policy:                  &policy.CalendarIntegrationEnabled{Val: true},
		},
		{
			Name:                    "disabled",
			ShouldFindEventListView: false,
			ShouldFindManagedIcon:   true,
			ShouldFindAnnotation:    false,
			Policy:                  &policy.CalendarIntegrationEnabled{Val: false},
		},
	} {
		s.Run(ctx, param.Name, func(ctx context.Context, s *testing.State) {
			// Perform cleanup.
			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Fatal("Failed to clean up: ", err)
			}

			// Update policies.
			policies := []policy.Policy{param.Policy}
			policyBlob := policy.NewBlob()
			policyBlob.PolicyUser = gaiaCreds.User
			policyBlob.AddPolicies(policies)
			if err := policyutil.ServeBlobAndRefresh(ctx, fdms, cr, policyBlob); err != nil {
				s.Fatal("Failed to update policies: ", err)
			}
			if err := policyutil.Verify(ctx, tconn, policies); err != nil {
				s.Fatal("Failed to verify updated policies: ", err)
			}

			// Setup browser based on the chrome type.
			br, closeBrowser, err := browserfixt.SetUp(ctx, cr, s.Param().(browser.Type))
			if err != nil {
				s.Fatal("Failed to open the browser: ", err)
			}
			defer closeBrowser(ctx)

			// Open the net-export page and start logging. Skip for lacros until we
			// fix b/278750986.
			if !isLacros(s) {
				if err := annotations.StartLogging(ctx, cr, br); err != nil {
					s.Fatal("Failed to start logging: ", err)
				}
			}

			if err := calendarintegration.TriggerCalendarIntegration(ctx, param, br, tconn, s); err != nil {
				s.Fatal("Failed to trigger and verify calendar integration: ", err)
			}

			// Check for annotation. Skip for lacros until we fix b/278750986.
			if !isLacros(s) {
				didFindAnnotation := false
				// Stop logging and check the logs for calendar_get_events
				// NetworkTrafficAnnotationTag with hash 86429515.
				if foundAnnotation, err := annotations.StopLoggingCheckLogs(ctx, cr, br, "86429515"); err != nil {
					s.Fatal("Failed to stop logging and check logs: ", err)
				} else if foundAnnotation == true {
					didFindAnnotation = true
				}

				if param.ShouldFindAnnotation && didFindAnnotation == false {
					s.Fatal("Did not find expected NetworkTrafficAnnotationTag with id calendar_get_events")
				}

				if !param.ShouldFindAnnotation && didFindAnnotation == true {
					s.Fatal("Found unexpected NetworkTrafficAnnotationTag with id calendar_get_events")
				}
			}
		})
	}
}

func isLacros(s *testing.State) bool {
	return s.Param().(browser.Type) == browser.TypeLacros
}
