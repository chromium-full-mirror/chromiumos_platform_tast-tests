// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package filemanager

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/metrics"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/drivefs"
	"go.chromium.org/tast-tests/cros/local/oobe"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           BulkPinningDrivePinningChoobeScreen,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		LacrosStatus:   testing.LacrosVariantUnneeded,
		Desc:           "Verify that the the CHOOBE screen shows for consumer users",
		BugComponent:   "b:167289",
		Contacts: []string{
			"chromeos-files-syd@google.com",
			"benreich@google.com",
		},
		SoftwareDeps: []string{
			"chrome",
			"chrome_internal",
			"drivefs",
		},
		Attr: []string{
			"group:cbx",
			"cbx_feature_enabled",
			"cbx_unstable",
		},
		VarDeps: []string{
			"ui.signinProfileTestExtensionManifestKey",
			"drivefs.accountPool",
		},
		TestBedDeps: []string{tbdep.Cbx(true)},
		SearchFlags: []*testing.StringPair{{
			Key:   "feature_id",
			Value: "screenplay-ad7ec913-8fa9-4b51-9fb8-869e96aa678a",
		}},
		Timeout: 5 * time.Minute,
	})
}

func BulkPinningDrivePinningChoobeScreen(ctx context.Context, s *testing.State) {
	const maxUITimeout = 10 * time.Second

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 10*time.Second)
	defer cancel()

	chromeOptions := []chrome.Option{
		chrome.EnableFeatures("FeatureManagementDriveFsBulkPinning"),
		chrome.DontSkipOOBEAfterLogin(),
		chrome.GAIALoginPool(s.RequiredVar("drivefs.accountPool")),
		chrome.LoadSigninProfileExtension(s.RequiredVar("ui.signinProfileTestExtensionManifestKey")),
	}

	cr, err := chrome.New(ctx, chromeOptions...)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	oobeConn, err := cr.WaitForOOBEConnection(ctx)
	if err != nil {
		s.Fatal("Failed to create OOBE connection: ", err)
	}
	defer oobeConn.Close()

	tconn, err := cr.SigninProfileTestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create the signin profile test API connection: ", err)
	}
	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	dfs, err := drivefs.NewDriveFs(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to wait for DriveFS to mount: ", err)
	}
	defer dfs.SaveLogsOnError(cleanupCtx, s.HasError)

	// Wait for the `TimeSpentListing` UMA to get emitted, this happens when the
	// listing of files has successfully completed.
	umaName := "FileBrowser.GoogleDrive.BulkPinning.TimeSpentListing"
	histograms, err := metrics.RunAndWaitAll(ctx, tconn, 30*time.Second, func(ctx context.Context) error {
		return oobe.AdvanceThroughConsolidatedConsentIfShown(ctx, oobeConn, tconn)
	}, umaName)
	if err != nil {
		s.Fatal("Failed to retrieve histogram data: ", err)
	}

	if len(histograms) != 1 {
		s.Fatalf("Failed to record histogram, got %d want 1", len(histograms))
	}
	if histograms[0].Sum == 0 {
		s.Fatal("Failed to record a histogram value for: ", umaName)
	}

	if err := oobeConn.Eval(ctx, "OobeAPI.advanceToScreen('choobe')", nil); err != nil {
		s.Fatal("Failed to advance to the choobe screen: ", err)
	}

	s.Log("Selecting the Everything Offline feature in CHOOBE and clicking next")
	if err := oobeConn.WaitForExprFailOnErrWithTimeout(ctx, "OobeAPI.screens.ChoobeScreen.isReadyForTesting()", maxUITimeout); err != nil {
		s.Fatal("Failed to wait for the choobe screen to be ready for testing: ", err)
	}

	if err := oobeConn.Eval(ctx, "OobeAPI.screens.ChoobeScreen.clickDrivePinningScreen()", nil); err != nil {
		s.Fatal("Failed to click the drive pinning screen button: ", err)
	}

	if err := oobeConn.WaitForExprFailOnErrWithTimeout(ctx, "OobeAPI.screens.ChoobeScreen.isDrivePinningScreenChecked()", maxUITimeout); err != nil {
		s.Fatal("Failed to wait for the drive pinning screen to be checked: ", err)
	}

	if err := oobeConn.Eval(ctx, "OobeAPI.screens.ChoobeScreen.clickNext()", nil); err != nil {
		s.Fatal("Failed to click next on the CHOOBE screen: ", err)
	}

	s.Log("Verifying that file sync is enabled by default then pressing next")
	if err := oobeConn.WaitForExprFailOnErrWithTimeout(ctx, "OobeAPI.screens.ChoobeDrivePinningScreen.isReadyForTesting()", maxUITimeout); err != nil {
		s.Fatal("Failed to wait for the choobe drive pinning screen to be ready for testing: ", err)
	}

	if err := oobeConn.WaitForExprFailOnErr(ctx, "OobeAPI.screens.ChoobeDrivePinningScreen.isFileSyncEnabled()"); err != nil {
		s.Fatal("Failed as file sync is disabled when it should be enabled by default: ", err)
	}

	if err := oobeConn.Eval(ctx, "OobeAPI.screens.ChoobeDrivePinningScreen.clickNext()", nil); err != nil {
		s.Fatal("Failed to click next on the Drive pinning CHOOBE screen: ", err)
	}

	ui := uiauto.New(tconn)
	if ui.LeftClick(nodewith.Role(role.Button).Name("Get started"))(ctx); err != nil {
		s.Fatal("Failed to click the Get started button: ", err)
	}

	tconn, err = cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create the test API connection: ", err)
	}
	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	filesApp, err := filesapp.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Could not launch the Files app: ", err)
	}

	if err := filesApp.WaitUntilExists(nodewith.Name("File sync is on"))(ctx); err != nil {
		s.Fatal("Failed to wait for File sync is on cloud button: ", err)
	}
}
