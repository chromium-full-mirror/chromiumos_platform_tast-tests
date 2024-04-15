// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package apps

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/apps/fixture"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/apps/pre"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/apps/helpapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         LaunchHelpAppInBrowserMenu,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Help app can be launched in browser menu",
		Contacts: []string{
			"showoff-eng@google.com",
		},
		BugComponent: "b:690873",
		SoftwareDeps: []string{"chrome", "chrome_internal"},
		Params: []testing.Param{
			{
				Name:              "stable_fieldtrial_testing_config_off",
				ExtraHardwareDeps: hwdep.D(pre.AppsStableModels),
				ExtraAttr:         []string{"group:mainline"},
				Fixture:           fixture.LoggedInFieldTrialConfigDisable,
			}, {
				Name:              "stable_fieldtrial_testing_config_on",
				ExtraHardwareDeps: hwdep.D(pre.AppsStableModels),
				ExtraAttr:         []string{"group:mainline"},
				Fixture:           fixture.LoggedInFieldTrialConfigEnable,
			}, {
				Name:              "unstable",
				ExtraHardwareDeps: hwdep.D(pre.AppsUnstableModels),
				// b:238260020 - disable aged (>1y) unpromoted informational tests
				// ExtraAttr:         []string{"group:mainline", "informational"},
				Fixture: fixture.LoggedIn,
			},
		},
	})
}

// LaunchHelpAppInBrowserMenu verifies launching Help app in chrome browser three dot menu.
func LaunchHelpAppInBrowserMenu(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(fixture.FixtData).Chrome
	tconn := s.FixtValue().(fixture.FixtData).TestAPIConn

	defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)

	conn, err := cr.NewConn(ctx, chrome.BlankURL)
	if err != nil {
		s.Fatal("Failed to lunch a new browser: ", err)
	}
	defer conn.Close()

	ui := uiauto.New(tconn)

	browserAppMenuButtonFinder := nodewith.ClassName("BrowserAppMenuButton").Role(role.PopUpButton)
	helpMenuItemFinder := nodewith.ClassName("MenuItemView").Name("Help")
	getHelpMenuItemFinder := nodewith.ClassName("MenuItemView").Name("Get Help")

	if err := uiauto.Combine("launch Help app from Chrome browser",
		ui.LeftClick(browserAppMenuButtonFinder),
		ui.LeftClick(helpMenuItemFinder),
		ui.LeftClick(getHelpMenuItemFinder),
		helpapp.NewContext(cr, tconn).WaitForApp(),
	)(ctx); err != nil {
		s.Fatal("Failed to launch or render Help app from Chrome browser menu: ", err)
	}
}
