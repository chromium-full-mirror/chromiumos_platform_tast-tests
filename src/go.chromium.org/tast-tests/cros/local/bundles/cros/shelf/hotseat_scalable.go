// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package shelf

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         HotseatScalable,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests the basic features of hotseat",
		Contacts: []string{
			"chromeos-launcher@google.com",
			"chromeos-sw-engprod@google.com",
		},
		// ChromeOS > Software > System UI Surfaces > Shelf
		BugComponent: "b:1288352",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		SearchFlags: []*testing.StringPair{{
			Key:   "feature_id",
			Value: "screenplay-a64ba692-0294-456d-a714-5c4226b9c93c",
		}},
		Fixture: "chromeLoggedIn",
	})
}

// HotseatScalable verifies the launcher icon, pinned apps and status menu should be displayed in clamshell mode.
func HotseatScalable(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to get Test API connection: ", err)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	cleanup, err := ash.EnsureTabletModeEnabled(ctx, tconn, false)
	if err != nil {
		s.Fatal("Failed to ensure clamshell mode: ", err)
	}
	defer cleanup(cleanupCtx)

	ui := uiauto.New(tconn)

	if err := ash.WaitForHotseatAnimatingToIdealState(ctx, tconn, ash.ShelfShownClamShell); err != nil {
		s.Fatal("Failed to find launcher button: ", err)
	}

	resetPinState, err := ash.ResetShelfPinState(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get the function to reset pin states: ", err)
	}
	defer resetPinState(cleanupCtx)

	app := apps.FilesSWA

	if err := ash.PinApps(ctx, tconn, []string{app.ID}); err != nil {
		s.Fatal("Failed to pin apps to the shelf: ", err)
	}

	ids, err := ash.GetPinnedAppIds(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get pinned app ids: ", err)
	}

	var appExist bool
	for _, id := range ids {
		if app.ID == id {
			appExist = true
			break
		}
	}
	if !appExist {
		s.Fatal("Failed to find app in pinned list")
	}

	if err := uiauto.Combine("wait launcher and status menu exist",
		ui.WaitUntilExists(nodewith.Name("Launcher").HasClass("ash/HomeButton").Role(role.Button)),
		ui.WaitUntilExists(nodewith.HasClass("UnifiedSystemTray").Role(role.Button)),
	)(ctx); err != nil {
		s.Fatal("Failed to verify launcher or status menu: ", err)
	}
}
