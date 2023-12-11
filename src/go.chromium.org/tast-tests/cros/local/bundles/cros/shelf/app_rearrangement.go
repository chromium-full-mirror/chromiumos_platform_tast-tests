// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package shelf

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/mouse"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type rearrangmentTargetAppType string

const (
	chromeAppTest  rearrangmentTargetAppType = "ChromeAppTest"  // Verify the rearrangement behavior on a Chrome app.
	fileAppTest    rearrangmentTargetAppType = "FileAppTest"    // Verify the rearrangement behavior on the File app.
	pwaAppTest     rearrangmentTargetAppType = "PwaAppTest"     // Verify the rearrangement behavior on a PWA.
	androidAppTest rearrangmentTargetAppType = "AndroidAppTest" // Verify the rearrangement behavior on an Android app.
)

const (
	// Constant indices that refer to different states in the test. Used to check the app orders.
	appsBeforeDragToPin   int = 0
	appsAfterDragToPin    int = 1
	appsBeforeDragToUnpin int = 2
	appsAfterDragToUnpin  int = 3
)

type rearrangmentTestType struct {
	appType  rearrangmentTargetAppType
	underRTL bool // If true, the system UI is adapted to right-to-left languages.
	bt       browser.Type
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         AppRearrangement,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Tests the rearrangement of shelf app icons",
		Contacts: []string{
			"cros-system-ui-eng@google.com",
			"andrewxu@chromium.org",
			"tbarzic@chromium.org",
			"chromeos-sw-engprod@google.com",
		},
		// ChromeOS > Software > System UI Surfaces > Shelf
		BugComponent: "b:1288352",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      3 * time.Minute,
		Data:         []string{"web_app_install_force_list_index.html", "web_app_install_force_list_manifest.json", "web_app_install_force_list_service-worker.js", "web_app_install_force_list_icon-192x192.png", "web_app_install_force_list_icon-512x512.png"},
		Params: []testing.Param{{
			Name: "rearrange_chrome_apps",
			Val: rearrangmentTestType{
				appType:  chromeAppTest,
				underRTL: false,
				bt:       browser.TypeAsh,
			},
			Fixture: "install2Apps",
		}, {
			Name: "rearrange_chrome_apps_rtl",
			Val: rearrangmentTestType{
				appType:  chromeAppTest,
				underRTL: true,
				bt:       browser.TypeAsh,
			},
			Fixture: "install2Apps",
		}, {
			Name: "rearrange_file_app",
			Val: rearrangmentTestType{
				appType:  fileAppTest,
				underRTL: false,
				bt:       browser.TypeAsh,
			},
			Fixture: "install2Apps",
		}, {
			Name: "rearrange_file_app_rtl",
			Val: rearrangmentTestType{
				appType:  fileAppTest,
				underRTL: true,
				bt:       browser.TypeAsh,
			},
			Fixture: "install2Apps",
		}, {
			Name: "rearrange_pwa_app",
			Val: rearrangmentTestType{
				appType:  pwaAppTest,
				underRTL: false,
				bt:       browser.TypeAsh,
			},
			Fixture: fixture.ChromePolicyLoggedIn,
		}, {
			Name: "rearrange_android_app_androidp",
			Val: rearrangmentTestType{
				appType:  androidAppTest,
				underRTL: false,
				bt:       browser.TypeAsh,
			},
			Fixture:           "arcBooted",
			ExtraSoftwareDeps: []string{"android_p"},
		}, {
			Name: "rearrange_android_app_androidvm",
			Val: rearrangmentTestType{
				appType:  androidAppTest,
				underRTL: false,
				bt:       browser.TypeAsh,
			},
			Fixture:           "arcBooted",
			ExtraSoftwareDeps: []string{"android_vm"},
		}, {
			Name: "rearrange_chrome_apps_lacros",
			Val: rearrangmentTestType{
				appType:  chromeAppTest,
				underRTL: false,
				bt:       browser.TypeLacros,
			},
			ExtraSoftwareDeps: []string{"lacros"},
			Fixture:           "install2LacrosApps",
		}, {
			Name: "rearrange_chrome_apps_rtl_lacros",
			Val: rearrangmentTestType{
				appType:  chromeAppTest,
				underRTL: true,
				bt:       browser.TypeLacros,
			},
			ExtraSoftwareDeps: []string{"lacros"},
			Fixture:           "install2LacrosApps",
		}, {
			Name: "rearrange_file_app_lacros",
			Val: rearrangmentTestType{
				appType:  fileAppTest,
				underRTL: false,
				bt:       browser.TypeLacros,
			},
			ExtraSoftwareDeps: []string{"lacros"},
			Fixture:           "install2LacrosApps",
		}, {
			Name: "rearrange_file_app_rtl_lacros",
			Val: rearrangmentTestType{
				appType:  fileAppTest,
				underRTL: true,
				bt:       browser.TypeLacros,
			},
			ExtraSoftwareDeps: []string{"lacros"},
			Fixture:           "install2LacrosApps",
		}, {
			Name: "rearrange_pwa_app_lacros",
			Val: rearrangmentTestType{
				appType:  pwaAppTest,
				underRTL: false,
				bt:       browser.TypeLacros,
			},
			ExtraSoftwareDeps: []string{"lacros"},
			Fixture:           fixture.LacrosPolicyLoggedIn,
		}, {
			Name: "rearrange_android_app_androidp_lacros",
			Val: rearrangmentTestType{
				appType:  androidAppTest,
				underRTL: false,
				bt:       browser.TypeLacros,
			},
			Fixture:           "lacrosWithArcBooted",
			ExtraSoftwareDeps: []string{"android_p", "lacros"},
		}, {
			Name: "rearrange_android_app_androidvm_lacros",
			Val: rearrangmentTestType{
				appType:  androidAppTest,
				underRTL: false,
				bt:       browser.TypeLacros,
			},
			Fixture:           "lacrosWithArcBooted",
			ExtraSoftwareDeps: []string{"android_vm", "lacros"},
		}},
	})
}

