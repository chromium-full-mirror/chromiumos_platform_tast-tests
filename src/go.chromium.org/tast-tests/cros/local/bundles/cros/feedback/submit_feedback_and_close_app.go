// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package feedback

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/feedbackapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SubmitFeedbackAndCloseApp,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "User is able to submit a report and then close feedback app",
		Contacts: []string{
			"cros-feedback-app@google.com",
			"xiangdongkong@google.com",
		},
		// ChromeOS > Data > Engineering > Feedback
		BugComponent: "b:1033360",
		SearchFlags: []*testing.StringPair{
			{
				Key:   "feature_id",
				Value: "screenplay-3f028d06-0100-4b5b-b1f3-99ceeaf3d62b",
			},
		},
		Fixture:      "chromeLoggedInWithOsFeedback",
		Attr:         []string{"group:mainline", "group:hw_agnostic", "informational"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      2 * time.Minute,
	})
}

// SubmitFeedbackAndCloseApp verifies the user can submit a report and
// close Feedback app.
func SubmitFeedbackAndCloseApp(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(*chrome.Chrome)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr,
		"ui_dump")

	ui := uiauto.New(tconn).WithTimeout(20 * time.Second)

	// Launch feedback app and go to share data page.
	feedbackRootNode, err := feedbackapp.LaunchAndGoToShareDataPage(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch feedback app and navigating to share data page: ", err)
	}

	// Find send button and then submit the feedback.
	sendButton := nodewith.Name("Send").Role(role.Button).Ancestor(feedbackRootNode)
	if err := ui.DoDefault(sendButton)(ctx); err != nil {
		s.Fatal("Failed to submit feedback: ", err)
	}

	// Verify essential elements exist in the share data page.
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

	// Click done button and verify feedback window is closed.
	doneButton := nodewith.Name("Done").Role(role.Button).Ancestor(feedbackRootNode)

	if err := uiauto.Combine("Verify feedback window is closed",
		ui.DoDefault(doneButton),
		ui.WaitUntilGone(feedbackRootNode),
	)(ctx); err != nil {
		s.Fatal("Failed to verify feedback window is closed: ", err)
	}
}
