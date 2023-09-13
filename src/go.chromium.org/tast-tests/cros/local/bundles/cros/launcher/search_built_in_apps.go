// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package launcher

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/launcher"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SearchBuiltInApps,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Launches a built-in app through the launcher",
		Contacts: []string{
			"cros-system-ui-eng@google.com",
			"tbarzic@chromium.org",
			"chromeos-sw-engprod@google.com",
		},
		BugComponent: "b:1288350",
		Attr:         []string{"group:mainline"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "chromeLoggedIn",
		Params: []testing.Param{{
			Name: "clamshell_mode",
			Val:  launcher.TestCase{TabletMode: false},
			// b/229135388
			ExtraAttr: []string{"informational"},
		}, {
			Name:              "tablet_mode",
			Val:               launcher.TestCase{TabletMode: true},
			ExtraHardwareDeps: hwdep.D(hwdep.InternalDisplay()),
			ExtraAttr:         []string{"group:hw_agnostic"},
		}},
	})
}

// SearchBuiltInApps searches for the Settings app in the Launcher.
func SearchBuiltInApps(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	app := apps.Settings

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
	cleanup, err := launcher.SetUpLauncherTest(ctx, tconn, testCase.TabletMode, false /*stabilizeAppCount*/)
	if err != nil {
		s.Fatal("Failed to set up launcher test case: ", err)
	}
	defer cleanup(cleanupCtx)

	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	if err := launcher.SearchAndWaitForAppOpen(tconn, kb, app)(ctx); err != nil {
		s.Fatal("Failed to launch app: ", err)
	}
}
