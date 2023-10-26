// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wmp

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/pointer"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ZeroStateDesksBar,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that zero state desks bar in overview works correctly",
		Contacts: []string{
			"chromeos-wms@google.com",
			"chromeos-sw-engprod@google.com",
			"dandersson@google.com",
		},
		//  ChromeOS > Software > Window Management > Virtual Desks
		BugComponent: "b:1238200",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "chromeLoggedIn",
		SearchFlags: []*testing.StringPair{{
			Key:   "feature_id",
			Value: "screenplay-0996004d-b395-4948-899f-1eba89966e34",
		}},
	})
}

func ZeroStateDesksBar(ctx context.Context, s *testing.State) {
	// Reserve five seconds for various cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	cleanup, err := ash.EnsureTabletModeEnabled(ctx, tconn, false)
	if err != nil {
		s.Fatal("Failed to ensure clamshell mode: ", err)
	}
	defer cleanup(cleanupCtx)

	defer ash.CleanUpDesks(cleanupCtx, tconn)
	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	ac := uiauto.New(tconn)

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to create a keyboard: ", err)
	}
	pc := pointer.NewMouse(tconn)
	defer pc.Close(cleanupCtx)

	// Enters overview mode.
	if err := ash.SetOverviewModeAndWait(ctx, tconn, true); err != nil {
		s.Fatal("Failed to set overview mode: ", err)
	}
	defer ash.SetOverviewModeAndWait(cleanupCtx, tconn, false)

	// Zero state desks bar is shown when there is only one desk. Tab to
	// default desk button inside and press "Enter" should expand the desks
	// bar but no new desk should be created.
	desk1DeskNameView := nodewith.ClassName("DeskNameView").First()
	if err := uiauto.Combine(
		"press the default desk button to expand desks bar",
		kb.AccelAction("Tab"),
		kb.AccelAction("Enter"),
		// The desk name view of the default desk mini view should be focused.
		ac.WaitUntilExists(desk1DeskNameView.Focused()),
	)(ctx); err != nil {
		s.Fatal("Failed to switch to expanded desks bar: ", err)
	}
	// Verifies that there is only 1 desk.
	oneDeskMiniViewsInfo, err := ash.FindDeskMiniViews(ctx, ac)
	if err != nil {
		s.Fatal("Failed to find desks: ", err)
	}
	if len(oneDeskMiniViewsInfo) != 1 {
		s.Fatalf("Got %v desks, want 1 desk", len(oneDeskMiniViewsInfo))
	}

	// Tab to the new desk button inside expanded desks bar and press "Enter"
	// should create a new desk.
	desk2DeskNameView := nodewith.ClassName("DeskNameView").Nth(1)
	newDeskName := "new desk"
	if err := uiauto.Combine(
		"create a new desk through new desk button in the expanded desks bar",
		kb.AccelAction("Tab"),
		kb.AccelAction("Enter"),
		ac.WaitUntilExists(desk2DeskNameView),
		kb.TypeAction(newDeskName),
		kb.AccelAction("Enter"),
	)(ctx); err != nil {
		s.Fatal("Failed to create a new desk through expanded new desk button: ", err)
	}
	// Verifies that there 2 desks.
	twoDeskMiniViewsInfo, err := ash.FindDeskMiniViews(ctx, ac)
	if err != nil {
		s.Fatal("Failed to find desks: ", err)
	}
	if len(twoDeskMiniViewsInfo) != 2 {
		s.Fatalf("Got %v desks, want 2 desks", len(twoDeskMiniViewsInfo))
	}

	firstDeskMiniViewLoc := twoDeskMiniViewsInfo[0].Location
	secondDeskMiniViewLoc := twoDeskMiniViewsInfo[1].Location
	// Verifies that the new desk button inside the expanded desks bar has the
	// same height as the desk preview.
	newDeskButtonInExpandedDesksBarLoc, err := ac.Location(ctx, nodewith.ClassName("CrOSNextDeskIconButton"))
	if err != nil {
		s.Fatal("Failed to get the location of the new desk button inside expanded desks bar: ", err)
	}
	deskPreviewViewLocation, err := ac.Location(ctx, nodewith.ClassName("DeskPreviewView").First())
	if err != nil {
		s.Fatal("Failed to get location of desk preview view: ", err)
	}
	if (*newDeskButtonInExpandedDesksBarLoc).Height != deskPreviewViewLocation.Height {
		s.Fatalf("Got height of the expanded new desk button is %d, want %d", (*newDeskButtonInExpandedDesksBarLoc).Height, deskPreviewViewLocation.Height)
	}

	// Drag "new desk" to be the first desk and delete it.
	if err := pc.Drag(
		secondDeskMiniViewLoc.CenterPoint(),
		pc.DragTo(firstDeskMiniViewLoc.CenterPoint(), time.Second))(ctx); err != nil {
		s.Fatal("Failed to drag and drop desks: ", err)
	}
	closeDeskButton := nodewith.ClassName("CloseButton")
	if err := ac.LeftClick(closeDeskButton)(ctx); err != nil {
		s.Fatal("Failed to delete new desk: ", err)
	}

	// It should not switch back to zero state desk bar and should instead stay
	// expanded when there is only 1 desk. Click the new desk button inside the
	// zero state desks bar should create a new desk and expand the desks bar.
	addDeskButton := nodewith.ClassName("CrOSNextDeskIconButton")
	newDeskNameView := nodewith.ClassName("DeskNameView").First()
	if err := uiauto.Combine(
		"create a new desk from expanded state desks bar",
		ac.LeftClick(addDeskButton),
		// The focus on the new desk should be on the desk name field.
		ac.WaitUntilExists(newDeskNameView.Focused()),
		kb.TypeAction(newDeskName),
		kb.AccelAction("Enter"),
	)(ctx); err != nil {
		s.Fatal("Failed to create a new desk: ", err)
	}
	twoDeskMiniViewsInfoSecond, err := ash.FindDeskMiniViews(ctx, ac)
	if err != nil {
		s.Fatal("Failed to find desks: ", err)
	}
	if len(twoDeskMiniViewsInfoSecond) != 2 {
		s.Fatalf("Got %v desks, want 2 desks", len(twoDeskMiniViewsInfoSecond))
	}
}
