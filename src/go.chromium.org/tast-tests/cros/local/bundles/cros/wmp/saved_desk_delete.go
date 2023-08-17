// Copyright 2023 The ChromiumOS Authors
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
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/saveddesks"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SavedDeskDelete,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks saved desk can be deleted",
		Contacts: []string{
			"chromeos-wmp@google.com",
			"cros-commercial-productivity-eng@google.com",
			"chromeos-sw-engprod@google.com",
			"yongshun@chromium.org",
			"yzd@google.com",
			"zhumatthew@google.com",
		},
		// Chrome OS Server Projects > Enterprise Management > Commercial Productivity
		BugComponent: "b:1020793",
		Attr:         []string{"group:mainline"},
		SoftwareDeps: []string{"chrome", "no_kernel_upstream"},
		Timeout:      chrome.GAIALoginTimeout + arc.BootTimeout + 180*time.Second,
		SearchFlags: []*testing.StringPair{{
			Key: "feature_id",
			// Delete saved desk
			Value: "screenplay-7f6d909c-3450-4ec0-ab4d-a3f704ec3e7f",
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

func SavedDeskDelete(ctx context.Context, s *testing.State) {
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
	// Enter overview mode.
	if err := ash.SetOverviewModeAndWait(ctx, tconn, true); err != nil {
		s.Fatal("Failed to enter overview mode: ", err)
	}

	defer ash.SetOverviewModeAndWait(cleanupCtx, tconn, false)

	// Wait for saved desk sync.
	ash.WaitForSavedDeskSync(ctx, ac)

	// Exit overview mode.
	if err := ash.SetOverviewModeAndWait(ctx, tconn, false); err != nil {
		s.Fatal("Failed to exit overview mode: ", err)
	}
	// Open PlayStore, Chrome and Files.
	browserApp, err := apps.PrimaryBrowser(ctx, tconn)
	if err != nil {
		s.Fatal("Could not find the primary browser app info: ", err)
	}
	appsList = append(appsList, browserApp)
	if err := saveddesks.OpenApps(ctx, tconn, ac, appsList); err != nil {
		s.Fatal("Failed to open apps: ", err)
	}

	// Enter overview mode.
	if err := ash.SetOverviewModeAndWait(ctx, tconn, true); err != nil {
		s.Fatal("Failed to enter overview mode: ", err)
	}

	// Save current desk as `Saved Desk 1` of type `SaveAndRecall`.
	savedDeskName := "Saved Desk 1"
	if err := ash.SaveCurrentDesk(ctx, ac, ash.SaveAndRecall, savedDeskName); err != nil {
		s.Fatal("Failed to save current desk as 'Saved Desk 1' of type 'SaveAndRecall': ", err)
	}

	// Exit overview mode.
	if err := ash.SetOverviewModeAndWait(ctx, tconn, false); err != nil {
		s.Fatal("Failed to exit overview mode: ", err)
	}

	// Check all windows are closed after saving the save and recall template.
	if err := saveddesks.VerifyWindowCount(ctx, tconn, 0); err != nil {
		s.Fatal("Failed to verify window count: ", err)
	}

	// Enter overview mode.
	if err := ash.SetOverviewModeAndWait(ctx, tconn, true); err != nil {
		s.Fatal("Failed to enter overview mode: ", err)
	}

	// Enter library page.
	if err := ash.EnterLibraryPage(ctx, ac); err != nil {
		s.Fatal("Failed to enter library page: ", err)
	}

	// Additional desks may be synced to this test device. Verify that there is at least one saved desk.
	savedDeskViewInfo, err := ash.FindSavedDesks(ctx, ac)
	if err != nil {
		s.Fatal("Failed to find saved desks: ", err)
	}
	if len(savedDeskViewInfo) < 1 {
		s.Fatalf("Found inconsistent number of desks(s): got %v, want 1 or more", len(savedDeskViewInfo))
	}

	if err := ash.DeleteDeskTemplateByName(ctx, ac, tconn, savedDeskName); err != nil {
		s.Fatal("Failed to delete save and recall desk template: ", err)
	}

	// Exit overview mode.
	if err := ash.SetOverviewModeAndWait(ctx, tconn, false); err != nil {
		s.Fatal("Failed to exit overview mode: ", err)
	}

}
