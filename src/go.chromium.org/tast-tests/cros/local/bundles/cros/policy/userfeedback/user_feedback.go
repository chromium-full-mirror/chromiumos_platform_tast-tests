// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package userfeedback contains helpers to verify
// user feedback enabled policy.
package userfeedback

import (
	"context"
	"net/http/httptest"
	"time"

	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/feedbackapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/errors"
)

// TestCase defines test expectations based on the policy value.
type TestCase struct {
	Name                 string
	ShouldFindAnnotation bool
	Value                *policy.UserFeedbackAllowed
	WantReportOption     bool
}

// Hash code for NetworkTrafficAnnotationTag with id.
const (
	HelpContentProviderHashCode     = "92685132"  // help_content_provider
	ChromeFeedbackReportAppHashCode = "134729048" // chrome_feedback_report_app
)

// GetTestCases returns the list of TestCase objects on which
// UserFeedbackAllowed policy is tested.
func GetTestCases() []TestCase {
	// Reordering the TestCase objects in the returned list may break tests.
	return []TestCase{
		{
			Name:                 "deny",
			Value:                &policy.UserFeedbackAllowed{Val: false},
			WantReportOption:     false,
			ShouldFindAnnotation: false,
		},
		{
			Name:                 "allow",
			Value:                &policy.UserFeedbackAllowed{Val: true},
			WantReportOption:     true,
			ShouldFindAnnotation: true,
		},
		{
			Name:                 "unset",
			Value:                &policy.UserFeedbackAllowed{Stat: policy.StatusUnset},
			WantReportOption:     true,
			ShouldFindAnnotation: true,
		},
	}
}

// TriggerUserFeedback verifies feedback app doesnt send reports when policy is
// off.
func TriggerUserFeedback(ctx context.Context, _ *chrome.Chrome, br *browser.Browser, _ *httptest.Server, tconn *chrome.TestConn, paramIndex int) (err error) {
	param := GetTestCases()[paramIndex]

	ui := uiauto.New(tconn).WithTimeout(5 * time.Second)

	// Open Chrome to run test.
	conn, err := br.NewConn(ctx, "")
	if err != nil {
		return errors.Wrap(err, "failed to connect to the browser")
	}
	defer conn.Close()

	// 10 seconds should be enough time to wait to make sure a node appears
	// or not.
	waitTimeout := 10 * time.Second

	// Launch feedback app and go to confirmation page.
	feedbackRootNode, err := feedbackapp.LaunchAndGoToShareDataPage(ctx, tconn)

	if param.WantReportOption {
		if err != nil {
			return errors.Wrap(err, "failed to launch feedback app")
		}

		// Find send button and then click send the feedback.
		sendButton := nodewith.Name("Send").Role(role.Button).Ancestor(feedbackRootNode)
		if err := ui.DoDefault(sendButton)(ctx); err != nil {
			return errors.Wrap(err, "failed to submit feedback")
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
			return errors.Wrap(err, "failed to find element")
		}

		// Find Done button and close the feedback window.
		doneButton := nodewith.Name("Done").Role(role.Button).Ancestor(feedbackRootNode)
		if err := uiauto.Combine("Verify feedback window is closed",
			ui.DoDefault(doneButton),
			ui.WaitUntilGone(feedbackRootNode),
		)(ctx); err != nil {
			return errors.Wrap(err, "failed to verify feedback window is closed")
		}
	} else {
		feedbackRoot := nodewith.Name("Send feedback").HasClass("RootView")
		if err := ui.EnsureGoneFor(feedbackRoot, waitTimeout)(ctx); err != nil {
			return errors.Wrap(err, "failed to make sure feedback app is not available")
		}
	}

	return nil
}
