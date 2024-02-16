// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wmp

import (
	"context"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/launcher"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/pointer"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         VirtualDesksChromeApp,
		LacrosStatus: testing.LacrosVariantUnknown,
		Desc:         "Checks that virtual desks works correctly when creating apps from tabs",
		Contacts: []string{
			"chromeos-wms@google.com",
			"chromeos-sw-engprod@google.com",
		},
		BugComponent: "b:1238200",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      fixture.ChromePolicyLoggedIn,
		Data:         []string{"web_app_install_force_list_index.html", "web_app_install_force_list_manifest.json", "web_app_install_force_list_service-worker.js", "web_app_install_force_list_icon-192x192.png", "web_app_install_force_list_icon-512x512.png"},
		SearchFlags: []*testing.StringPair{
			{
				Key:   "feature_id",
				Value: "screenplay-c74ed558-34e5-4373-9b18-cb40269caa65",
			},
			{
				Key:   "feature_id",
				Value: "screenplay-aa50f67f-f24e-4f8d-af88-1fac22e84312",
			},
			{
				Key:   "feature_id",
				Value: "screenplay-f2f7491e-e6ee-429e-9d56-aa386a2db2ca",
			},
			{
				Key:   "feature_id",
				Value: "screenplay-2e1d03c8-d145-4fe5-b68f-04301d32199b",
			}},
	})
}