// AppRearrangement tests app icon rearrangement on the shelf.
func AppRearrangement(ctx context.Context, s *testing.State) {
	// Reserve some time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	var cr *chrome.Chrome
	var closeBrowser uiauto.Action
	var err error

	testType := s.Param().(rearrangmentTestType)
	testAppType := testType.appType
	isunderRTL := testType.underRTL
	bt := testType.bt
	switch testAppType {
	case chromeAppTest, fileAppTest:
		options := s.FixtValue().([]chrome.Option)
		if isunderRTL {
			options = append(options, chrome.ExtraArgs("--force-ui-direction=rtl"))
		}
		cr, _, closeBrowser, err = browserfixt.SetUpWithNewChrome(ctx, bt, lacrosfixt.NewConfig(), options...)
		if err != nil {
			s.Fatalf("Failed to start %v browser: %v", bt, err)
		}
		defer cr.Close(cleanupCtx)
		defer closeBrowser(cleanupCtx)
	case pwaAppTest:
		cr = s.FixtValue().(chrome.HasChrome).Chrome()
		// Setup the browser based on the type. ash-chrome can load PWA immediately on startup, but lacros-chrome won't until lacros process starts first.
		_, closeBrowser, err = browserfixt.SetUp(ctx, cr, bt)
		if err != nil {
			s.Fatalf("Failed to start %v browser: %v", bt, err)
		}
		defer closeBrowser(cleanupCtx)
	case androidAppTest:
		cr = s.FixtValue().(*arc.PreData).Chrome
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}

	// Ensure that the device is in clamshell mode.
	cleanup, err := ash.EnsureTabletModeEnabled(ctx, tconn, false)
	if err != nil {
		s.Fatal("Failed to ensure clamshell/tablet mode: ", err)
	}
	defer cleanup(ctx)

	resetPinState, err := ash.ResetShelfPinState(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get the function to reset pin states: ", err)
	}
	defer resetPinState(ctx)

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree")

	// Ensure the display is shown before test starts dragging apps. If the
	// display is off, mouse press on the shelf item will turn on the display,
	// causing display configuration change, which in turns resets the mouse
	// capture, causing mouse drag events not to get propagated to the shelf
	// item. See https://crbug.com/1364741.
	if err := power.TurnOnDisplay(ctx); err != nil {
		s.Fatal("Failed to turn on display: ", err)
	}

	items, err := ash.ShelfItems(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get shelf items: ", err)
	}

	// Get the expected browser.
	browserApp, err := apps.PrimaryBrowser(ctx, tconn)
	if err != nil {
		s.Fatal("Could not find the primary browser app info: ", err)
	}

	var itemsToUnpin []string
	for _, item := range items {
		if item.AppID != browserApp.ID {
			itemsToUnpin = append(itemsToUnpin, item.AppID)
		}
	}

	// Unpin all apps except the browser.
	if err := ash.UnpinApps(ctx, tconn, itemsToUnpin); err != nil {
		s.Fatalf("Failed to unpin apps %v: %v", itemsToUnpin, err)
	}

	// The ids of the apps to pin.
	var appIDsToPin []string

	// The app ids by pin order before any drag-and-drop operations. An app that is pinned earlier has a smaller array index.
	var defaultAppIDsInPinOrder []string

	// The updated app ids by pin order after dragging the target app from the last slot to the first slot.
	var updatedAppIDsInPinOrder []string

	// The app ids by pin order after dragging a pinned app from the first slot across the separator to the last slot.
	// Note that the Settings app would be unpinned before the drag.
	var draggedToUnpinAppIDsInPinOrder []string

	// A list of pinned app ids arrays that caches the correct order of the pinned apps at different states.
	var arraysOfPinnedApps [][]string

	// Update appIDsToPin based on the test type.
	switch testAppType {
	case chromeAppTest:
		fakeApps, err := ash.InstalledFakeApps(ctx, tconn)
		if err != nil {
			s.Fatal("Failed to get fake app names when the test type is ChromeAppTest: ", err)
		}

		if len(fakeApps) != 2 {
			s.Fatalf("Failed to find all fake apps: want 2; got %q: ", len(fakeApps))
		}

		appIDsToPin = []string{apps.Settings.ID, fakeApps[1].AppID, fakeApps[0].AppID}
		defaultAppIDsInPinOrder = []string{browserApp.ID, apps.Settings.ID, fakeApps[1].AppID, fakeApps[0].AppID}
		updatedAppIDsInPinOrder = []string{fakeApps[0].AppID, browserApp.ID, apps.Settings.ID, fakeApps[1].AppID}
		draggedToUnpinAppIDsInPinOrder = []string{browserApp.ID, fakeApps[1].AppID, fakeApps[0].AppID, apps.Settings.ID}

	case fileAppTest:
		fakeApps, err := ash.InstalledFakeApps(ctx, tconn)
		if err != nil {
			s.Fatal("Failed to get fake app names when the test type is FileAppTest: ", err)
		}

		if len(fakeApps) != 2 {
			s.Fatalf("Failed to find all fake apps: want 2; got %q: ", len(fakeApps))
		}

		appIDsToPin = []string{apps.Settings.ID, fakeApps[1].AppID, apps.FilesSWA.ID}
		defaultAppIDsInPinOrder = []string{browserApp.ID, apps.Settings.ID, fakeApps[1].AppID, apps.FilesSWA.ID}
		updatedAppIDsInPinOrder = []string{apps.FilesSWA.ID, browserApp.ID, apps.Settings.ID, fakeApps[1].AppID}
		draggedToUnpinAppIDsInPinOrder = []string{browserApp.ID, fakeApps[1].AppID, apps.FilesSWA.ID, apps.Settings.ID}

	case pwaAppTest:
		fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()
		var cleanUp func(ctx context.Context) error
		pwaAppID, _, cleanUp, err := policyutil.InstallPwaAppByPolicy(ctx, tconn, cr, fdms, s.DataFileSystem())
		if err != nil {
			s.Fatal("Failed to install PWA: ", err)
		}

		appIDsToPin = []string{apps.Settings.ID, apps.FilesSWA.ID, pwaAppID}
		defaultAppIDsInPinOrder = []string{browserApp.ID, apps.Settings.ID, apps.FilesSWA.ID, pwaAppID}
		updatedAppIDsInPinOrder = []string{pwaAppID, browserApp.ID, apps.Settings.ID, apps.FilesSWA.ID}
		draggedToUnpinAppIDsInPinOrder = []string{browserApp.ID, apps.FilesSWA.ID, pwaAppID, apps.Settings.ID}

		// Use a shortened context for test operations to reserve time for cleanup.
		cleanupCtx := ctx
		var cancel context.CancelFunc
		ctx, cancel = ctxutil.Shorten(ctx, 10*time.Second)
		defer cancel()

		defer cleanUp(cleanupCtx)

	case androidAppTest:
		// Install a mock Android app under temporary sort.
		const apk = "ArcInstallAppWithAppListSortedTest.apk"
		a := s.FixtValue().(*arc.PreData).ARC
		if err := a.Install(ctx, arc.APKPath(apk)); err != nil {
			s.Fatal("Failed installing app under temporary sort: ", err)
		}

		appName := "InstallAppWithAppListSortedMockApp"
		installedArcAppID, err := ash.WaitForChromeAppByNameInstalled(ctx, tconn, appName, 1*time.Minute)
		if err != nil {
			s.Fatalf("Failed to wait until %s is installed: %v", appName, err)
		}

		appIDsToPin = []string{apps.Settings.ID, apps.FilesSWA.ID, installedArcAppID}
		defaultAppIDsInPinOrder = []string{browserApp.ID, apps.Settings.ID, apps.FilesSWA.ID, installedArcAppID}
		updatedAppIDsInPinOrder = []string{installedArcAppID, browserApp.ID, apps.Settings.ID, apps.FilesSWA.ID}
		draggedToUnpinAppIDsInPinOrder = []string{browserApp.ID, apps.FilesSWA.ID, installedArcAppID, apps.Settings.ID}
	}

	// Set the expected pinned app ids arrays.
	if arraysOfPinnedApps, err = getExpectedPinnedAppIds(defaultAppIDsInPinOrder, browserApp, bt); err != nil {
		s.Fatal("Failed to get the array of expected pinned app ids: ", err)
	}

	// Pin additional apps to create a more complex scenario for testing.
	if err := ash.PinApps(ctx, tconn, appIDsToPin); err != nil {
		s.Fatalf("Failed to pin %v to shelf: %v", appIDsToPin, err)
	}

	if err := ash.WaitUntilShelfIconAnimationFinishAction(tconn)(ctx); err != nil {
		s.Fatal("Failed to wait for shelf icon animation to finish after pinning additional apps: ", err)
	}

	if err := ash.VerifyShelfIconIndices(ctx, tconn, defaultAppIDsInPinOrder); err != nil {
		s.Fatal("Failed to verify shelf icon indices before any drag-and-drop operations: ", err)
	}

	defaultPinnedAppNamesInOrder, err := ash.ShelfItemTitleFromID(ctx, tconn, defaultAppIDsInPinOrder)
	if err != nil {
		s.Fatalf("Failed to get the app names of default pinned apps %v: %v", defaultAppIDsInPinOrder, err)
	}

	// Always use the last app as the target app. The target app is the one that is going to be dragged around the shelf.
	targetAppName := defaultPinnedAppNamesInOrder[len(defaultPinnedAppNamesInOrder)-1]
	targetAppID := defaultAppIDsInPinOrder[len(defaultAppIDsInPinOrder)-1]

	ui := uiauto.New(tconn)
	if err := ash.VerifyShelfAppBounds(ctx, tconn, ui, appNamesInVisualOrder(defaultPinnedAppNamesInOrder, isunderRTL), true); err != nil {
		s.Fatal("Failed to verify shelf app bounds: ", err)
	}

	scrollableShelfInfo, err := ash.FetchScrollableShelfInfoForState(ctx, tconn, &ash.ShelfState{})
	if err != nil {
		s.Fatal("Failed to fetch the scrollable shelf info: ", err)
	}

	shelfAppBounds := scrollableShelfInfo.IconsBoundsInScreen
	if len(shelfAppBounds) != 4 {
		s.Fatal("Wrong number of shelf apps: ", err)
	}

	firstSlotCenter := shelfAppBounds[0].CenterPoint()
	lastSlotCenter := shelfAppBounds[len(shelfAppBounds)-1].CenterPoint()

	const middleAppIndex = 2
	middleSlotBounds := shelfAppBounds[middleAppIndex]
	middleSlotCenter := middleSlotBounds.CenterPoint()
	middleAppName := defaultPinnedAppNamesInOrder[middleAppIndex]

	if err := startDragAction(tconn, "start drag on the target app from the last slot", lastSlotCenter)(ctx); err != nil {
		s.Fatal("Failed to start drag on the target app before moving to the middle point from the last slot: ", err)
	}

	if err := uiauto.Combine("move from the last slot to the middle slot",
		mouse.Move(tconn, middleSlotCenter, 1*time.Second),
		ash.WaitUntilShelfIconAnimationFinishAction(tconn))(ctx); err != nil {
		s.Fatal("Failed to move the target app from the last slot to the middle slot: ", err)
	}

	updatedMiddleAppBounds, err := ash.ShelfAppBoundsForNames(ctx, tconn, ui, []string{middleAppName})
	if err != nil {
		s.Fatal("Failed to get shelf app bounds after moving the target app from the last slot to the middle slot: ", err)
	}

	if !isunderRTL && updatedMiddleAppBounds[0].Left <= middleSlotBounds.Right() {
		// Expect that the app icon previously located on moveMiddleLocation moves rightward when it is not under RTL.
		s.Fatalf("Failed to check the app movement: want %s to move rightward; actually it does not move or moves leftward", middleAppName)
	}

	if isunderRTL && updatedMiddleAppBounds[0].Right() >= middleSlotBounds.Left {
		// Expect that the app icon previously located on moveMiddleLocation moves leftward when it is under RTL.
		s.Fatalf("Failed to check the app movement under RTL: want %s to move leftward; actually it does not move or moves rightward", middleAppName)
	}

	if err := uiauto.Combine("move to the first slot then release", mouse.Move(tconn, firstSlotCenter, time.Second),
		ash.WaitUntilShelfIconAnimationFinishAction(tconn),
		mouse.Release(tconn, mouse.LeftButton), uiauto.Sleep(time.Second))(ctx); err != nil {
		s.Fatalf("Failed to move %s to the first slot: %v", targetAppName, err)
	}

	if err := ash.VerifyShelfIconIndices(ctx, tconn, updatedAppIDsInPinOrder); err != nil {
		s.Fatalf("Failed to verify shelf icon indices to be %v: %v", updatedAppIDsInPinOrder, err)
	}

	// Update middleAppName after drag-and-drop.
	updatedPinnedAppNamesInOrder, err := ash.ShelfItemTitleFromID(ctx, tconn, updatedAppIDsInPinOrder)
	if err != nil {
		s.Fatalf("Failed to get the app names of the updated pinned apps %v: %v", updatedAppIDsInPinOrder, err)
	}
	middleAppName = updatedPinnedAppNamesInOrder[middleAppIndex]

	if err := startDragAction(tconn, "start drag on the target app from the first slot", firstSlotCenter)(ctx); err != nil {
		s.Fatal("Failed to start drag on the target app before moving from the first slot to the middle slot: ", err)
	}

	if err := uiauto.Combine("move from the first slot to the middle slot",
		mouse.Move(tconn, middleSlotCenter, 1*time.Second),
		ash.WaitUntilShelfIconAnimationFinishAction(tconn))(ctx); err != nil {
		s.Fatal("Failed to move the target app from the first slot to the middle slot: ", err)
	}

	updatedMiddleAppBounds, err = ash.ShelfAppBoundsForNames(ctx, tconn, ui, []string{middleAppName})
	if err != nil {
		s.Fatal("Failed to get shelf app bounds after moving the target app from the first slot to the middle location: ", err)
	}

	if !isunderRTL && updatedMiddleAppBounds[0].Right() >= middleSlotBounds.Left {
		// Expect that the app icon previously located on moveMiddleLocation moves leftward.
		s.Fatalf("Failed to check the app movement after moving from the first to middle: want %s to move leftward; actually it does not move or moves rightward", middleAppName)
	}

	if isunderRTL && updatedMiddleAppBounds[0].Left <= middleSlotBounds.Right() {
		// Expect that the app icon previously located on moveMiddleLocation moves rightward under RTL.
		s.Fatalf("Failed to check the app movement after moving from the first to middle under RTL: want %s to move rightward; actually it does not move or moves leftward", middleAppName)
	}

	if err := uiauto.Combine("move to the last slot then release", mouse.Move(tconn, lastSlotCenter, time.Second), ash.WaitUntilShelfIconAnimationFinishAction(tconn),
		mouse.Release(tconn, mouse.LeftButton), uiauto.Sleep(time.Second))(ctx); err != nil {
		s.Fatalf("Failed to move %s to the last slot: %v", targetAppName, err)
	}

	if err := ash.VerifyShelfIconIndices(ctx, tconn, defaultAppIDsInPinOrder); err != nil {
		s.Fatal("Failed to verify shelf icon indices before launching the target app: ", err)
	}

	// Launch the target app.
	if err := ash.LaunchAppFromShelf(ctx, tconn, targetAppName, targetAppID); err != nil {
		s.Fatalf("Failed to launch %s(%s) from the shelf: %v", targetAppName, targetAppID, err)
	}

	// Verify that app rearrangement works for a pinned shelf app with the activated window.
	if err := getDragAndDropAction(tconn, "move the target app with the activated window from the last slot to the first slot", lastSlotCenter, firstSlotCenter)(ctx); err != nil {
		s.Fatal("Failed to move the target app with the activated window from the last slot to the first slot: ", err)
	}

	if err := ash.VerifyShelfIconIndices(ctx, tconn, updatedAppIDsInPinOrder); err != nil {
		s.Fatal("Failed to verify shelf icon indices after moving the target app with the activated window from the last slot to the first slot: ", err)
	}

	if err := getDragAndDropAction(tconn, "move the target app with the activated window from the first slot to the last slot", firstSlotCenter, lastSlotCenter)(ctx); err != nil {
		s.Fatal("Failed to move the target app with the activated window from the first slot to the last slot")
	}

	if err := ash.VerifyShelfIconIndices(ctx, tconn, defaultAppIDsInPinOrder); err != nil {
		s.Fatal("Failed to verify shelf icon indices after moving the target app with the activated window from the first slot to the last slot: ", err)
	}

	// Start testing the behavior that dragging an unpinned app across the separator can pin the app
	if err := ash.UnpinApps(ctx, tconn, []string{targetAppID}); err != nil {
		s.Fatalf("Failed to unpin %s(%s): %v", targetAppName, targetAppID, err)
	}

	// Check the order of the pinned apps on the shelf.
	if err := ash.VerifyPinnedAppIndices(ctx, tconn, arraysOfPinnedApps[appsBeforeDragToPin]); err != nil {
		s.Fatal("Failed to verify pinned icon indices before the unpinned app is dragged to pin: ", err)
	}

	if err := getDragAndDropAction(tconn, "move the unpinned app with the activated window from the last slot to the first slot", lastSlotCenter, firstSlotCenter)(ctx); err != nil {
		s.Fatal("Failed to move the unpinned app from the last slot to the first slot: ", err)
	}

	// Verify that an unpinned app can be moved across the separator to the pinned apps and pin the app.
	if err := ash.VerifyShelfIconIndices(ctx, tconn, updatedAppIDsInPinOrder); err != nil {
		s.Fatal("Failed to verify shelf icon indices after the unpinned app is dragged then dropped: ", err)
	}

	// Check the order of the pinned apps on the shelf.
	if err := ash.VerifyPinnedAppIndices(ctx, tconn, arraysOfPinnedApps[appsAfterDragToPin]); err != nil {
		s.Fatal("Failed to verify pinned icon indices after the unpinned app is dragged to pin: ", err)
	}

	// Start testing the behavior that dragging a pinned app across the separator can unpin the app.

	// To have an unpinned app on the shelf and make the separator visible, we have to first launch an instance of the app "A" and unpin it, then we can drag another pinned app "B" to unpin.
	// To prevent fake apps using the same window and fail to launch the app on different windows, the Settings app is chosen here as the app "A" to be unpinned.
	unpinAppName := apps.Settings.Name
	unpinAppID := apps.Settings.ID

	// Launch the Settings app that will be unpinned later.
	if err := ash.LaunchAppFromShelf(ctx, tconn, unpinAppName, unpinAppID); err != nil {
		s.Fatalf("Failed to launch %s(%s) from the shelf: %v", unpinAppName, unpinAppID, err)
	}

	// Unpin the Settings app first to make sure there is an unpinned app and the separator exists on the shelf.
	if err := ash.UnpinApps(ctx, tconn, []string{unpinAppID}); err != nil {
		s.Fatalf("Failed to unpin %s(%s): %v", unpinAppName, unpinAppID, err)
	}

	// Check the order of the pinned apps on the shelf.
	if err := ash.VerifyPinnedAppIndices(ctx, tconn, arraysOfPinnedApps[appsBeforeDragToUnpin]); err != nil {
		s.Fatal("Failed to verify pinned icon indices before the pinned app is dragged to unpin: ", err)
	}

	// Drag the target app (app "B") at the first slot to the last slot where the app will be unpinned.
	if err := getDragAndDropAction(tconn, "move the target app with the activated window from the first slot to the last slot", firstSlotCenter, lastSlotCenter)(ctx); err != nil {
		s.Fatal("Failed to move the target app with the activated window from the first slot to the last slot")
	}

	if err := ash.VerifyShelfIconIndices(ctx, tconn, draggedToUnpinAppIDsInPinOrder); err != nil {
		s.Fatal("Failed to verify shelf icon indices after dragging the pinned app to unpin from the first slot to the last slot: ", err)
	}

	// Check the order of the pinned apps on the shelf.
	if err := ash.VerifyPinnedAppIndices(ctx, tconn, arraysOfPinnedApps[appsAfterDragToUnpin]); err != nil {
		s.Fatal("Failed to verify pinned icon indices after the pinned app is dragged to unpin: ", err)
	}

	// Cleanup.
	activeWindow, err := ash.GetActiveWindow(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get the active window: ", err)
	}
	if err := activeWindow.CloseWindow(ctx, tconn); err != nil {
		s.Fatalf("Failed to close the window(%s): %v", activeWindow.Name, err)
	}
}

