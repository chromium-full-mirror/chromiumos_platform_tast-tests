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
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/feedbackapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         OpenChromebookHelpForum,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "User is able to open chromebook help forum",
		Contacts: []string{
			"cros-feedback-app@google.com",
			"xiangdongkong@google.com",
		},
		BugComponent: "b:1033360", // ChromeOS > Data > Engineering > Feedback
		Attr:         []string{"group:mainline", "group:hw_agnostic", "informational"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      3 * time.Minute,
		SearchFlags: []*testing.StringPair{
			{
				Key:   "feature_id",
				Value: "screenplay-3f028d06-0100-4b5b-b1f3-99ceeaf3d62b",
			},
		},
		Params: []testing.Param{{
			Name:    "ash",
			Fixture: "chromeLoggedInWithOsFeedback",
			Val:     browser.TypeAsh,
		}, {
			Name:              "lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Fixture:           "lacrosOsFeedback",
			Val:               browser.TypeLacros,
		}},
	})
}

const linkAddress = "https://support.google.com/chromebook/?hl=en#topic=3399709"

// OpenChromebookHelpForum verifies the user is able to open chromebook help forum.
func OpenChromebookHelpForum(ctx context.Context, s *testing.State) {
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

	// Find link and click.
	link := nodewith.NameContaining("Chromebook community").Ancestor(feedbackRootNode)
	if err := ui.DoDefault(link)(ctx); err != nil {
		s.Fatal("Failed to find link: ", err)
	}

	bt := s.Param().(browser.Type)

	// Verify browser is opened.
	id := apps.Chrome.ID
	if bt != browser.TypeAsh {
		id = apps.Lacros.ID
	}
	if err = ash.WaitForApp(ctx, tconn, id, time.Minute); err != nil {
		s.Fatal("Could not find browser in shelf after launch: ", err)
	}

	// Verify browser contains correct address.
	br, brCleanUp, err := browserfixt.Connect(ctx, cr, bt)
	if err != nil {
		s.Fatalf("Failed to connect to active browser for %v: %v", linkAddress, err)
	}
	defer brCleanUp(ctx)
	c, err := br.NewConnForTarget(ctx, chrome.MatchTargetURL(linkAddress))
	if err != nil {
		s.Fatalf("Failed to find browser window for %v: %v", linkAddress, err)
	}
	defer c.Close()
	defer c.CloseTarget(ctx)
}
