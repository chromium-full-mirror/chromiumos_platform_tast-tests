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
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ShowMissingDescriptionErrorMessage,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Show missing description error when continue without description",
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

// ShowMissingDescriptionErrorMessage verifies the user click continue button
// with no issue description will show error message. Now enter text, error message will disappear.
func ShowMissingDescriptionErrorMessage(ctx context.Context, s *testing.State) {
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

	// Set up keyboard.
	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to find keyboard: ", err)
	}
	defer kb.Close(ctx)

	// Launch feedback app.
	feedbackRootNode, err := feedbackapp.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch feedback app: ", err)
	}

	// Find continue button and click it.
	continueButton := nodewith.Name("Continue").Role(role.Button)
	if err := ui.DoDefault(continueButton)(ctx); err != nil {
		s.Fatal("Failed to click continue button: ", err)
	}

	// Verify missing description error message exists.
	missingDescriptionErrorMessage := nodewith.Name("Description is required").Role(
		role.StaticText).Ancestor(feedbackRootNode)
	if err := ui.WaitUntilExists(missingDescriptionErrorMessage)(ctx); err != nil {
		s.Fatal("Failed to find missing description error message: ", err)
	}

	// Find the issue description text input.
	issueDescriptionInput := nodewith.Role(role.TextField).Ancestor(feedbackRootNode)
	if err := ui.EnsureFocused(issueDescriptionInput)(ctx); err != nil {
		s.Fatal("Failed to find the issue description text input: ", err)
	}

	// Type issue description.
	if err := kb.Type(ctx, "Make the Description is required disappear"); err != nil {
		s.Fatal("Failed to type issue description: ", err)
	}

	// Verify missing description error message is disappeared.
	if err := ui.WaitUntilGone(missingDescriptionErrorMessage)(ctx); err != nil {
		s.Fatal("Failed to make missing description error message disappeared: ", err)
	}
}
