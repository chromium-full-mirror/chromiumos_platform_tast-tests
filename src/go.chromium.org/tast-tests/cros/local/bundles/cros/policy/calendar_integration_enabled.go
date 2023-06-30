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
	"go.chromium.org/tast-tests/cros/local/annotations"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/policy/calendarintegration"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/ctxutil"
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
		Attr:         []string{"group:golden_tier"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      3 * time.Minute,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.CalendarIntegrationEnabled{}, pci.VerifiedFunctionalityUI),
			pci.SearchFlag(&policy.CalendarIntegrationEnabled{}, pci.VerifiedFunctionalityOS),
		},
		Params: []testing.Param{{
			Val:     browser.TypeAsh,
			Fixture: fixture.ChromePolicyRealUserLoggedIn,
		}, {
			Name:              "lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Fixture:           fixture.LacrosPolicyRealUserLoggedIn,
			Val:               browser.TypeLacros,
		}},
	})
}

func CalendarIntegrationEnabled(ctx context.Context, s *testing.State) {
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Set up keyboard.
	kb, err := input.VirtualKeyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer kb.Close(cleanupCtx)

	for index, param := range calendarintegration.GetTestCases() {
		s.Run(ctx, param.Name, func(ctx context.Context, s *testing.State) {
			defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree_"+param.Name)

			// Perform cleanup.
			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Fatal("Failed to clean up: ", err)
			}

			// Update policies.
			if err := policyutil.ServeAndVerify(ctx, fdms, cr, []policy.Policy{param.Policy}); err != nil {
				s.Fatal("Failed to update policies: ", err)
			}

			// Setup browser based on the chrome type.
			br, closeBrowser, err := browserfixt.SetUp(ctx, cr, s.Param().(browser.Type))
			if err != nil {
				s.Fatal("Failed to open the browser: ", err)
			}
			defer closeBrowser(cleanupCtx)

			// Open the net-export page and start logging.
			if isLacros(s) {
				if err := annotations.StartOSLogging(ctx, cr, br, kb); err != nil {
					s.Fatal("Failed to start logging: ", err)
				}
			} else {
				if err := annotations.StartLogging(ctx, cr, br); err != nil {
					s.Fatal("Failed to start logging: ", err)
				}
			}

			if err := calendarintegration.TriggerCalendarIntegration(ctx, s, cr, br, nil, tconn, index); err != nil {
				s.Fatal("Failed to trigger and verify calendar integration: ", err)
			}

			// Stop logging and check the logs for annotation.
			foundAnnotation := false
			if isLacros(s) {
				if foundAnnotation, err = annotations.StopOSLoggingCheckLogs(ctx, cr, br, kb, calendarintegration.AnnotationHashCode); err != nil {
					s.Fatal("Failed to stop logging and check logs: ", err)
				}
			} else {
				if foundAnnotation, err = annotations.StopLoggingCheckLogs(ctx, cr, br, calendarintegration.AnnotationHashCode); err != nil {
					s.Fatal("Failed to stop logging and check logs: ", err)
				}
			}

			if param.ShouldFindAnnotation != foundAnnotation {
				s.Fatalf("Unexpected state of calendar_get_events annotation: got %t wanted %t", foundAnnotation, param.ShouldFindAnnotation)
			}
		})
	}
}

func isLacros(s *testing.State) bool {
	return s.Param().(browser.Type) == browser.TypeLacros
}
