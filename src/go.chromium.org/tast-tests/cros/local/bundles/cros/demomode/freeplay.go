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

func init() {
	testing.AddTest(&testing.Test{
		Func:         Freeplay,
		LacrosStatus: testing.LacrosVariantNeeded,
		Desc:         "Verify that all preinstalled sample apps are present in Demo Mode freeplay",
		Contacts:     []string{"cros-demo-mode-eng@google.com", "jacksontadie@google.com"},
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
			Name:    "alpha",
			Val:     policy.DMServerAlphaURL, // DMServerURL
			Fixture: fixture.PostDemoModeOOBEAlpha,
		}, {
			Name:    "prod",
			Val:     policy.DMServerProdURL, // DMServerURL
			Fixture: fixture.PostDemoModeOOBEProd,
		}},
	})
}

func Freeplay(ctx context.Context, s *testing.State) {
	dmServerURL := s.Param().(string)

	cr, err := chrome.New(ctx,
		chrome.NoLogin(),
		chrome.ARCSupported(),
		chrome.KeepEnrollment(),
		// --force-devtools-available forces devtools on regardless of policy (devtools is
		// disabled in Demo Mode policy) to support connecting to the test API extension.
		chrome.ExtraArgs("--force-devtools-available"),
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

	if err := demomode.BreakSWAAttractLoop(ctx, tconn); err != nil {
		s.Fatal("Failed to break Attract Loop: ", err)
	}

	// Maps app names to Shelf Item IDs. These "names" are arbitrary, only having
	// relevance for the context of this test; they are not the actual Shelf Item titles,
	// as the app publisher could change the title at will. So one should only rely on the
	// ID, as this is unchanging (derived from the URL for PWAs or the package name for
	// Android Apps).
	var freeplayAppsToIDs = map[string]string{
		"Zoom":          "jldpdkiafafcejhceeincjmlkmibemgj",
		"Youtube":       "agimnkijcaahngcdmfeangaknmldooml",
		"GoogleDocs":    "cepkndkdlbllfhpfhledabdcdbidehkd",
		"BeFunky":       "fjoomcalbeohjbnlcneddljemclcekeg",
		"SumoPaint":     "ioeckplmjbkckfdcpkibkhhbefdabjgc",
		"Spotify":       "pjibgclleladliembfgfagdaldikeohf",
		"GooglePhotos":  "fdbkkojdbojonckghlanfaopfakedeca",
		"StardewValley": "ljibeljdcmpldadfgijmbaocjibloonn",
	}

	for appName, appID := range freeplayAppsToIDs {
		s.Log("Verifying that " + appName + " is pinned")
		if err := waitForAppPinned(ctx, tconn, appID); err != nil {
			s.Fatal("Timed out waiting for "+appName+" app to appear in the shelf: ", err)
		}
	}

	// GoBigSleepLint: Sleep for 15 seconds to give ARC a bit of extra time to boot up
	// before trying to launch Google Photos (b/263517131).
	if err := testing.Sleep(ctx, 15*time.Second); err != nil {
		s.Fatal("Failed to sleep: ", err)
	}

	googlePhotosID := freeplayAppsToIDs["GooglePhotos"]

	title, err := ash.ShelfItemTitleFromID(ctx, tconn, []string{googlePhotosID})
	if err != nil {
		s.Fatal("Failed to get app title for Google Photos app: ", err)
	}
	if err := ash.LaunchAppFromShelf(ctx, tconn, title[0], googlePhotosID); err != nil {
		s.Fatal("Failed to launch Google Photos app from shelf: ", err)
	}

	// Use uidetection library as Google Photos is an Android App (so no accessibility tree).
	ud := uidetection.NewDefault(tconn)
	// Verify app has started by ensuring "Google Photos" text is present on screen.
	appHeaderText := uidetection.TextBlock([]string{"Google", "Photos"})
	if err := ud.WaitUntilExists(appHeaderText)(ctx); err != nil {
		s.Fatal("Failed to wait for Google Photos to launch: ", err)
	}
	// Verify sample photos have loaded by lack of "No Photos" error message.
	errorText := uidetection.TextBlock([]string{"No", "Photos"})
	// First try and look for error message, to ensure ud.WaitUntilGone doesn't quickly miss
	// it and pass incorrectly
	if err := uiauto.IfSuccessThen(
		ud.WaitUntilExists(errorText),
		ud.WaitUntilGone(errorText),
	)(ctx); err != nil {
		s.Fatal("Failed to wait for \"No Photos\" text to not be present: ", err)
	}
	// TODO(b/263520014): Add individual testing for additional freeplay apps
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
