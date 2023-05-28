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

const defaultEmailName = "testuser@gmail.com"

func init() {
	testing.AddTest(&testing.Test{
		Func:         VerifyUserEmailIsDisplayedAndSelectedByDefault,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify user email is displayed and selected by default",
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
		Timeout:      3 * time.Minute,
	})
}

// VerifyUserEmailIsDisplayedAndSelectedByDefault verifies user email is
// displayed and selected by default.
func VerifyUserEmailIsDisplayedAndSelectedByDefault(ctx context.Context, s *testing.State) {
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
	if _, err := feedbackapp.LaunchAndGoToShareDataPage(ctx, tconn); err != nil {
		s.Fatal("Failed to launch feedback app and go to share data page: ", err)
	}

	// Verify user email is displayed by default.
	emailDropdown := nodewith.Name("Select email").ClassName("md-select")
	emailDropdownInfo, err := ui.Info(ctx, emailDropdown)
	if err != nil {
		s.Fatal("Failed to get email dropdown info: ", err)
	}
	if emailDropdownInfo.Value != defaultEmailName {
		s.Fatal("Failed to verify user email is displayed by default")
	}

	// Verify user email is selected by default.
	userEmail := nodewith.Name(defaultEmailName).Role(role.ListBoxOption)
	if err := ui.LeftClickUntil(emailDropdown, ui.WithTimeout(
		2*time.Second).WaitUntilExists(userEmail))(ctx); err != nil {
		s.Fatal("Failed to get user email: ", err)
	}
	userEmailInfo, err := ui.Info(ctx, userEmail)
	if err != nil {
		s.Fatal("Failed to get user email info: ", err)
	}
	if !userEmailInfo.Selected {
		s.Fatal("Failed to verify user email is selected by default")
	}
}
