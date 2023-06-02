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
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/feedbackapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
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
		BugComponent: "b:1263917",
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

	// Hash code for NetworkTrafficAnnotationTag with id.
	const (
		helpContentProviderHashCode     = "92685132"  // help_content_provider
		chromeFeedbackReportAppHashCode = "134729048" // chrome_feedback_report_app
	)

	// The popup to send feedback to Google is opened in two ways: 1) Key
	// combination (Alt+Shift+I); 2) From the menu (Chrome Menu > Help >
	// Report an Issue). In this test, we are checking policy using scenario 1).
	for _, param := range []struct {
		name             string                      // subtest name.
		value            *policy.UserFeedbackAllowed // policy value.
		wantReportOption bool                        // expected result.
		// shouldFindAnnotation states whether given annotations should be found in the net-export log.
		shouldFindAnnotation bool
	}{
		{
			name:                 "allow",
			value:                &policy.UserFeedbackAllowed{Val: true},
			wantReportOption:     true,
			shouldFindAnnotation: true,
		},
		{
			name:                 "deny",
			value:                &policy.UserFeedbackAllowed{Val: false},
			wantReportOption:     false,
			shouldFindAnnotation: false,
		},
		{
			name:                 "unset",
			value:                &policy.UserFeedbackAllowed{Stat: policy.StatusUnset},
			wantReportOption:     true,
			shouldFindAnnotation: true,
		},
	} {
		s.Run(ctx, param.name, func(ctx context.Context, s *testing.State) {
			// Perform cleanup.
			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Fatal("Failed to clean up: ", err)
			}

			// Update policies.
			if err := policyutil.ServeAndVerify(ctx, fdms, cr, []policy.Policy{param.value}); err != nil {
				s.Fatal("Failed to update policies: ", err)
			}

			ui := uiauto.New(tconn).WithTimeout(5 * time.Second)

			// Setup browser based on the chrome type.
			br, closeBrowser, err := browserfixt.SetUp(ctx, cr, s.Param().(browser.Type))
			if err != nil {
				s.Fatal("Failed to open the browser: ", err)
			}
			defer closeBrowser(cleanupCtx)

			// Open the net-export page and start logging.
			if err := annotations.StartLogging(ctx, cr, br); err != nil {
				s.Fatal("Failed to start logging: ", err)
			}

			// Open Chrome to run test.
			conn, err := br.NewConn(ctx, "")
			if err != nil {
				s.Fatal("Failed to connect to the browser: ", err)
			}
			defer conn.Close()

			// 10 seconds should be enough time to wait to make sure a node appears
			// or not.
			waitTimeout := 10 * time.Second

			defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree_"+param.name+"_key_combination")

			// Launch feedback app and go to confirmation page.
			feedbackRootNode, err := feedbackapp.LaunchAndGoToShareDataPage(ctx, tconn)

			if param.wantReportOption {
				if err != nil {
					s.Fatal("Failed to launch feedback app: ", err)
				}

				// Find send button and then click send the feedback.
				sendButton := nodewith.Name("Send").Role(role.Button).Ancestor(feedbackRootNode)
				if err := ui.DoDefault(sendButton)(ctx); err != nil {
					s.Fatal("Failed to submit feedback: ", err)
				}

				// Verify essential elements exist in the confirmation page.
				title := nodewith.Name("Thanks for your feedback").Role(role.StaticText).Ancestor(
					feedbackRootNode)
				newReportButton := nodewith.Name("Send new report").Role(role.Button).Ancestor(
					feedbackRootNode)
				exploreAppLink := nodewith.NameContaining("Explore app").Role(role.Link).Ancestor(
					feedbackRootNode)
				diagnosticsAppLink := nodewith.NameContaining("Diagnostics app").Role(role.Link).Ancestor(
					feedbackRootNode)
				if err := uiauto.Combine("Verify essential elements exist",
					ui.WaitUntilExists(title),
					ui.WaitUntilExists(newReportButton),
					ui.WaitUntilExists(exploreAppLink),
					ui.WaitUntilExists(diagnosticsAppLink),
				)(ctx); err != nil {
					s.Fatal("Failed to find element: ", err)
				}

				// Find Done button and close the feedback window.
				doneButton := nodewith.Name("Done").Role(role.Button).Ancestor(feedbackRootNode)
				if err := uiauto.Combine("Verify feedback window is closed",
					ui.DoDefault(doneButton),
					ui.WaitUntilGone(feedbackRootNode),
				)(ctx); err != nil {
					s.Fatal("Failed to verify feedback window is closed: ", err)
				}
			} else {
				feedbackRoot := nodewith.Name("Send feedback").HasClass("RootView")
				if err := ui.EnsureGoneFor(feedbackRoot, waitTimeout)(ctx); err != nil {
					s.Error("Failed to make sure feedback app is not available: ", err)
				}
			}

			foundAnnotationHelpContentProvider, err := annotations.CheckLogs(ctx, cr, helpContentProviderHashCode)
			if err != nil {
				s.Fatal("Failed to check logs: ", err)
			}

			// Wait to allow feedback reports app to log network calls.
			// In contrast to above Help content request, this call is made just once (vs on each keystroke)
			// and is sometimes delayed in logging.
			foundAnnotationErr := testing.Poll(ctx, func(ctx context.Context) (err error) {
				// Check the logs for given annotation.
				isFound, err := annotations.CheckLogs(ctx, cr, chromeFeedbackReportAppHashCode)
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
			if err := annotations.StopLogging(ctx, cr, br); err != nil {
				s.Fatal("Failed to stop logging: ", err)
			}

			if foundAnnotationHelpContentProvider != param.shouldFindAnnotation {
				s.Fatalf("help_content_provider annotation mismatch. Expected: %t. Actual: %t", param.shouldFindAnnotation, foundAnnotationHelpContentProvider)
			}

			if foundAnnotationChromeFeedbackReportApp != param.shouldFindAnnotation {
				s.Fatalf("chrome_feedback_report_app annotation mismatch. Expected: %t. Actual: %t", param.shouldFindAnnotation, foundAnnotationChromeFeedbackReportApp)
			}
		})
	}
}
