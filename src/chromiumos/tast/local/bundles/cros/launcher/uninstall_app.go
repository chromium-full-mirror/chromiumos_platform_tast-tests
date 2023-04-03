// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package launcher

import (
	"context"
	"time"

	"chromiumos/tast/common/android/ui"
	"chromiumos/tast/ctxutil"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/apps"
	"chromiumos/tast/local/arc"
	"chromiumos/tast/local/arc/apputil"
	"chromiumos/tast/local/arc/playstore"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/ash"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/cws"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/chrome/uiauto/launcher"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/ossettings"
	"chromiumos/tast/local/chrome/uiauto/role"
	"chromiumos/tast/testing"
)

type appSource int

const (
	fromCWS appSource = iota
	fromPlayStore
)

type appInfo struct {
	name         string
	id           string
	source       appSource
	isTabletMode bool

	cwsApp cws.App

	arcAppPkgName string
}

const installationTimeout = apputil.InstallationTimeout

func init() {
	testing.AddTest(&testing.Test{
		Func:         UninstallApp,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies app can be deleted from the list of app in launcher",
		Contacts: []string{
			"cros-system-ui-eng@google.com",
			"yulunwu@chromium.org",
			"chromeos-sw-engprod@google.com",
		},
		BugComponent: "b:1288350",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome", "chrome_internal"},
		Timeout:      3*time.Minute + installationTimeout,
		Params: []testing.Param{
			{
				Name: "cws",
				Val: &appInfo{
					name:         "Wicked Good Unarchiver",
					id:           "mljpablpddhocfbnokacjggdbmafjnon",
					source:       fromCWS,
					isTabletMode: false,
					cwsApp: cws.App{
						Name: "Wicked Good Unarchiver",
						URL:  "https://chrome.google.com/webstore/detail/wicked-good-unarchiver/mljpablpddhocfbnokacjggdbmafjnon",
					},
				},
				Fixture: "chromeLoggedInWithGaia",
			},
			{
				Name: "cws_tablet",
				Val: &appInfo{
					name:         "Wicked Good Unarchiver",
					id:           "mljpablpddhocfbnokacjggdbmafjnon",
					source:       fromCWS,
					isTabletMode: true,
					cwsApp: cws.App{
						Name: "Wicked Good Unarchiver",
						URL:  "https://chrome.google.com/webstore/detail/wicked-good-unarchiver/mljpablpddhocfbnokacjggdbmafjnon",
					},
				},
				Fixture: "chromeLoggedInWithGaia",
			},
			{
				Name: "arc",
				Val: &appInfo{
					name:          apps.Chat.Name,
					id:            apps.Chat.ID,
					source:        fromPlayStore,
					isTabletMode:  false,
					arcAppPkgName: "com.google.android.apps.dynamite",
				},
				Fixture:           "arcBootedWithPlayStore",
				ExtraSoftwareDeps: []string{"arc"},
			},
			{
				Name: "arc_tablet",
				Val: &appInfo{
					name:          apps.Chat.Name,
					id:            apps.Chat.ID,
					source:        fromPlayStore,
					isTabletMode:  true,
					arcAppPkgName: "com.google.android.apps.dynamite",
				},
				Fixture:           "arcBootedWithPlayStore",
				ExtraSoftwareDeps: []string{"arc"},
			},
		},
	})
}

// UninstallApp verifies app can be deleted from the list of app in launcher.
func UninstallApp(ctx context.Context, s *testing.State) {
	app := s.Param().(*appInfo)

	var cr *chrome.Chrome
	var a *arc.ARC
	var device *ui.Device

	switch app.source {
	case fromCWS:
		cr = s.FixtValue().(chrome.HasChrome).Chrome()
	case fromPlayStore:
		cr = s.FixtValue().(*arc.PreData).Chrome
		a = s.FixtValue().(*arc.PreData).ARC
		device = s.FixtValue().(*arc.PreData).UIDevice
	default:
		s.Fatal("Unexpected app source: ", app.source)
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create test API connection: ", err)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	if err := installApp(ctx, cr, tconn, a, device, app); err != nil {
		s.Fatalf("Failed to install app %s: %v", app.name, err)
	}
	// Ensure cleanup the app if test case fail before uninstalling it.
	defer uninstall(cleanupCtx, cr, tconn, app)
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "uninstall_app_from_launcher")

	cleanup, err := launcher.SetUpLauncherTest(ctx, tconn, app.isTabletMode, true /*stabilizeAppCount*/)
	if err != nil {
		s.Fatal("Failed to set up launcher test case: ", err)
	}
	defer cleanup(cleanupCtx)

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "uninstall_app_from_launcher_after_test_setup")

	ui := uiauto.New(tconn)

	allAppsRoot := nodewith.Ancestor(nodewith.Role(role.Group).NameContaining("All Apps"))
	appItem := allAppsRoot.Name(app.name).HasClass(launcher.ExpandedItemsClass)

	if err := launcher.UninstallsAppUsingContextMenu(ctx, tconn, appItem); err != nil {
		s.Fatal("Failed to delete app in launcher: ", err)
	}

	if err := ui.EnsureGoneFor(nodewith.Name(app.name).HasClass(launcher.ExpandedItemsClass), 10*time.Second)(ctx); err != nil {
		s.Fatalf("The app %q still remains in launcher: %v", app.name, err)
	}
}

// installApp installs the app and returns an error indicating if the app has been successfully installed.
func installApp(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn, arcSession *arc.ARC, arcDevice *ui.Device, app *appInfo) error {
	testing.ContextLog(ctx, "Installing app")

	switch app.source {
	case fromCWS:
		// The cws has already sets the installation timeout, use the ctx directly.
		if err := cws.InstallApp(ctx, cr.Browser(), tconn, app.cwsApp); err != nil {
			return errors.Wrapf(err, "failed to install %s", app.name)
		}
	case fromPlayStore:
		if arcSession == nil {
			return errors.New("arc session not provided")
		}
		if arcDevice == nil {
			return errors.New("arc device not provided")
		}
		if err := playstore.InstallOrUpdateAppAndClose(ctx, tconn, arcSession, arcDevice, app.arcAppPkgName, &playstore.Options{TryLimit: -1, InstallationTimeout: installationTimeout}); err != nil {
			return errors.Wrapf(err, "failed to install %s", app.name)
		}
	default:
		return errors.Errorf("unexpected app source %v", app.source)
	}

	return nil
}

// uninstall uninstalls the app via ossettings.
func uninstall(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn, app *appInfo) error {
	isInstalled, err := ash.ChromeAppInstalled(ctx, tconn, app.id)
	if err != nil {
		return errors.Wrap(err, "failed to check app's existance")
	}

	if !isInstalled {
		return nil
	}

	defer func(ctx context.Context) {
		settings := ossettings.New(tconn)
		settings.Close(ctx)
	}(ctx)
	testing.ContextLogf(ctx, "Uninstall app: %q", app.name)
	return ossettings.UninstallApp(ctx, tconn, cr, app.name, app.id)
}
