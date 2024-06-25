// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wmp

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/ui"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/optin"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/saveddesks"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

type dtTestParams struct {
	testApps       []apps.App
	closePlayStore bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         DesksTemplatesLaunch,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks desks templates can be launched",
		Contacts: []string{
			"chromeos-wms@google.com",
			"cros-commercial-productivity-eng@google.com",
			"chromeos-sw-engprod@google.com",
			"zhumatthew@google.com",
			"yzd@google.com",
		},
		// Chrome OS Server Projects > Enterprise Management > Commercial Productivity
		BugComponent: "b:1020793",
		Attr:         []string{"group:mainline"},
		SoftwareDeps: []string{"chrome", "chrome_internal", "no_kernel_upstream", "gaia"},
		Timeout:      chrome.GAIALoginTimeout + arc.BootTimeout + 180*time.Second,
		SearchFlags: []*testing.StringPair{{
			Key: "feature_id",
			// Launch workspace template.
			Value: "screenplay-2e1d03c8-d145-4fe5-b68f-04301d32199b",
		}},
		VarDeps: []string{ui.GaiaPoolDefaultVarName},
		Params: []testing.Param{{
			Fixture: "savedDesksEnableWithoutArc",
			Val:     dtTestParams{[]apps.App{apps.FilesSWA}, false},
		}, {
			Name:              "arc_enabled",
			Fixture:           "savedDesksEnableWithArc",
			Val:               dtTestParams{[]apps.App{apps.FilesSWA, apps.PlayStore}, true},
			ExtraSoftwareDeps: []string{"android_vm"},
			ExtraAttr:         []string{"informational"},
		}},
	})
}

func DesksTemplatesLaunch(ctx context.Context, s *testing.State) {
	// TODO(b/238645466): Remove `no_kernel_upstream` from SoftwareDeps once kernel_uprev boards are more stable.
	// Reserve five seconds for various cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	cr := s.FixtValue().(*saveddesks.SavedDeskFixtData).Chrome

	// Set up the apps to launch list and close play store flag.
	appsList := s.Param().(dtTestParams).testApps
	closePlayStore := s.Param().(dtTestParams).closePlayStore

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

	// Enter overview mode.
	if err := ash.SetOverviewModeAndWait(ctx, tconn, true); err != nil {
		s.Fatal("Failed to set overview mode: ", err)
	}

	// Enter library page.
	if err := ash.EnterLibraryPage(ctx, ac); err != nil {
		s.Fatal("Failed to enter library page: ", err)
	}

	// Launch saved desk `Template 1` of type `Template`.
	if err := ash.LaunchSavedDesk(ctx, ac, "Template 1", 0); err != nil {
		s.Fatal("Failed to launch saved desk 'Template 1' of type 'Template': ", err)
	}

	// Wait for apps to launch.
	if err := saveddesks.WaitforAppsToLaunch(ctx, tconn, ac, appsList); err != nil {
		s.Fatal("Failed to wait for apps to launch: ", err)
	}

	// Wait for apps to be visible.
	if err := saveddesks.WaitforAppsToBeVisible(ctx, tconn, ac, appsList); err != nil {
		s.Fatal("Failed to wait for apps to be visible: ", err)
	}

	// Exit overview mode.
	if err := ash.SetOverviewModeAndWait(ctx, tconn, false); err != nil {
		s.Fatal("Failed to set overview mode: ", err)
	}

	// If Play Store is an included in the app launch list then close Play Store.
	if closePlayStore {
		if err := optin.ClosePlayStore(ctx, tconn); err != nil {
			s.Fatal("Failed to close Play Store: ", err)
		}
	}

	// Close all existing windows.
	if err := ash.CloseAllWindows(ctx, tconn); err != nil {
		s.Fatal("Failed to close all windows: ", err)
	}

	// Remove the active desk.
	if err := ash.RemoveActiveDesk(ctx, tconn); err != nil {
		s.Fatal("Failed to remove the active desk: ", err)
	}

	// Enter overview mode.
	if err := ash.SetOverviewModeAndWait(ctx, tconn, true); err != nil {
		s.Fatal("Failed to set overview mode: ", err)
	}

	// Enter library page.
	if err := ash.EnterLibraryPage(ctx, ac); err != nil {
		s.Fatal("Failed to enter library page: ", err)
	}

	// Verify saved desk.
	if err := ash.VerifySavedDesk(ctx, ac, []string{"Template 1", "Saved Desk 1"}); err != nil {
		s.Fatal("Failed to verify saved desk: ", err)
	}

	// Launch saved desk `Saved Desk 1` of type `SaveAndRecall`.
	if err := ash.LaunchSavedDesk(ctx, ac, "Saved Desk 1", 1); err != nil {
		s.Fatal("Failed to launch saved desk 'Saved Desk 1' of type 'SaveAndRecall': ", err)
	}

	for _, app := range appsList {
		if err := ash.WaitForApp(ctx, tconn, app.ID, time.Minute); err != nil {
			s.Fatal("App did not appear in shelf after launch: ", err)
		}
	}
	// Exit overview mode.
	if err := ash.SetOverviewModeAndWait(ctx, tconn, false); err != nil {
		s.Fatal("Failed to set overview mode: ", err)
	}

	if err := saveddesks.VerifyWindowCount(ctx, tconn, len(appsList)); err != nil {
		s.Fatal("Failed to verify window count: ", err)
	}

	// If Play Store is an included in the app launch list then close Play Store.
	if closePlayStore {
		if err := optin.ClosePlayStore(ctx, tconn); err != nil {
			s.Fatal("Failed to close Play Store: ", err)
		}
	}

	// Close all existing windows.
	if err := ash.CloseAllWindows(ctx, tconn); err != nil {
		s.Fatal("Failed to close all windows: ", err)
	}

	// Remove the active desk.
	if err := ash.RemoveActiveDesk(ctx, tconn); err != nil {
		s.Fatal("Failed to remove the active desk: ", err)
	}

	// Enter overview mode.
	if err := ash.SetOverviewModeAndWait(ctx, tconn, true); err != nil {
		s.Fatal("Failed to set overview mode: ", err)
	}

	// Enter library page.
	if err := ash.EnterLibraryPage(ctx, ac); err != nil {
		s.Fatal("Failed to enter library page: ", err)
	}

	// Verify saved desk.
	if err := ash.VerifySavedDesk(ctx, ac, []string{"Template 1"}); err != nil {
		s.Fatal("Failed to verify saved desk: ", err)
	}

	// Exit overview mode.
	if err := ash.SetOverviewModeAndWait(ctx, tconn, false); err != nil {
		s.Fatal("Failed to exit overview mode: ", err)
	}
}
