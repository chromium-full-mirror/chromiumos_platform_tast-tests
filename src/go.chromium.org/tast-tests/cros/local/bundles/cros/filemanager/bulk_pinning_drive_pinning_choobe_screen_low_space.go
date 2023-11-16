// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package filemanager

import (
	"context"
	"os"
	"time"

	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/metrics"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/cryptohome/cleanup"
	"go.chromium.org/tast-tests/cros/local/disk"
	"go.chromium.org/tast-tests/cros/local/drivefs"
	"go.chromium.org/tast-tests/cros/local/oobe"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         BulkPinningDrivePinningChoobeScreenLowSpace,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify that the the CHOOBE screen does not show if not enough space",
		BugComponent: "b:167289",
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
			Value: "screenplay-eff7dccd-84e0-451e-87ff-c55c25698648",
		}},
		Timeout: 5 * time.Minute,
	})
}

func BulkPinningDrivePinningChoobeScreenLowSpace(ctx context.Context, s *testing.State) {
	const maxUITimeout = 10 * time.Second

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 10*time.Second)
	defer cancel()

	chromeOptions := []chrome.Option{
		chrome.EnableFeatures("FeatureManagementDriveFsBulkPinning"),
		chrome.DontSkipOOBEAfterLogin(),
		chrome.GAIALoginPool(s.RequiredVar("drivefs.accountPool")),
		chrome.LoadSigninProfileExtension(s.RequiredVar("ui.signinProfileTestExtensionManifestKey")),
		chrome.ExtraArgs("--vmodule=drivefs_pin_manager=1"),
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

	// Fill the disk until there is minimal free space (currently 512MiB) available.
	// Bulk pinning should not be able to toggle on at this point.
	fillFile, err := disk.FillUntil("/home/chronos/user/GCache/v2", cleanup.MinimalFreeSpace)
	if err != nil {
		s.Fatal("Failed to fill disk space: ", err)
	}
	defer func() {
		if err := os.Remove(fillFile); err != nil {
			s.Logf("Failed to remove fill file %s: %v", fillFile, err)
		}
	}()

	// Wait for the `CHOOBEScreenStage` UMA to get emitted, it finishes with the
	// integer "7" which maps to the `kNotEnoughSpace` stage.
	umaName := "FileBrowser.GoogleDrive.BulkPinning.CHOOBEScreenStage"
	histograms, err := metrics.RunAndWaitAll(ctx, tconn, 30*time.Second, func(ctx context.Context) error {
		if err := oobe.AdvanceThroughConsolidatedConsentIfShown(ctx, oobeConn, tconn); err != nil {
			return errors.Wrap(err, "failed to advance through the consolidation screen")
		}

		if err := oobeConn.Eval(ctx, "OobeAPI.advanceToScreen('choobe')", nil); err != nil {
			return errors.Wrap(err, "failed to advance to the choobe screen")
		}

		if err := oobeConn.WaitForExprFailOnErrWithTimeout(ctx, "OobeAPI.screens.ChoobeScreen.isReadyForTesting()", maxUITimeout); err != nil {
			return errors.Wrap(err, "failed to wait for the choobe screen to be ready for testing")
		}

		// TODO(b/295775123): Currently the `isVisible` method on the `DrivePinningScreen`
		// fails as it checks hidden on the null object. For now we catch and return
		// true for this, but crrev.com/c/4865496 will fix this, remove the hack
		// once that has landed and uprevved.
		return oobeConn.WaitForExprFailOnErr(ctx, `new Promise((resolve, reject) => {
			try {
				if (!OobeAPI.screens.ChoobeScreen.isDrivePinningScreenVisible()) {
					resolve()
				} else {
					reject();
				}
			} catch (e) {
				if (e instanceof 'TypeError') {
					resolve(true);
				}
				reject(e);
			}
		})`)
	}, umaName)

	if err != nil {
		s.Fatal("Failed to retrieve histogram data: ", err)
	}

	if len(histograms) != 1 || len(histograms[0].Buckets) != 1 {
		s.Fatalf("Failed to record histogram, got %d want 1", len(histograms))
	}

	if histograms[0].Buckets[0].Min != 7 {
		s.Fatalf("Failed to record the correct value for %q: got %d, want 7 ", umaName, histograms[0].Buckets[0].Min)
	}
}