func startDragAction(tconn *chrome.TestConn, actionName string, dragStartLocation coords.Point) uiauto.Action {
	return uiauto.Combine(actionName,
		mouse.Move(tconn, dragStartLocation, 0),
		// Drag in tablet mode starts with a long press.
		mouse.Press(tconn, mouse.LeftButton),
		uiauto.Sleep(time.Second))
}

func getDragAndDropAction(tconn *chrome.TestConn, actionName string, startLocation, endLocation coords.Point) uiauto.Action {
	return uiauto.Combine(actionName,
		mouse.Move(tconn, startLocation, 0),
		// Drag in tablet mode starts with a long press.
		mouse.Press(tconn, mouse.LeftButton),
		uiauto.Sleep(time.Second),

		mouse.Move(tconn, endLocation, 2*time.Second),
		ash.WaitUntilShelfIconAnimationFinishAction(tconn),
		mouse.Release(tconn, mouse.LeftButton),
		uiauto.Sleep(time.Second))
}

// appNamesInVisualOrder returns the apps names in visual order from the name array in pin order.
func appNamesInVisualOrder(namesInPinOrder []string, isunderRTL bool) []string {
	// When it is not under RTL, the visual order is the same as the pin order.
	if !isunderRTL {
		return namesInPinOrder
	}

	// Under RTL, the visual order is the reversal of the pin order.
	size := len(namesInPinOrder)
	namesInVisualOrder := make([]string, size)
	for i := size - 1; i >= 0; i-- {
		namesInVisualOrder[size-1-i] = namesInPinOrder[i]
	}
	return namesInVisualOrder
}

