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
	"go.chromium.org/tast-tests/cros/local/bundles/cros/policy/networkrequestmonitor"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/policy/userfeedback"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/netexport"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/testenv/proxy"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: UserFeedbackAllowed,
		Desc: "Behavior of UserFeedbackAllowed policy on Ash browser",
		Contacts: []string{
			"dp-chromeos-eng@google.com",
			"princya@chromium.org", // Test author
		},
		BugComponent: "b:1129862",
		// TODO(b/302232315): mitmproxy fails to start on arm devices.
		// Remove architecture restrictions when the problem is solved.
		SoftwareDeps: []string{"chrome", "chrome_internal", "no_arm"},
		Attr:         []string{"group:golden_tier", "group:hw_agnostic"},
		Fixture:      fixture.ChromePolicyLoggedIn,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.UserFeedbackAllowed{}, pci.VerifiedFunctionalityUI),
		},
		Timeout: 5 * time.Minute,
	})
}

func UserFeedbackAllowed(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	// Reserve ten seconds for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	mp, err := proxy.NewMitmProxy(ctx,
		proxy.DumpHTTPFlow(true),
	)
	if err != nil {
		s.Fatal("Failed to start mitmproxy: ", err)
	}
	defer mp.Close(cleanupCtx)
	if err := mp.Connect(ctx, cr); err != nil {
		s.Fatal("Failed to configure chrome for proxy: ", err)
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
	for key, param := range userfeedback.TestCases() {
		s.Run(ctx, param.Name, func(ctx context.Context, s *testing.State) {
			// Perform cleanup.
			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Fatal("Failed to clean up: ", err)
			}

			// Update policies.
			if err := policyutil.ServeAndVerify(ctx, fdms, cr, []policy.Policy{param.Policy}); err != nil {
				s.Fatal("Failed to update policies: ", err)
			}
			// Setup browser based on the chrome type.
			br, closeBrowser, err := browserfixt.SetUp(ctx, cr, browser.TypeAsh)
			if err != nil {
				s.Fatal("Failed to open the browser: ", err)
			}
			defer closeBrowser(cleanupCtx)
			defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree_"+param.Name+"_key_combination")

			// Open the net-export page and start logging.
			netExport, err := netexport.Start(ctx, cr, br, browser.TypeAsh)
			if err != nil {
				s.Fatal("Failed to start net export: ", err)
			}
			defer netExport.Cleanup(cleanupCtx)

			if err := userfeedback.TriggerUserFeedback(ctx,
				networkrequestmonitor.OptionalServiceParams{
					Chrome:        cr,
					Browser:       br,
					PolicySetting: key}); err != nil {
				s.Fatal("Failed to trigger user feedback: ", err)
			}

			foundAnnotationHelpContentProvider, err := netExport.Find(userfeedback.HelpContentProviderHashCode)
			if err != nil {
				s.Fatal("Failed to check logs: ", err)
			}

			// Wait to allow feedback reports app to log network calls.
			// In contrast to above Help content request, this call is made just once (vs on each keystroke)
			// and is sometimes delayed in logging.
			foundAnnotationChromeFeedbackReportApp, err := netExport.FindUntil(ctx, userfeedback.ChromeFeedbackReportAppHashCode, &testing.PollOptions{
				Timeout:  40 * time.Second,
				Interval: 10 * time.Second,
			})
			if err != nil {
				s.Fatal("Failed to check for chrome_feedback_report_app annotation: ", err)
			}

			if foundAnnotationHelpContentProvider != param.ShouldFindAnnotation {
				s.Fatalf("help_content_provider annotation mismatch. Expected: %t. Actual: %t", param.ShouldFindAnnotation, foundAnnotationHelpContentProvider)
			}

			if foundAnnotationChromeFeedbackReportApp != param.ShouldFindAnnotation {
				s.Fatalf("chrome_feedback_report_app annotation mismatch. Expected: %t. Actual: %t", param.ShouldFindAnnotation, foundAnnotationChromeFeedbackReportApp)
			}

			resp, err := mp.DumpHTTPFlow(ctx, true, false)
			if err != nil {
				s.Fatal("Failed to get dump httpflow from mitmproxy: ", err)
			}

			v := proxy.NewNetworkVerifier(resp)
			allow := param.TrafficShouldFind
			disallow := param.TrafficShouldNotFind
			// allow/disallow traffic are passed by test case param.
			// We will ignore traffic other than the allow and disallow list for verifying diff.
			if err := v.Verify(allow, disallow, []string{".*"}); err != nil {
				s.Fatal("Diff test result: ", err)
			}
		})
	}
}
