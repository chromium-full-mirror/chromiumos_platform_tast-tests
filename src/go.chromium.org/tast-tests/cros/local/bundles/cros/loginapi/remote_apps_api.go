// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package loginapi

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/launcher"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/mgs"
	"go.chromium.org/tast-tests/cros/local/session"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: RemoteAppsAPI,
		Desc: "Test chrome.enterprise.remoteApps Extension API",
		Contacts: []string{
			"chromeos-commercial-identity@google.com",
			"mpetrisor@chromium.org",
		},
		BugComponent: "b:1253162", // Using the same as login_screen_storage_api
		Attr: []string{
			"group:mainline",
			"informational",
		},
		SoftwareDeps: []string{"reboot", "chrome"},
		Fixture:      fixture.FakeDMSEnrolled,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DeviceLoginScreenExtensions{}, pci.VerifiedFunctionalityJS),
			pci.SearchFlag(&policy.ExtensionInstallForcelist{}, pci.VerifiedFunctionalityJS),
			{
				Key: "feature_id",
				// Launch healthcare application on device (COM_HEALTH_CUJ1_TASK2_WF1).
				Value: "screenplay-446812cc-07af-4094-bfb2-00150301ede3",
			},
		},
	})
}

// remoteAppsAPIFuncs provides a helper to call chrome.enterprise.remoteApps
// API functions, wrapping them in a Promise and handling chrome.runtime.lastError.
const remoteAppsAPIFuncs = `
  self.callRemoteAppsAPI = (funcName, ...args) => {
    return new Promise((resolve, reject) => {
      const apiFunc = chrome.enterprise.remoteApps[funcName];
      if (!apiFunc) {
        reject(new Error(` + "`API function ${funcName} not found`" + `));
        return;
      }

      // The last argument to the API call is the callback
      const callback = (result) => {
        if (chrome.runtime.lastError) {
          reject(new Error(chrome.runtime.lastError.message));
          return;
        }
        resolve(result);
      };

      // Call the API function
      apiFunc.apply(chrome.enterprise.remoteApps, [...args, callback]);
    });
  };
`