// getExpectedPinnedAppIds returns an array of 4 arrays of pinned app ids before/after the drag-to-pin and drag-to-unpin actions.
func getExpectedPinnedAppIds(defaultAppIDsInPinOrder []string, browserApp apps.App, bt browser.Type) ([][]string, error) {
	// Suppose the IDs in defaultAppIDsInPinOrder are order like below. Before testing drag-to-pin, target dragged app 3 will be unpinned
	// [ 0 1 2 | 3 ], where | is the separator that separate the pinned and unpinned apps.
	// After dragging target app 3, the order becomes
	// [ 3 0 1 2 ]
	// The Settings app will be unpinned here, where it should be app 1 according to the set up.
	// [ 3 0 2 | 1 ]
	// Then the target dragged app 3 will be dragged to the last position of the pinned apps.
	// [ 0 2 3 | 1 ]

	if len(defaultAppIDsInPinOrder) != 4 {
		return nil, errors.Errorf("there should be 4 IDs in the array but it only has %d", len(defaultAppIDsInPinOrder))
	}

	if defaultAppIDsInPinOrder[0] != browserApp.ID {
		return nil, errors.New("the browser shortcut is not located at the expected position")
	}

	if defaultAppIDsInPinOrder[1] != apps.Settings.ID {
		return nil, errors.New("the Settings app is not located at the expected position")
	}

	var arraysOfPinnedApps [4][]string

	// Lacros browser is not considered as a browser shortcut so it should be counted as a pinned app.
	if bt == browser.TypeLacros {
		arraysOfPinnedApps[appsBeforeDragToPin] = []string{defaultAppIDsInPinOrder[0], defaultAppIDsInPinOrder[1], defaultAppIDsInPinOrder[2]}
		arraysOfPinnedApps[appsAfterDragToPin] = []string{defaultAppIDsInPinOrder[3], defaultAppIDsInPinOrder[0], defaultAppIDsInPinOrder[1], defaultAppIDsInPinOrder[2]}
		arraysOfPinnedApps[appsBeforeDragToUnpin] = []string{defaultAppIDsInPinOrder[3], defaultAppIDsInPinOrder[0], defaultAppIDsInPinOrder[2]}
		arraysOfPinnedApps[appsAfterDragToUnpin] = []string{defaultAppIDsInPinOrder[0], defaultAppIDsInPinOrder[2], defaultAppIDsInPinOrder[3]}
	} else {
		arraysOfPinnedApps[appsBeforeDragToPin] = []string{defaultAppIDsInPinOrder[1], defaultAppIDsInPinOrder[2]}
		arraysOfPinnedApps[appsAfterDragToPin] = []string{defaultAppIDsInPinOrder[3], defaultAppIDsInPinOrder[1], defaultAppIDsInPinOrder[2]}
		arraysOfPinnedApps[appsBeforeDragToUnpin] = []string{defaultAppIDsInPinOrder[3], defaultAppIDsInPinOrder[2]}
		arraysOfPinnedApps[appsAfterDragToUnpin] = []string{defaultAppIDsInPinOrder[2], defaultAppIDsInPinOrder[3]}
	}

	return arraysOfPinnedApps[:], nil
}