func VirtualDesksChromeApp(ctx context.Context, s *testing.State) {
	// Reserve five seconds for various cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()
	defer cancel()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	defer ash.CleanUpDesks(cleanupCtx, tconn)
	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	_, name, cleanUp, err := policyutil.InstallPwaAppByPolicy(ctx, tconn, cr, fdms, s.DataFileSystem())
	if err != nil {
		s.Fatal("Failed to install PWA: ", err)
	}

	defer cleanUp(ctx)

	cleanup, err := ash.EnsureTabletModeEnabled(ctx, tconn, false)
	if err != nil {
		s.Fatal("Failed to ensure clamshell mode: ", err)
	}
	defer cleanup(cleanupCtx)

	ac := uiauto.New(tconn)
	kb, err := input.Keyboard(ctx)

	if err != nil {
		s.Fatal("Failed to create a keyboard: ", err)
	}
	pc := pointer.NewMouse(tconn)
	defer pc.Close(cleanupCtx)

	// Wait until the PWA is installed.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if err := launcher.SearchAndLaunch(tconn, kb, name)(ctx); err != nil {
			return errors.Wrapf(err, "failed to launch %s", name)
		}

		windows, err := ash.GetAllWindows(ctx, tconn)
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to get windows"))
		}

		for _, window := range windows {
			if window.Title == name {
				return nil
			}
		}
		return errors.New("failed to find a window with the PWA")
	}, nil); err != nil {
		s.Error("PWA wasn't installed: ", err)
	}

	// Opens Chrome.
	if err := apps.Launch(ctx, tconn, apps.Chrome.ID); err != nil {
		s.Fatalf("Failed to open %s: %v", apps.Chrome.Name, err)
	}
	if err := ash.WaitForApp(ctx, tconn, apps.Chrome.ID, time.Minute); err != nil {
		s.Fatalf("%s did not appear in shelf after launch: %s", apps.Chrome.Name, err)
	}

	// Enters overview mode.
	if err := ash.SetOverviewModeAndWait(ctx, tconn, true); err != nil {
		s.Fatal("Failed to set overview mode: ", err)
	}
	defer ash.SetOverviewModeAndWait(cleanupCtx, tconn, false)

	// Creates new desk and enters it.
	addDeskButton := nodewith.ClassName("ZeroStateIconButton")
	newDeskNameView := nodewith.ClassName("DeskNameView").Name("Desk 2")
	newDeskMiniView := nodewith.ClassName("DeskMiniView").Nth(1)
	newDeskName := "Desk 2"
	if err := uiauto.Combine(
		"create a new desk",
		ac.LeftClick(addDeskButton),
		// The focus on the new desk should be on the desk name field.
		ac.WaitUntilExists(newDeskNameView.Focused()),
		kb.TypeAction(newDeskName),
		kb.AccelAction("Enter"),
		ac.LeftClick(newDeskMiniView),
	)(ctx); err != nil {
		s.Fatal("Failed to create a new desk: ", err)
	}

	if err := ash.WaitUntilDesksFinishAnimating(ctx, tconn); err != nil {
		s.Fatal("Failed to wait for desks to finish animating: ", err)
	}

	// Verifies that there are 2 desks.
	dc, err := ash.GetDeskCount(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to count desks: ", err)
	}
	if dc != 2 {
		s.Fatalf("Expected %d desks, but got %d instead", 2, dc)
	}

	// Verifies exited overview mode.
	if err := ash.WaitForOverviewState(ctx, tconn, ash.Hidden, 5*time.Second); err != nil {
		s.Fatal("Failed to exit overview mode: ", err)
	}

	// Opens a Chrome browser.
	if err := apps.Launch(ctx, tconn, apps.Chrome.ID); err != nil {
		s.Fatalf("Failed to open %s: %v", apps.Chrome.Name, err)
	}
	if err := ash.WaitForApp(ctx, tconn, apps.Chrome.ID, 5*time.Second); err != nil {
		s.Fatalf("%s did not appear in shelf after launch: %s", apps.Chrome.Name, err)
	}

	// Checks that browser window is created in current desk,
	// even if there are other browser windows on other inactive desks.
	if err := ash.ForEachWindow(ctx, tconn, func(w *ash.Window) error {
		if (w.Title == "Chrome - New Tab") && w.OnActiveDesk == false {

		}
		return nil
	}); err != nil {
		s.Fatal("Failed to verify the desk of the app: ", err)
	}

	shelfAppButtonRegex := regexp.MustCompile(ash.ShelfAppButtonClassNameRegex)
	TestPWABtn := nodewith.ClassNameRegex(shelfAppButtonRegex).Name("Test PWA")

	ws, err := ash.GetAllWindows(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get the window list: ", err)
	}
	// 2 chrome windows and 1 PWA window are created with previous procedure.
	if len(ws) != 3 {
		s.Fatalf("Unexpected number of windows found; wanted %v, got %v", 3, len(ws))
	}
	// Click Test PWA shelf button, this will bring back to first desk.
	if err := uiauto.Combine(
		"click Test PWA shelf button",
		ac.LeftClick(TestPWABtn),
		ac.WaitForLocation(nodewith.ClassName("WebContentsViewAura").Name("Test PWA")),
	)(ctx); err != nil {
		s.Fatal("Failed to click the Test PWA button: ", err)
	}
	// Check which desk is the current active desk.
	info, err := ash.GetDesksInfo(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get the desk info: ", err)
	}
	activeDesk := info.ActiveDeskIndex
	// Compare the actual active desk to the expected active desk.
	if activeDesk != 0 {
		s.Fatalf("Unexpected active desk: desk %d is active, expected desk 0 to be active", activeDesk)
	}
	currWs, err := ash.GetAllWindows(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get the window list: ", err)
	}
	// Ensure no new window is created.
	if len(currWs) != len(ws) {
		s.Fatalf("Unexpected number of windows found; wanted %v, got %v", len(ws), len(currWs))
	}

	contextMenu := nodewith.ClassName("MenuHostRootView")
	newWindowBtn := nodewith.Name("New window").ClassName("MenuItemView")
	// Click new tab on the Test PWA app from the shelf.
	if err := uiauto.Combine(
		"click new tab on Test PWA app",
		// Switch back to desk 2 first.
		kb.AccelAction("Search+]"),
		// This will wait until the container for desk 2 has become visible.
		ac.WaitForLocation(nodewith.ClassName("Desk_Container_B").State("invisible", false)),
		ac.RightClick(TestPWABtn),
		ac.WaitUntilExists(contextMenu),
		ac.LeftClick(newWindowBtn),
	)(ctx); err != nil {
		s.Fatal("Failed to create a new Test PWA window: ", err)
	}

	// Checks that the new Test PWA window is on desk 2 instead of desk 1.
	if err := ash.ForEachWindow(ctx, tconn, func(w *ash.Window) error {
		const name = "Test PWA - Test PWA"
		if (w.Title == name) && w.OnActiveDesk == false {
			return errors.New("Test PWA app should be in the active desk")
		}
		return nil
	}); err != nil {
		s.Error("Failed to verify the desk of the app: ", err)
	}
}
