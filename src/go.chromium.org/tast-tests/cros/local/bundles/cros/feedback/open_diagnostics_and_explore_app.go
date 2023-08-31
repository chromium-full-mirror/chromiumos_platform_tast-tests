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

// openAppsTestParam contains all the data needed to run a single test iteration.
type openAppsTestParam struct {
	appLinkName string
	app         apps.App
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         OpenDiagnosticsAndExploreApp,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "User is able to open diagnostics and explore app",
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
		Params: []testing.Param{{
			Name: "open_explore_app",
			Val: openAppsTestParam{
				appLinkName: "Explore app",
				app:         apps.Help,
			},
		}, {
			Name: "open_diagnostics_app",
			Val: openAppsTestParam{
				appLinkName: "Diagnostics app",
				app:         apps.Diagnostics,
			},
		}},
	})
}

// OpenDiagnosticsAndExploreApp verifies the user is able to open diagnostics and explore app.
func OpenDiagnosticsAndExploreApp(ctx context.Context, s *testing.State) {
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

	ui := uiauto.New(tconn)

	// Launch feedback app and navigate to confirmation page.
	feedbackRootNode, err := feedbackapp.LaunchAndGoToConfirmationPage(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch feedback app and go to confirmation page: ", err)
	}

	// Find app link and click.
	appLink := nodewith.NameContaining(s.Param().(openAppsTestParam).appLinkName).Role(
		role.Link).Ancestor(feedbackRootNode).First()
	if err := ui.DoDefault(appLink)(ctx); err != nil {
		s.Fatal("Failed to find app link: ", err)
	}

	// Verify app is opened.
	if err = ash.WaitForApp(ctx, tconn, s.Param().(openAppsTestParam).app.ID, time.Minute); err != nil {
		s.Fatal("Could not find app in shelf after launch: ", err)
	}
}
