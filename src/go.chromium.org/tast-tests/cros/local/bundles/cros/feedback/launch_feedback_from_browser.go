// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package feedback

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         LaunchFeedbackFromBrowser,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Feedback app can be launched from the browser",
		Contacts: []string{
			"cros-feedback-app@google.com",
			"xiangdongkong@google.com",
		},
		// ChromeOS > Data > Engineering > Feedback
		BugComponent: "b:1033360",
		Attr:         []string{"group:mainline", "group:hw_agnostic", "informational"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      1 * time.Minute,
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

const settingLinkAddress = "chrome://settings/help"

// LaunchFeedbackFromBrowser verifies the Feedback app can be launched from the browser.
func LaunchFeedbackFromBrowser(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	bt := s.Param().(browser.Type)

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

	conn, _, closeBrowser, err := browserfixt.SetUpWithURL(ctx, cr, bt, settingLinkAddress)
	if err != nil {
		s.Fatal("Failed to setup chrome: ", err)
	}
	defer closeBrowser(cleanupCtx)
	defer conn.Close()

	link := nodewith.Name("Report an issue").Role(role.Link)
	feedbackHeading := nodewith.Name("Send feedback").Role(role.Heading)

	// The feedback app is a SWA. It may not be ready when clicking the "report an
	// issue" button. When the app does not show up, just try to launch it again
	// until the app is launched successfully or when it times out.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		// Open feedback app from browser.
		if err := uiauto.Combine("Open feedback app from browser",
			ui.DoDefault(link),
			ui.WithTimeout(4*time.Second).WaitUntilExists(feedbackHeading),
		)(ctx); err != nil {
			return errors.Wrap(err, "failed to open feedback app from browser")
		}
		return nil
	}, &testing.PollOptions{Interval: time.Second, Timeout: time.Minute}); err != nil {
		s.Fatal("Failed to launch feedback app")
	}
}
