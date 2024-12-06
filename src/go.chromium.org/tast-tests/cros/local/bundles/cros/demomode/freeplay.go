// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package demomode

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/demomode"
	"go.chromium.org/tast-tests/cros/local/uidetection"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type freeplayTestParams struct {
	dmServerURL                        string
	downloadDemoModeAppComponent       bool
	downloadDemoModeResourcesComponent bool
	verifyWebApps                      bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:     Freeplay,
		Desc:     "Verify that all preinstalled sample apps are present in Demo Mode freeplay",
		Contacts: []string{"cros-demo-mode-eng@google.com", "wanghaifan@google.com"},
		// Chrome OS Server Projects > Enterprise Management > Demo Mode
		BugComponent: "b:812312",
		Attr:         []string{"group:mainline", "informational"},
		// Demo Mode uses Zero Touch Enrollment for enterprise enrollment, which
		// requires a real TPM.
		// We require "arc" and "chrome_internal" because the ARC TOS screen
		// is only shown for chrome-branded builds when the device is ARC-capable.
		// Demo Mode doesn't support VMs, use "crossystem" to exclude VMs.
		SoftwareDeps: []string{"chrome", "chrome_internal", "arc", "tpm2", "crossystem"},
		Timeout:      5 * time.Minute,
		Params: []testing.Param{{
			Name: "web_apps_prod",
			Val: freeplayTestParams{
				dmServerURL: policy.DMServerProdURL,
				// --demo-mode-swa-content-directory and --demo-mode-resource-directory
				// were used to skip the demo mode app and resources component download
				// and install process.
				downloadDemoModeAppComponent:       false,
				downloadDemoModeResourcesComponent: false,
				verifyWebApps:                      true,
			},
			Fixture: fixture.PostDemoModeOOBESkipBothComponentsProd,
			// TODO (b/346725308): Refactor to use utility and known dependency list.
			ExtraSearchFlags: []*testing.StringPair{{
				Key: "external_dependency", Value: "DMServerProd",
			}},
		}},
	})
}

func Freeplay(ctx context.Context, s *testing.State) {
	params := s.Param().(freeplayTestParams)
	dmServerURL := params.dmServerURL

	// --force-devtools-available forces devtools on regardless of the policy
	// (devtools is disabled in Demo Mode policy) to support connecting to the test
	// API extension.
	//
	// --component-updater=test-request adds a "test-request" parameter to Omaha
	// update requests, causing the fetched Demo Mode App component to come from a
	// test cohort.
	//
	// --log-level=0 increase Chrome's log level to INFO
	chromeExtraArgs := []string{"--force-devtools-available",
		"--component-updater=test-request",
		"--log-level=0",
	}
	if !params.downloadDemoModeAppComponent {
		s.Log("Skipping demo mode app component download")
		chromeExtraArgs = append(chromeExtraArgs, "--demo-mode-swa-content-directory")
	}
	if !params.downloadDemoModeResourcesComponent {
		s.Log("Skipping demo mode resources component download")
		chromeExtraArgs = append(chromeExtraArgs, "--demo-mode-resource-directory")
	}

	cr, err := chrome.New(ctx,
		chrome.NoLogin(),
		chrome.ARCSupported(),
		chrome.KeepEnrollment(),
		chrome.ExtraArgs(chromeExtraArgs...),
		chrome.DMSPolicy(dmServerURL),
	)
	if err != nil {
		s.Fatal("Failed to restart Chrome: ", err)
	}

	clearUpCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()
	defer cr.Close(clearUpCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create the test API connection: ", err)
	}
	defer faillog.DumpUITreeOnError(clearUpCtx, s.OutDir(), s.HasError, tconn)

	// If we did not download the demo mode app component, the app will still exist,
	// but just with a blank content.
	s.Log("Waiting for Demo Mode app to appear")
	if err := demomode.WaitForDemoModeApp(ctx, tconn); err != nil {
		s.Fatal("Failed to wait for Demo Mode app: ", err)
	}

	s.Log("Breaking the Attract Loop")
	if params.downloadDemoModeAppComponent {
		if err := demomode.BreakSWAAttractLoop(ctx, tconn); err != nil {
			s.Fatal("Failed to break Attract Loop: ", err)
		}
	}

	if params.verifyWebApps {
		s.Log("Verifying web apps are present")
		if err := verifyWebApps(ctx, tconn); err != nil {
			s.Fatal("Failed to verify web apps: ", err)
		}
	}

	// If we did not download the demo mode resources component, we will not have any
	// Android apps.
	if params.downloadDemoModeResourcesComponent {
		s.Log("Verifying Android apps are present")
		if err := verifyAndroidApps(ctx, tconn); err != nil {
			s.Fatal("Failed to verify Android apps: ", err)
		}
	}

	// TODO(b/263520014): Add individual testing for additional freeplay apps
}

