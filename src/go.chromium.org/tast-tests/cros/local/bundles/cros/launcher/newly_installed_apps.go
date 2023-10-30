// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package launcher

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/cws"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/launcher"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const cwsAppID = "mljpablpddhocfbnokacjggdbmafjnon"
const cwsAppName = "Wicked Good Unarchiver"
const newInstallDescription = "New install"

func init() {
	testing.AddTest(&testing.Test{
		Func:         NewlyInstalledApps,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that newly installed apps are marked as such in launcher",
		Contacts: []string{
			"chromeos-launcher@google.com",
			"chromeos-sw-engprod@google.com",
		},
		BugComponent: "b:1288350",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		SearchFlags: []*testing.StringPair{{
			Key:   "feature_id",
			Value: "screenplay-90e4fecc-d2ea-40dc-b9db-eb9d61089e22",
		}},
		Params: []testing.Param{
			{
				Name:    "cws_clamshell_mode",
				Val:     launcher.TestCase{TabletMode: false},
				Fixture: "chromeLoggedInWithGaia",
			},
			{
				Name:              "cws_tablet_mode",
				Val:               launcher.TestCase{TabletMode: true},
				ExtraHardwareDeps: hwdep.D(hwdep.InternalDisplay()),
				Fixture:           "chromeLoggedInWithGaia",
			},
		},
	})
}

// NewlyInstalledApps checks that newly installed apps are marked as such in launcher.
func NewlyInstalledApps(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	tc := s.Param().(launcher.TestCase)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}

	if err := cws.InstallApp(ctx, cr.Browser(), tconn, cws.App{
		Name: cwsAppName,
		URL:  "https://chrome.google.com/webstore/detail/wicked-good-unarchiver/" + cwsAppID,
	}); err != nil {
		s.Fatal("Unable to install cws app: ", err)
	}
	defer closeAppAndUninstallViaSettings(ctx, cr, tconn, cwsAppName, cwsAppID)

	cleanup, err := launcher.SetUpLauncherTest(ctx, tconn, tc.TabletMode, true /*stabilizeAppCount*/)
	if err != nil {
		s.Fatal("Failed to set up launcher test case: ", err)
	}
	defer cleanup(cleanupCtx)

	defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree")

	view := appItemViewNode(cwsAppName, tc.TabletMode)

	if isNewInstall, err := isInNewInstallState(ctx, cr, tconn, view); err != nil {
		s.Fatal("Unable to compute new install state: ", err)
	} else if !isNewInstall {
		s.Fatalf("Unexpected new install state before launching the app; got %t, want %t", isNewInstall, true)
	}
	if err := launcher.HideLauncher(tconn, !tc.TabletMode)(ctx); err != nil {
		s.Fatal("Failed to hide launcher: ", err)
	}

	if err := launcher.LaunchAndWaitForAppOpen(tconn, apps.App{ID: cwsAppID, Name: cwsAppName})(ctx); err != nil {
		s.Fatal("Unable to launch the app: ", err)
	}

	if err := launcher.OpenProductivityLauncher(ctx, tconn, tc.TabletMode); err != nil {
		s.Fatal("Failed to open launcher: ", err)
	}
	if isNewInstall, err := isInNewInstallState(ctx, cr, tconn, view); err != nil {
		s.Fatal("Unable to compute new install state: ", err)
	} else if isNewInstall {
		s.Fatalf("Unexpected new install state after launching the app; got %t, want %t", isNewInstall, false)
	}
}

// appItemViewNode finds the app node ignoring the recent apps section.
func appItemViewNode(appName string, tabletMode bool) *nodewith.Finder {
	var ancestorNode *nodewith.Finder
	if tabletMode {
		ancestorNode = nodewith.ClassName(launcher.PagedAppsGridViewClass)
	} else {
		ancestorNode = nodewith.ClassName(launcher.BubbleAppsGridViewClass)
	}
	return launcher.AppItemViewFinder(appName).Ancestor(ancestorNode).First()
}

// isInNewInstallState computes if the app view is in new install state.
func isInNewInstallState(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn, view *nodewith.Finder) (bool, error) {
	ui := uiauto.New(tconn)
	viewInfo, err := ui.Info(ctx, view)
	if err != nil {
		return false, errors.Wrap(err, "failed to get app item view info")
	}
	return viewInfo.Description == newInstallDescription, nil
}

// closeAppAndUninstallViaSettings closes the app and uninstalls it via ossettings.
func closeAppAndUninstallViaSettings(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn, name, id string) error {
	isInstalled, err := ash.ChromeAppInstalled(ctx, tconn, id)
	if err != nil {
		return errors.Wrap(err, "failed to check app's existance")
	}

	if !isInstalled {
		testing.ContextLogf(ctx, "App %q is already uninstalled", name)
		return nil
	}

	// Some apps have "always on top" popup that prevents from clicking on correct nodes in Settings.
	if err := apps.Close(ctx, tconn, id); err != nil {
		return errors.Wrap(err, "failed to close the app")
	}

	defer func() {
		settings := ossettings.New(tconn)
		settings.Close(ctx)
	}()

	testing.ContextLogf(ctx, "Uninstall app: %q", name)
	if err := ossettings.UninstallApp(ctx, tconn, cr, name, id); err != nil {
		return errors.Wrap(err, "failed to uninstall the app via os settings")
	}

	return ash.WaitForChromeAppUninstalled(ctx, tconn, id, 30*time.Second)
}
