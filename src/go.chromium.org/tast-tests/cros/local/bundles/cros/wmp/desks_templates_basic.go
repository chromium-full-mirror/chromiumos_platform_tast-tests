// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wmp

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/event"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/saveddesks"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DesksTemplatesBasic,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks desks can be saved as a desk template",
		Contacts: []string{
			"chromeos-wmp@google.com",
			"cros-commercial-productivity-eng@google.com",
			"chromeos-sw-engprod@google.com",
			"zhumatthew@google.com",
			"yzd@google.com",
		},
		// Chrome OS Server Projects > Enterprise Management > Commercial Productivity
		BugComponent: "b:1020793",
		Attr:         []string{"group:mainline"},
		SoftwareDeps: []string{"chrome", "no_kernel_upstream"},
		Timeout:      chrome.GAIALoginTimeout + arc.BootTimeout + 180*time.Second,
		SearchFlags: []*testing.StringPair{{
			Key: "feature_id",
			// Setup workspace templates.
			Value: "screenplay-a28f92cc-e3f3-47e7-8234-f7508f7722fe",
		}},
		VarDeps: []string{"ui.gaiaPoolDefault"},
		Params: []testing.Param{{
			Fixture: "savedDesksEnableWithoutArc",
			Val:     []apps.App{apps.FilesSWA},
		}, {
			Name:              "lacros",
			Fixture:           "savedDesksEnabledLacrosWithArcBooted",
			Val:               []apps.App{apps.FilesSWA, apps.PlayStore},
			ExtraSoftwareDeps: []string{"lacros", "android_vm"},
			ExtraAttr:         []string{"informational"},
		}, {
			Name:              "arc_enabled",
			Fixture:           "savedDesksEnableWithArc",
			Val:               []apps.App{apps.FilesSWA, apps.PlayStore},
			ExtraSoftwareDeps: []string{"android_vm"},
			ExtraAttr:         []string{"informational"},
		}},
	})
}

func DesksTemplatesBasic(ctx context.Context, s *testing.State) {
	// TODO(b/238645466): Remove `no_kernel_upstream` from SoftwareDeps once kernel_uprev boards are more stable.
	// Reserve five seconds for various cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	cr := s.FixtValue().(*saveddesks.SavedDeskFixtData).Chrome

	// Set up the apps to launch list.
	appsList := s.Param().([]apps.App)

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
	s.AttachErrorHandlers(
		func(errMsg string) {
			faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree_error")
		},
		func(errMsg string) {
			faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree_fatal")
		})

	// Close all existing windows.
	if err := ash.CloseAllWindows(ctx, tconn); err != nil {
		s.Fatal("Failed to close all windows: ", err)
	}
	ac := uiauto.New(tconn)
	// Delete all existing saved desks.
	if err := saveddesks.DeleteSavedDesks(cleanupCtx, tconn, ac); err != nil {
		s.Fatal("Failed to delete saved desks: ", err)
	}
	// Opens PlayStore, Browser and Files.
	browserApp, err := apps.PrimaryBrowser(ctx, tconn)
	if err != nil {
		s.Fatal("Could not find the primary browser app info: ", err)
	}
	appsList = append(appsList, browserApp)

	if err := saveddesks.OpenApps(ctx, tconn, ac, appsList); err != nil {
		s.Fatal("Failed to open apps: ", err)
	}

	// TODO(b/296141011): Disable the customize chrome pop up on a browser launch instead of waiting for the popup event to be over.
	if err := ac.WithInterval(5*time.Second).WaitUntilNoEvent(nodewith.Root(), event.HaspopupChanged)(ctx); err != nil {
		s.Fatal("Failed to wait for other popup events to be completed: ", err)
	}

	// Enter overview mode.
	if err := ash.SetOverviewModeAndWait(ctx, tconn, true); err != nil {
		s.Fatal("Failed to set overview mode: ", err)
	}

	defer ash.SetOverviewModeAndWait(cleanupCtx, tconn, false)

	// Save current desk as `Template 1` of type `Template`.
	if err := ash.SaveCurrentDesk(ctx, ac, ash.Template, "Template 1"); err != nil {
		s.Fatal("Failed to save current desk as 'Template 1' of type 'Template': ", err)
	}

	defer saveddesks.DeleteSavedDesks(cleanupCtx, tconn, ac)

	// Verify saved desk.
	if err := ash.VerifySavedDesk(ctx, ac, []string{"Template 1"}); err != nil {
		s.Fatal("Failed to verify saved desk: ", err)
	}

	// Exit overview mode.
	if err := ash.SetOverviewModeAndWait(ctx, tconn, false); err != nil {
		s.Fatal("Failed to set overview mode: ", err)
	}

	// Verify window count.
	if err := saveddesks.VerifyWindowCount(ctx, tconn, len(appsList)); err != nil {
		s.Fatal("Failed to verify window count: ", err)
	}

	// Enter overview mode.
	if err := ash.SetOverviewModeAndWait(ctx, tconn, true); err != nil {
		s.Fatal("Failed to set overview mode: ", err)
	}

	// Save current desk as `Saved Desk 1` of type `SaveAndRecall`.
	if err := ash.SaveCurrentDesk(ctx, ac, ash.SaveAndRecall, "Saved Desk 1"); err != nil {
		s.Fatal("Failed to save current desk as 'Saved Desk 1' of type 'SaveAndRecall': ", err)
	}

	// Exit and reenter library page.
	if err := ash.ExitAndReenterLibrary(ctx, ac, tconn); err != nil {
		s.Fatal("Failed to exit and reenter library page: ", err)
	}

	// Verify saved desk.
	if err := ash.VerifySavedDesk(ctx, ac, []string{"Template 1", "Saved Desk 1"}); err != nil {
		s.Fatal("Failed to verify saved desk: ", err)
	}

	// Exit overview mode.
	if err := ash.SetOverviewModeAndWait(ctx, tconn, false); err != nil {
		s.Fatal("Failed to set overview mode: ", err)
	}

	// Verify window count.
	if err := saveddesks.VerifyWindowCount(ctx, tconn, 0); err != nil {
		s.Fatal("Failed to verify window count: ", err)
	}

	// Exit and reenter library page.
	if err := ash.ExitAndReenterLibrary(ctx, ac, tconn); err != nil {
		s.Fatal("Failed to exit and reenter library page: ", err)
	}

	// Verify that there are two saved desks.
	savedDeskViewInfo, err := ash.FindSavedDesks(ctx, ac)
	if err != nil {
		s.Fatal("Failed to find saved desks: ", err)
	}
	if len(savedDeskViewInfo) != 2 {
		s.Fatalf("Found inconsistent number of desks(s): got %v, want 2", len(savedDeskViewInfo))
	}

	// Exit overview mode.
	if err := ash.SetOverviewModeAndWait(ctx, tconn, false); err != nil {
		s.Fatal("Failed to exit overview mode: ", err)
	}

	// Verify that the app windows are closed.
	ws, err := ash.GetAllWindows(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to count windows: ", err)
	}
	if len(ws) != 0 {
		for _, window := range ws {
			s.Log("Found window ", window)
		}
		s.Fatalf("Found inconsistent number of window(s): got %v, want 0", len(ws))
	}
}
