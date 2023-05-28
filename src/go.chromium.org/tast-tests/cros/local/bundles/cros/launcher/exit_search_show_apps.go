// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package launcher

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/launcher"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ExitSearchShowApps,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks for best match search results in the launcher",
		Contacts: []string{
			"cros-system-ui-eng@google.com",
			"yulunwu@chromium.org",
			"chromeos-sw-engprod@google.com",
		},
		BugComponent: "b:1288350",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{{
			Name:    "clamshell_mode",
			Fixture: "chromeLoggedIn",
			Val:     launcher.TestCase{TabletMode: false},
		}, {
			Name:              "tablet_mode",
			Fixture:           "chromeLoggedIn",
			Val:               launcher.TestCase{TabletMode: true},
			ExtraHardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		}},
	})
}

type exitSearchShowAppsTestCase struct {
	name             string
	exitSearchAction uiauto.Action
}

// ExitSearchShowApps checks that exiting the productivity launcher search UI
// transitions the user back to the app list UI.
func ExitSearchShowApps(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(*chrome.Chrome)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to find keyboard: ", err)
	}
	defer kb.Close(ctx)

	testCase := s.Param().(launcher.TestCase)
	tabletMode := testCase.TabletMode

	// Setting up a launcher test opens the launcher.
	cleanup, err := launcher.SetUpLauncherTest(ctx, tconn, tabletMode, false /*stabilizeAppCount*/)
	if err != nil {
		s.Fatal("Failed to set up launcher test case: ", err)
	}
	defer cleanup(cleanupCtx)

	ui := uiauto.New(tconn)

	subtests := []exitSearchShowAppsTestCase{
		{
			name:             "Escape Key",
			exitSearchAction: kb.TypeKeyAction(input.KEY_ESC),
		},
		{
			name:             "Close Button",
			exitSearchAction: ui.DoDefault(nodewith.ClassName("SearchBoxImageButton")),
		},
		{
			name: "Multiple Backspace",
			exitSearchAction: uiauto.Combine("Multiple Backspace",
				kb.TypeKeyAction(input.KEY_BACKSPACE),
				kb.TypeKeyAction(input.KEY_BACKSPACE),
				kb.TypeKeyAction(input.KEY_BACKSPACE),
				kb.TypeKeyAction(input.KEY_BACKSPACE)),
		},
	}

	for _, subtest := range subtests {
		s.Run(ctx, subtest.name, func(ctx context.Context, s *testing.State) {
			defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree_"+subtest.name)

			if err := uiauto.Combine("search launcher",
				launcher.Search(tconn, kb, "test"),
				subtest.exitSearchAction,
				launcher.WaitForLauncherSearchExit(tconn, tabletMode),
			)(ctx); err != nil {
				s.Fatal("Failed to enter and exit search: ", err)
			}
		})
	}

}