func verifyAndroidApps(ctx context.Context, tconn *chrome.TestConn) error {
	// Maps app names to Shelf Item IDs. These "names" are arbitrary, only having
	// relevance for the context of this test; they are not the actual Shelf Item
	// titles, as the app publisher could change the title at will. So one should only
	// rely on the ID, as this is unchanging (derived from the package name for Android
	// Apps).
	var AndroidAppsToIDs = map[string]string{
		"GooglePhotos": "fdbkkojdbojonckghlanfaopfakedeca",
		// Temporarily unpin Stardew Valley (b/343228202) but still have it installed.
		// Pin it back once Stardew Valley issue (b/328569631) is fixed.
		// "StardewValley": "ljibeljdcmpldadfgijmbaocjibloonn",
	}

	if err := verifyAppsPinned(ctx, tconn, AndroidAppsToIDs); err != nil {
		return errors.Wrap(err, "failed to verify Android apps")
	}

	// GoBigSleepLint: Sleep for 15 seconds to give ARC a bit of extra time to boot up
	// before trying to launch Google Photos (b/263517131).
	if err := testing.Sleep(ctx, 15*time.Second); err != nil {
		return errors.Wrap(err, "failed to sleep")
	}

	googlePhotosID := AndroidAppsToIDs["GooglePhotos"]

	title, err := ash.ShelfItemTitleFromID(ctx, tconn, []string{googlePhotosID})
	if err != nil {
		return errors.Wrap(err, "failed to get app title for Google Photos app")
	}
	if err := ash.LaunchAppFromShelf(ctx, tconn, title[0], googlePhotosID); err != nil {
		return errors.Wrap(err, "failed to launch Google Photos app from shelf")
	}

	// Use uidetection library as Google Photos is an Android App (so no accessibility tree).
	ud := uidetection.NewDefault(tconn)
	// Verify app has started by ensuring "Google Photos" text is present on screen.
	appHeaderText := uidetection.TextBlock([]string{"Google", "Photos"})
	if err := ud.WaitUntilExists(appHeaderText)(ctx); err != nil {
		return errors.Wrap(err, "failed to wait for Google Photos to launch")
	}
	// Verify sample photos have loaded by lack of "No Photos" error message.
	errorText := uidetection.TextBlock([]string{"No", "Photos"})
	// First try and look for error message, to ensure ud.WaitUntilGone doesn't quickly miss
	// it and pass incorrectly
	if err := uiauto.IfSuccessThen(
		ud.WaitUntilExists(errorText),
		ud.WaitUntilGone(errorText),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to wait for \"No Photos\" text to not be present")
	}
	return nil
}

func verifyWebApps(ctx context.Context, tconn *chrome.TestConn) error {
	// Maps app names to Shelf Item IDs. These "names" are arbitrary, only having
	// relevance for the context of this test; they are not the actual Shelf Item titles,
	// as the app publisher could change the title at will. So one should only rely on the
	// ID, as this is unchanging (derived from the URL for PWAs or the package name for
	// Android Apps).
	var webAppsToIDs = map[string]string{
		"Zoom":       "ddamjdmghnhnicfnliimfobemngigiom",
		"Youtube":    "agimnkijcaahngcdmfeangaknmldooml",
		"GoogleDocs": "cepkndkdlbllfhpfhledabdcdbidehkd",
		"BeFunky":    "fjoomcalbeohjbnlcneddljemclcekeg",
		"SumoPaint":  "genadphlobhbpdnafiphnppelkagmghm",
		"Spotify":    "pjibgclleladliembfgfagdaldikeohf",
	}
	if err := verifyAppsPinned(ctx, tconn, webAppsToIDs); err != nil {
		return errors.Wrap(err, "failed to verify web apps")
	}
	return nil
}

func verifyAppsPinned(ctx context.Context, tconn *chrome.TestConn,
	freeplayAppsToIDs map[string]string) error {
	for appName, appID := range freeplayAppsToIDs {
		if err := waitForAppPinned(ctx, tconn, appID); err != nil {
			return errors.Wrap(err, "Timed out waiting for "+appName+
				" app to appear in the shelf")
		}
	}
	return nil
}

func waitForAppPinned(ctx context.Context, tconn *chrome.TestConn, targetAppID string) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		if pinned, err := appIsPinned(ctx, tconn, targetAppID); err != nil {
			return testing.PollBreak(err)
		} else if !pinned {
			return errors.New("Failed to wait for app to be pinned - ID: " + targetAppID)
		}
		return nil
	}, &testing.PollOptions{Timeout: 3 * time.Minute})
}

func appIsPinned(ctx context.Context, tconn *chrome.TestConn, targetAppID string) (bool, error) {
	pinnedAppIDs, err := ash.GetPinnedAppIds(ctx, tconn)
	if err != nil {
		return false, errors.Wrap(err, "failed to get pinned App IDs")
	}
	for _, appID := range pinnedAppIDs {
		if appID == targetAppID {
			return true, nil
		}
	}
	return false, nil
}
