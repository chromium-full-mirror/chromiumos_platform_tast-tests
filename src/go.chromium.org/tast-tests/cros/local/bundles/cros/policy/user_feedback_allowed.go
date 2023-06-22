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
	"go.chromium.org/tast-tests/cros/local/bundles/cros/policy/userfeedback"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         UserFeedbackAllowed,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Behavior of UserFeedbackAllowed policy on both Ash and Lacros browser",
		Contacts: []string{
			"dp-chromeos-eng@google.com",
			"princya@chromium.org", // Test author
		},
		BugComponent: "b:1129862",
		SoftwareDeps: []string{"chrome", "chrome_internal"},
		Attr:         []string{"group:golden_tier"},
		Params: []testing.Param{{
			Fixture: fixture.ChromePolicyLoggedIn,
			Val:     browser.TypeAsh,
		}, {
			Name:              "lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Fixture:           fixture.LacrosPolicyLoggedIn,
			Val:               browser.TypeLacros,
		}},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.UserFeedbackAllowed{}, pci.VerifiedFunctionalityUI),
		},
		Timeout: 4 * time.Minute,
	})
}

func UserFeedbackAllowed(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()
	isLacros := s.Param().(browser.Type) == browser.TypeLacros

	// Reserve ten seconds for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Connect to Test API to use it with the UI library.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	// Get virtual keyboard to test key combination behavior.
	keyboard, err := input.VirtualKeyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer keyboard.Close(ctx)

	// The popup to send feedback to Google is opened in two ways: 1) Key
	// combination (Alt+Shift+I); 2) From the menu (Chrome Menu > Help >
	// Report an Issue). In this test, we are checking policy using scenario 1).
	for index, param := range userfeedback.GetTestCases() {
		s.Run(ctx, param.Name, func(ctx context.Context, s *testing.State) {
			// Perform cleanup.
			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Fatal("Failed to clean up: ", err)
			}

			// Update policies.
			if err := policyutil.ServeAndVerify(ctx, fdms, cr, []policy.Policy{param.Value}); err != nil {
				s.Fatal("Failed to update policies: ", err)
			}
			// Setup browser based on the chrome type.
			br, closeBrowser, err := browserfixt.SetUp(ctx, cr, s.Param().(browser.Type))
			if err != nil {
				s.Fatal("Failed to open the browser: ", err)
			}
			defer closeBrowser(cleanupCtx)
			defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree_"+param.Name+"_key_combination")

			// Open the net-export page and start logging.
			if isLacros {
				if err := annotations.StartOSLogging(ctx, cr, br, keyboard); err != nil {
					s.Fatal("Failed to start OS logging: ", err)
				}
			} else {
				if err := annotations.StartLogging(ctx, cr, br); err != nil {
					s.Fatal("Failed to start logging: ", err)
				}
			}

			if err := userfeedback.TriggerUserFeedback(ctx, s, cr, br, nil, tconn, index); err != nil {
				s.Fatal("Failed to trigger user feedback: ", err)
			}

			foundAnnotationHelpContentProvider := false
			if isLacros {
				foundAnnotationHelpContentProvider, err = annotations.CheckOSLogs(ctx, cr, userfeedback.HelpContentProviderHashCode)
			} else {
				foundAnnotationHelpContentProvider, err = annotations.CheckLogs(ctx, cr, userfeedback.HelpContentProviderHashCode)
			}
			if err != nil {
				s.Fatal("Failed to check logs: ", err)
			}

			// Wait to allow feedback reports app to log network calls.
			// In contrast to above Help content request, this call is made just once (vs on each keystroke)
			// and is sometimes delayed in logging.
			foundAnnotationErr := testing.Poll(ctx, func(ctx context.Context) (err error) {
				// Check the logs for given annotation.
				isFound := false
				if isLacros {
					isFound, err = annotations.CheckOSLogs(ctx, cr, userfeedback.ChromeFeedbackReportAppHashCode)
				} else {
					isFound, err = annotations.CheckLogs(ctx, cr, userfeedback.ChromeFeedbackReportAppHashCode)
				}
				if err != nil {
					return testing.PollBreak(err)
				}

				if isFound {
					return nil
				}
				return errors.New("Annotation ID not found yet")
			}, &testing.PollOptions{
				Timeout:  40 * time.Second,
				Interval: 10 * time.Second,
			})
			foundAnnotationChromeFeedbackReportApp := foundAnnotationErr == nil

			// Stop logging.
			if isLacros {
				if err := annotations.StopOSLogging(ctx, cr, br, keyboard); err != nil {
					s.Fatal("Failed to stop logging: ", err)
				}
			} else {
				if err := annotations.StopLogging(ctx, cr, br); err != nil {
					s.Fatal("Failed to stop logging: ", err)
				}
			}

			if foundAnnotationHelpContentProvider != param.ShouldFindAnnotation {
				s.Fatalf("help_content_provider annotation mismatch. Expected: %t. Actual: %t", param.ShouldFindAnnotation, foundAnnotationHelpContentProvider)
			}

			if foundAnnotationChromeFeedbackReportApp != param.ShouldFindAnnotation {
				s.Fatalf("chrome_feedback_report_app annotation mismatch. Expected: %t. Actual: %t", param.ShouldFindAnnotation, foundAnnotationChromeFeedbackReportApp)
			}
		})
	}
}
