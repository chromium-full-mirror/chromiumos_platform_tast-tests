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
		Func:         ScreenshotIsTaken,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify the screenshot is taken in share data page",
		Contacts: []string{
			"cros-feedback-app@google.com",
			"xiangdongkong@google.com",
		},
		// ChromeOS > Data > Engineering > Feedback
		BugComponent: "b:1033360",
		Fixture:      "chromeLoggedInWithOsFeedback",
		SearchFlags: []*testing.StringPair{
			{
				Key:   "feature_id",
				Value: "screenplay-3f028d06-0100-4b5b-b1f3-99ceeaf3d62b",
			},
		},
		Attr:         []string{"group:mainline", "group:hw_agnostic", "informational"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      5 * time.Minute,
	})
}

// ScreenshotIsTaken verifies the screenshot is taken when user navigates to share data page.
func ScreenshotIsTaken(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(*chrome.Chrome)

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

	// Verify screenshot checkbox and image exist.
	// Verify clicking screenshot will open screenshot diaglog.
	screenshotCheckBox := nodewith.Name("Screenshot").Role(role.CheckBox).Ancestor(
		feedbackRootNode)
	previewScreenshotButton := nodewith.Name("Preview Screenshot").Role(
		role.Button).Ancestor(feedbackRootNode)
	screenshotImg := nodewith.Role(role.Image).Ancestor(previewScreenshotButton)
	screenshotDialog := nodewith.Role(role.Dialog).Ancestor(feedbackRootNode).First()

	if err := uiauto.Combine("Verify screenshot exists",
		ui.WaitUntilExists(screenshotCheckBox),
		ui.DoDefault(screenshotImg),
		ui.WaitUntilExists(screenshotDialog),
	)(ctx); err != nil {
		s.Fatal("Failed to verify screenshot exists: ", err)
	}

	// Verify clicking screenshot button will close screenshot diaglog.
	screenshotButton := nodewith.Name("Back").Role(role.Button).Ancestor(feedbackRootNode)

	if err := uiauto.Combine("Verify clicking screenshot button closes dialog",
		ui.DoDefault(screenshotButton),
		ui.WaitUntilGone(screenshotDialog),
	)(ctx); err != nil {
		s.Fatal("Failed to verify clicking screenshot button closes dialog: ", err)
	}
}