func RemoteAppsAPI(ctx context.Context, s *testing.State) {
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	// Setup MGS Policies and Start Chrome.
	accountID := "foo@managedchrome.com"

	opts := []mgs.Option{
		mgs.Accounts(accountID),
		mgs.AddPublicAccountPolicies(accountID, []policy.Policy{
			// This policy installs the in-session extension.
			&policy.ExtensionInstallForcelist{Val: []string{mgs.InSessionExtensionID}},
		}),
		mgs.ExtraPolicies([]policy.Policy{
			// This policy installs the login-screen extension.
			&policy.DeviceLoginScreenExtensions{Val: []string{mgs.LoginScreenExtensionID}},
		}),
		mgs.ExtraChromeOptions(
			chrome.ExtraArgs("--force-devtools-available"),
		),
	}

	m, cr, err := mgs.New(ctx, fdms, opts...)
	if err != nil {
		s.Fatal("Failed to start Chrome on Signin screen with MGS accounts: ", err)
	}
	defer func() {
		if err := m.Close(ctx); err != nil {
			s.Fatal("Failed close MGS: ", err)
		}
	}()

	sm, err := session.NewSessionManager(ctx)
	if err != nil {
		s.Fatal("Failed to connect to session manager: ", err)
	}

	sw, err := sm.WatchSessionStateChanged(ctx, "started")
	if err != nil {
		s.Fatal("Failed to watch for D-Bus signals: ", err)
	}
	defer sw.Close(ctx)

	// Launch MGS using the Login Screen Extension.
	// We must connect to the login screen extension to trigger the MGS launch.
	conn, err := cr.NewConnForTarget(ctx, chrome.MatchTargetURL(mgs.LoginScreenExtensionURL))
	if err != nil {
		s.Fatal("Failed to connect to login screen extension: ", err)
	}
	defer conn.Close()

	// Wait for the login API to become available.
	if err = conn.WaitForExpr(ctx, `chrome.login !== undefined`); err != nil {
		conn.Close()
		s.Fatal("Failed to wait for the login API to be available: ", err)
	}

	// Call the API to launch the session.
	if err := conn.Eval(ctx, `new Promise((resolve, reject) => {
		chrome.login.launchManagedGuestSession(() => {
			if (chrome.runtime.lastError) {
				reject(new Error(chrome.runtime.lastError.message));
				return;
			}
			resolve();
		});
	})`, nil); err != nil {
		s.Fatal("Failed to launch MGS: ", err)
	}

	// Wait for the session to be fully started.
	select {
	case <-sw.Signals:
		// Session started successfully.
	case <-ctx.Done():
		s.Fatal("Timeout before getting SessionStateChanged signal: ", err)
	}

	// Connect to In-Session Extension and Setup Helpers.
	inSessionConn, err := cr.NewConnForTarget(ctx, chrome.MatchTargetURL(mgs.InSessionExtensionURL))
	if err != nil {
		s.Fatal("Failed to connect to in-session extension: ", err)
	}
	defer inSessionConn.Close()

	// Wait for the remoteApps API to become available.
	if err = inSessionConn.WaitForExpr(ctx, `chrome.enterprise.remoteApps !== undefined`); err != nil {
		inSessionConn.Close()
		s.Fatal("Failed to wait for the remoteApps API to be available: ", err)
	}

	// Inject the Promise-wrapping helper function into the extension's context.
	if err := inSessionConn.Eval(ctx, remoteAppsAPIFuncs, nil); err != nil {
		s.Fatal("Failed to inject API helper functions: ", err)
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	// Helper function to verify if an app is present in the launcher.
	verifyAppPresence := func(appID string, present bool) {
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			apps, err := ash.AppsInLauncher(ctx, tconn)
			if err != nil {
				return testing.PollBreak(err)
			}
			found := false
			for _, app := range apps {
				if app.AppID == appID {
					found = true
					break
				}
			}

			if present && !found {
				return errors.New("app not found in launcher")
			}
			if !present && found {
				return errors.New("app still in launcher")
			}
			return nil
		}, &testing.PollOptions{Timeout: 15 * time.Second}); err != nil {
			s.Fatalf("Failed to verify app presence for %s (present=%t): %v", appID, present, err)
		}
	}

	// Define UI interaction elements and actions once.
	ui := uiauto.New(tconn)
	showLauncher := launcher.ShowLauncher(tconn, true)
	hideLauncher := launcher.HideLauncher(tconn, true)

	// Define app/folder names.
	const folderName = "Clinical Apps"
	const appInFolderName = "Patient Records"
	const topLevelAppName = "Telehealth Portal"
	const frontAppName = "Imaging Viewer"
	const iconAppName = "Cardiology EMR"

	// A 1x1 red GIF to use as a test icon.
	const testIconDataURL = "data:image/gif;base64,R0lGODlhAQABAPAAAP8AACwAAAAAAQABAAACAkQBADs="

	folderItemName := "Folder " + folderName // How folders appear in the launcher.
	folderFinder := nodewith.ClassName(launcher.ExpandedItemsClass).Name(folderItemName)

	appsGridFinder := nodewith.ClassName("ScrollableAppsGridView")
	appItemFinder := nodewith.ClassName("AppListItemView").
		Role(role.Button).
		Ancestor(appsGridFinder)

	// Store app/folder IDs for later use.
	var folderID, appIDInFolder, topLevelAppID, frontAppID, iconAppID string

	// Run API Tests Sequentially.
	s.Run(ctx, "AddFolderAndApp", func(ctx context.Context, s *testing.State) {
		// Add a folder.
		if err := inSessionConn.Call(ctx, &folderID, `self.callRemoteAppsAPI`, "addFolder",
			map[string]string{"name": folderName}); err != nil {
			s.Fatal("Failed to add folder: ", err)
		}
		if folderID == "" {
			s.Fatal("addFolder did not return a folderId")
		}

		// Add an app inside the folder.
		if err := inSessionConn.Call(ctx, &appIDInFolder, `self.callRemoteAppsAPI`, "addApp",
			map[string]string{"name": appInFolderName, "folderId": folderID}); err != nil {
			s.Fatal("Failed to add app in folder: ", err)
		}
		if appIDInFolder == "" {
			s.Fatal("addApp in folder did not return an appId")
		}
		verifyAppPresence(appIDInFolder, true)

		// Verify the folder is visible in the launcher.
		if err := showLauncher(ctx); err != nil {
			s.Fatal("Failed to show launcher: ", err)
		}
		if err := ui.WaitUntilExists(folderFinder)(ctx); err != nil {
			s.Fatal("Failed to find folder in launcher: ", err)
		}
		if err := hideLauncher(ctx); err != nil {
			s.Log("Failed to hide launcher: ", err)
		}
	})

	s.Run(ctx, "AddTopLevelApp", func(ctx context.Context, s *testing.State) {
		if err := inSessionConn.Call(ctx, &topLevelAppID, `self.callRemoteAppsAPI`, "addApp",
			map[string]string{"name": topLevelAppName}); err != nil {
			s.Fatal("Failed to add top level app: ", err)
		}
		if topLevelAppID == "" {
			s.Fatal("addApp at top level did not return an appId")
		}
		verifyAppPresence(topLevelAppID, true)
	})

	s.Run(ctx, "AddAppWithIcon", func(ctx context.Context, s *testing.State) {
		if err := inSessionConn.Call(ctx, &iconAppID, `self.callRemoteAppsAPI`, "addApp",
			map[string]string{
				"name":    iconAppName,
				"iconUrl": testIconDataURL,
			}); err != nil {
			s.Fatal("Failed to add app with icon: ", err)
		}
		if iconAppID == "" {
			s.Fatal("addApp with icon did not return an appId")
		}

		// Verify app exists in the backend.
		verifyAppPresence(iconAppID, true)

		// Verify app is visible in the launcher UI.
		if err := showLauncher(ctx); err != nil {
			s.Fatal("Failed to show launcher: ", err)
		}
		if err := ui.WaitUntilExists(appsGridFinder)(ctx); err != nil {
			s.Fatal("Failed to find the apps grid: ", err)
		}

		// Find the app button itself.
		iconAppFinder := nodewith.Name(iconAppName).
			Role(role.Button).
			Ancestor(appsGridFinder)

		if err := ui.WaitUntilExists(iconAppFinder)(ctx); err != nil {
			s.Fatalf("Failed to find app %q in launcher: %v", iconAppName, err)
		}

		if err := hideLauncher(ctx); err != nil {
			s.Log("Failed to hide launcher: ", err)
		}
	})

	s.Run(ctx, "SetPinnedApps", func(ctx context.Context, s *testing.State) {
		if err := inSessionConn.Call(ctx, nil, `self.callRemoteAppsAPI`, "setPinnedApps",
			[]string{topLevelAppID}); err != nil {
			s.Fatal("Failed to set pinned apps: ", err)
		}

		// Verify the app is pinned to the shelf (which is inside the HomeButton).
		homeButtonFinder := nodewith.ClassName("HomeButton").Name("Launcher").Role(role.Button)
		quickAppButtonFinder := nodewith.Name(topLevelAppName).
			ClassName("ImageButton").
			Role(role.Button).
			Ancestor(homeButtonFinder)

		if err := ui.WaitUntilExists(quickAppButtonFinder)(ctx); err != nil {
			s.Fatalf("Failed to find the pinned %q button on the shelf: %v", topLevelAppName, err)
		}
	})

	s.Run(ctx, "OnRemoteAppLaunched", func(ctx context.Context, s *testing.State) {
		// Add a listener for the onRemoteAppLaunched event.
		if err := inSessionConn.Eval(ctx, `
				chrome.enterprise.remoteApps.onRemoteAppLaunched.addListener(appId => {
					 self.launchedApp = appId;
				});
		`, nil); err != nil {
			s.Fatal("Failed to add listener for onRemoteAppLaunched: ", err)
		}

		// Launch the app from the UI to trigger the event.
		if err := launcher.LaunchApp(tconn, topLevelAppName)(ctx); err != nil {
			s.Fatal("Failed to launch app: ", err)
		}

		// Verify the listener received the correct app ID.
		var launchedAppID string
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			if err := inSessionConn.Eval(ctx, `self.launchedApp`, &launchedAppID); err != nil {
				return testing.PollBreak(err)
			}
			if launchedAppID == "" {
				return errors.New("launchedAppID not set yet")
			}
			return nil
		}, &testing.PollOptions{Timeout: 10 * time.Second}); err != nil {
			s.Fatal("Failed to get launched app ID from listener: ", err)
		}

		if launchedAppID != topLevelAppID {
			s.Fatalf("Expected launched app ID to be %s, but got %s", topLevelAppID, launchedAppID)
		}
	})

	s.Run(ctx, "AddAppToFront", func(ctx context.Context, s *testing.State) {
		// Add an app with addToFront: true.
		if err := inSessionConn.Call(ctx, &frontAppID, `self.callRemoteAppsAPI`, "addApp",
			map[string]interface{}{"name": frontAppName, "addToFront": true}); err != nil {
			s.Fatal("Failed to add app with addToFront: ", err)
		}
		if frontAppID == "" {
			s.Fatal("addApp with addToFront did not return an appId")
		}
		verifyAppPresence(frontAppID, true)

		// Verify it's the first app in the launcher grid.
		if err := showLauncher(ctx); err != nil {
			s.Fatal("Failed to show launcher: ", err)
		}
		if err := ui.WaitUntilExists(appsGridFinder)(ctx); err != nil {
			s.Fatal("Failed to find the apps grid: ", err)
		}

		// Poll to wait for the app to appear at the front.
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			appItems, err := ui.NodesInfo(ctx, appItemFinder)
			if err != nil {
				return testing.PollBreak(errors.Wrap(err, "failed to get app items info"))
			}
			if len(appItems) == 0 {
				return errors.New("no apps found in the launcher grid")
			}

			actualFirstAppName := appItems[0].Name
			if actualFirstAppName != frontAppName {
				var appNames []string
				for _, item := range appItems {
					appNames = append(appNames, item.Name)
				}
				s.Logf("Waiting for %q to be first. Current first app: %q. Order: %v", frontAppName, actualFirstAppName, appNames)
				return errors.Errorf("unexpected first app: expected %q, got %q", frontAppName, actualFirstAppName)
			}
			return nil
		}, &testing.PollOptions{Timeout: 15 * time.Second}); err != nil {
			s.Fatalf("Failed to verify %q was added to front: %v", frontAppName, err)
		}
	})

	s.Run(ctx, "SortLauncher", func(ctx context.Context, s *testing.State) {
		// Call sortLauncher. This API does not have a callback.
		if err := inSessionConn.Eval(ctx, `
			chrome.enterprise.remoteApps.sortLauncher({position: 'REMOTE_APPS_FIRST'});
		`, nil); err != nil {
			s.Fatal("Failed to sort launcher: ", err)
		}
		s.Log("Called sortLauncher, polling for UI to update")

		if err := ui.WaitUntilExists(appsGridFinder)(ctx); err != nil {
			s.Fatal("Failed to find the apps grid: ", err)
		}

		// After sorting, remote apps should come first, sorted alphabetically.
		// Our remote items: "Cardiology EMR", "Folder Clinical Apps", "Imaging Viewer", "Telehealth Portal".
		expectedAppOrder := []string{iconAppName, folderItemName, frontAppName, topLevelAppName}
		s.Log("Waiting for remote apps to be sorted: ", expectedAppOrder)

		// Poll the UI until the app order is correct.
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			appItems, err := ui.NodesInfo(ctx, appItemFinder)
			if err != nil {
				return testing.PollBreak(errors.Wrap(err, "failed to get app items info"))
			}

			if len(appItems) < len(expectedAppOrder) {
				return errors.Errorf("not enough apps in grid: got %d, want at least %d", len(appItems), len(expectedAppOrder))
			}

			var currentOrder []string
			for i := 0; i < len(expectedAppOrder); i++ {
				currentOrder = append(currentOrder, appItems[i].Name)
			}

			for i, expectedName := range expectedAppOrder {
				if appItems[i].Name != expectedName {
					s.Logf("Waiting for sort. At index %d, got %q, want %q. Current order: %v",
						i, appItems[i].Name, expectedName, currentOrder)
					return errors.Errorf("unexpected app order: at index %d, got %q, want %q",
						i, appItems[i].Name, expectedName)
				}
			}

			// If we get here, the order is correct.
			return nil
		}, &testing.PollOptions{
			Timeout:  15 * time.Second,
			Interval: 500 * time.Millisecond,
		}); err != nil {
			s.Error("Failed to observe correct app order after sort within timeout: ", err)
		}

		if err := hideLauncher(ctx); err != nil {
			s.Log("Failed to hide launcher: ", err)
		}
	})

	s.Run(ctx, "DeleteAppsAndFolder", func(ctx context.Context, s *testing.State) {
		// Delete the app inside the folder.
		if err := inSessionConn.Call(ctx, nil, `self.callRemoteAppsAPI`, "deleteApp", appIDInFolder); err != nil {
			s.Fatal("Failed to delete app in folder: ", err)
		}
		verifyAppPresence(appIDInFolder, false)

		// Verify the folder is now gone (since it's empty).
		s.Log("Verifying folder is removed from launcher")
		if err := showLauncher(ctx); err != nil {
			s.Fatal("Failed to show launcher: ", err)
		}
		if err := ui.WaitUntilGone(folderFinder)(ctx); err != nil {
			s.Fatal("Folder was not removed after deleting its last app: ", err)
		}
		if err := hideLauncher(ctx); err != nil {
			s.Log("Failed to hide launcher: ", err)
		}

		// Delete the top-level app.
		if err := inSessionConn.Call(ctx, nil, `self.callRemoteAppsAPI`, "deleteApp", topLevelAppID); err != nil {
			s.Fatal("Failed to delete top level app: ", err)
		}
		verifyAppPresence(topLevelAppID, false)

		// Delete the 'Imaging Viewer' app.
		if err := inSessionConn.Call(ctx, nil, `self.callRemoteAppsAPI`, "deleteApp", frontAppID); err != nil {
			s.Fatal("Failed to delete front app: ", err)
		}
		verifyAppPresence(frontAppID, false)

		// Delete the 'Cardiology EMR' app.
		if err := inSessionConn.Call(ctx, nil, `self.callRemoteAppsAPI`, "deleteApp", iconAppID); err != nil {
			s.Fatal("Failed to delete icon app: ", err)
		}
		verifyAppPresence(iconAppID, false)
	})
}
