// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wmp

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/common/ui"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast/core/testing"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         MoveTabToAnotherWindowMenu,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Check if the move tab to another window menu is grouped by desks",
		Contacts: []string{
			"chromeos-wm@google.com",
			"chromeos-consumer-engprod@google.com",
		},
		// ChromeOS > Software > Window Management
		BugComponent: "b:1238037",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      chrome.GAIALoginTimeout + arc.BootTimeout + 120*time.Second,
		VarDeps:      []string{ui.GaiaPoolDefaultVarName},
		Params: []testing.Param{{
			Fixture: "chromeLoggedIn",
		}, {
			Name:              "lacros",
			Fixture:           "lacros",
			ExtraSoftwareDeps: []string{"lacros"},
		}},
		SearchFlags: []*testing.StringPair{
			{
				Key:   "feature_id",
				Value: "screenplay-ff7bb1e3-d3d8-45b4-b48d-16163633c78d",
			},
			{
				Key:   "feature_id",
				Value: "screenplay-c7f54dee-97c4-4834-b4c5-dbee34b1cb0a",
			},
			{
				Key:   "feature_id",
				Value: "screenplay-0996004d-b395-4948-899f-1eba89966e34",
			}},
	})
}

func MoveTabToAnotherWindowMenu(ctx context.Context, s *testing.State) {
	// Reserve five seconds for various cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	defer ash.CleanUpDesks(cleanupCtx, tconn)
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_dump")

	// Ensure there is no window open before test starts.
	if err := ash.CloseAllWindows(ctx, tconn); err != nil {
		s.Fatal("Failed to ensure no window is open: ", err)
	}

	// Open a browser window on Desk 1.
	browserApp, err := apps.PrimaryBrowser(ctx, tconn)
	if err != nil {
		s.Fatal("Could not find browser app info: ", err)
	}
	if err := apps.Launch(ctx, tconn, browserApp.ID); err != nil {
		s.Fatal("Failed to launch chrome: ", err)
	}
	if err := ash.WaitForApp(ctx, tconn, browserApp.ID, time.Minute); err != nil {
		s.Fatal("Browser did not appear in shelf after launch: ", err)
	}

	// Create 4 desks.
	const numNewDesks = 4
	for i := 1; i <= numNewDesks; i++ {
		if err := ash.CreateNewDesk(ctx, tconn); err != nil {
			s.Fatalf("Failed to create the Desk %v: %v", i+1, err)
		}
		if err := ash.ActivateDeskAtIndex(ctx, tconn, i); err != nil {
			s.Fatalf("Failed to activate Desk %v: %v", i+1, err)
		}

		// Open a browser window.
		browserApp, err := apps.PrimaryBrowser(ctx, tconn)
		if err != nil {
			s.Fatal("Could not find browser app info: ", err)
		}
		if err := apps.Launch(ctx, tconn, browserApp.ID); err != nil {
			s.Fatal("Failed to launch chrome: ", err)
		}
		if _, err := ash.WaitForAnyWindow(ctx, tconn, func(w *ash.Window) bool { return w.OnActiveDesk && w.IsVisible && !w.IsAnimating }); err != nil {
			s.Fatal("Failed to open and wait for browser window on active desk: ", err)
		}
	}
	ac := uiauto.New(tconn)
	if err := verifyTabGroupMenu(ctx, tconn, ac); err != nil {
		s.Fatal("Failed to verify the tab group menu: ", err)
	}
}

func verifyTabGroupMenu(ctx context.Context, tconn *chrome.TestConn, ac *uiauto.Context) error {
	info, err := ash.GetDesksInfo(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to get the desk info")
	}
	numDesks := info.NumDesks

	// Activate Desk 1.
	if info.ActiveDeskIndex != 0 {
		if err := ash.ActivateDeskAtIndex(ctx, tconn, 0); err != nil {
			return errors.Wrap(err, "failed to activate Desk 1")
		}
	}

	// Right click on a tab and move mouse to move tab to another window item.
	tab := nodewith.ClassName("Tab").Name("New Tab")
	if err := ac.RightClick(tab)(ctx); err != nil {
		return errors.Wrap(err, "failed to right click the tab view")
	}

	moveTabToAnotherWindowItem := nodewith.ClassName("MenuItemView").Name("Move tab to another window")
	newWindowItem := nodewith.ClassName("MenuItemView").Name("New window")
	if err := uiauto.Combine(
		"move mouse to and click new window item",
		ac.MouseMoveTo(moveTabToAnotherWindowItem, 0),
		ac.DoDefault(moveTabToAnotherWindowItem),
		ac.WaitUntilExists(newWindowItem),
	)(ctx); err != nil {
		return err
	}

	// Verify the tab group menu.
	for i := 2; i <= numDesks; i++ {
		deskItem := nodewith.ClassName("MenuItemView").Name(fmt.Sprintf("Desk Desk %d has 1 browser windows open", i))
		if err := ac.WithTimeout(5 * time.Second).WaitUntilExists(deskItem)(ctx); err != nil {
			return errors.Wrapf(err, "failed to find Desk %d item", i)
		}
		tabItem := nodewith.ClassName("MenuItemView").Name(fmt.Sprintf("New Tab belongs to desk Desk %d", i))
		if err := ac.WithTimeout(5 * time.Second).WaitUntilExists(tabItem)(ctx); err != nil {
			return errors.Wrapf(err, "failed to find the tab item of Desk %d", i)
		}
	}
	return nil
}
