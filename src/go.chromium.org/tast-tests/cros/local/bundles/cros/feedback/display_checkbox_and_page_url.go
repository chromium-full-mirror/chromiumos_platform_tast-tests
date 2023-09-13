// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package feedback

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
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
		Func:         DisplayCheckboxAndPageURL,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify url matches the current website where Feedback app is opened and checkbox is checked by default",
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
		Timeout:      2 * time.Minute,
	})
}

// DisplayCheckboxAndPageURL verifies the url matches the current website from where
// user opens the Feedback app and the checkbox should be checked by default.
func DisplayCheckboxAndPageURL(ctx context.Context, s *testing.State) {
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

	// Open chrome browser.
	if err := apps.Launch(ctx, tconn, apps.Chrome.ID); err != nil {
		s.Fatal("Failed to launch chrome app: ", err)
	}

	if err := ash.WaitForApp(ctx, tconn, apps.Chrome.ID, time.Minute); err != nil {
		s.Fatal("Chrome app did not appear in shelf after launch: ", err)
	}

	// Launch feedback app and go to share data page.
	feedbackRootNode, err := feedbackapp.LaunchAndGoToShareDataPage(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch feedback app and go to share data page: ", err)
	}

	// Verify url text exists.
	urlText := nodewith.NameContaining("chrome://newtab/").Role(role.Link).Ancestor(feedbackRootNode)
	if err := ui.WaitUntilExists(urlText)(ctx); err != nil {
		s.Fatal("Failed to find element: ", err)
	}

	// Verify share url checkbox is checked by default.
	checkboxAncestor := nodewith.NameContaining("Share URL").Role(
		role.GenericContainer).Ancestor(feedbackRootNode)
	checkbox := nodewith.Role(role.CheckBox).Ancestor(checkboxAncestor)
	if err := ui.WaitUntilExists(checkbox.Attribute("checked", "true"))(ctx); err != nil {
		s.Fatal("Failed to find checked share url checkbox: ", err)
	}
}
