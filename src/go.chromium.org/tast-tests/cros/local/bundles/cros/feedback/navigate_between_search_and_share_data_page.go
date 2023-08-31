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
		Func:         NavigateBetweenSearchAndShareDataPage,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "User is able to navigate between the search and share data page",
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

// NavigateBetweenSearchAndShareDataPage verifies the user can navigate
// between the search and share data page.
func NavigateBetweenSearchAndShareDataPage(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to Test API: ", err)
	}
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr,
		"ui_dump")

	ui := uiauto.New(tconn).WithTimeout(20 * time.Second)

	// Launch feedback app and go to share data page.
	feedbackRootNode, err := feedbackapp.LaunchAndGoToShareDataPage(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch feedback app and go to share data page: ", err)
	}

	// Verify essential elements exist in the share data page.
	sendButton := nodewith.Name("Send").Role(role.Button).Ancestor(feedbackRootNode)
	attachfilesTitle := nodewith.Name("Attach files").Role(role.StaticText).Ancestor(
		feedbackRootNode)
	emailTitle := nodewith.Name("Email").Role(role.StaticText).Ancestor(feedbackRootNode)
	shareDiagnosticDataTitle := nodewith.Name("Share diagnostic data").Role(
		role.StaticText).Ancestor(feedbackRootNode)

	if err := uiauto.Combine("Verify essential elements exist",
		ui.WaitUntilExists(sendButton),
		ui.WaitUntilExists(attachfilesTitle),
		ui.WaitUntilExists(emailTitle),
		ui.WaitUntilExists(shareDiagnosticDataTitle),
	)(ctx); err != nil {
		s.Fatal("Failed to find element: ", err)
	}

	// Find back button and click.
	backButton := nodewith.Name("Back").Role(role.Button).Ancestor(feedbackRootNode)
	if err := ui.DoDefault(backButton)(ctx); err != nil {
		s.Fatal("Failed to click back button: ", err)
	}

	// Verify the issue description input stores the text user entered previously.
	issueDescription := nodewith.Name(feedbackapp.IssueText).Role(role.StaticText).Ancestor(
		feedbackRootNode)
	if err := ui.WaitUntilExists(issueDescription)(ctx); err != nil {
		s.Fatal("Failed to find issue description user entered previously: ", err)
	}
}
