// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package filemanager

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/filemanager/bulkpinning"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/filemanager/helpers"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/drivefs"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         BulkPinningHostedFilePinning,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Verify DSS files are pinned and displayed as such after the bulk pinning feature is enabled",
		BugComponent: "b:167289",
		Contacts: []string{
			"chromeos-files-syd@google.com",
			"benreich@google.com",
			"fdegros@google.com",
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
		TestBedDeps: []string{tbdep.Cbx(true)},
		SearchFlags: []*testing.StringPair{{
			Key:   "feature_id",
			Value: "screenplay-3d076472-1afc-4bd8-a1a5-94017965d55e",
		}},
		Params: []testing.Param{{
			Val:     browser.TypeAsh,
			Fixture: "driveFsStartedBulkPinningEnabled",
		},
		// TODO(b/295775123): Uncomment when LaCrOS is enabled on CBX boards.
		// {
		// 	Name:              "lacros",
		// 	Val:               browser.TypeLacros,
		// 	ExtraSoftwareDeps: []string{"lacros"},
		// 	Fixture:           "driveFsStartedWithNativeMessagingAndBulkPinningEnabledLacros",
		// },
		},
		// TODO(crbug/1112246): Reduce timeout period once push notifications fixed.
		Timeout: 10 * time.Minute,
	})
}

func BulkPinningHostedFilePinning(ctx context.Context, s *testing.State) {
	fixt := s.FixtValue().(*drivefs.FixtureData)
	APIClient := fixt.APIClient
	driveFsClient := fixt.DriveFs
	tconn := fixt.TestAPIConn

	// Current refresh period is 2 minutes, leaving buffer for UI propagation.
	// TODO(crbug/1112246): Reduce refresh period once push notifications fixed.
	const filesAppUITimeout = 5 * time.Minute

	// Give the Drive API enough time to remove the file.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()
	defer driveFsClient.SaveLogsOnError(cleanupCtx, s.HasError)
	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	// File created BEFORE bulk pinning is enabled.
	testDocName1 := fmt.Sprintf("before-%d-%d", time.Now().UnixNano(), rand.Intn(10000))
	testDocName1WithExt := fmt.Sprintf("%s.gdoc", testDocName1)

	// File created after bulk pinning is enabled.
	testDocName2 := fmt.Sprintf("after-%d-%d", time.Now().UnixNano(), rand.Intn(10000))
	testDocName2WithExt := fmt.Sprintf("%s.gdoc", testDocName2)

	br, closeBrowser, err := browserfixt.SetUp(
		ctx,
		s.FixtValue().(*drivefs.FixtureData).Chrome,
		s.Param().(browser.Type),
	)
	if err != nil {
		s.Fatal("Failed to set up browser: ", err)
	}
	defer closeBrowser(cleanupCtx)

	// Install Google Docs Offline and Application Launcher for Drive extensions.
	if err := helpers.InstallRequiredExtensions(ctx, br, tconn); err != nil {
		s.Fatal("Failed to install the required extensions: ", err)
	}

	// Create a blank Google doc prior to enabling bulk pinning.
	file1, err := APIClient.CreateBlankGoogleDoc(ctx, testDocName1, []string{"root"})
	if err != nil {
		s.Fatal("Failed to create blank google doc: ", err)
	}
	defer APIClient.RemoveFileByID(cleanupCtx, file1.Id)

	// Launch Files app and check that Drive is accessible.
	filesApp, err := filesapp.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch the Files app: ", err)
	}

	if err := uiauto.Combine("wait for files created then enable bulk pinning",
		filesApp.OpenDrive(),
		filesApp.WithTimeout(filesAppUITimeout).WaitForFile(testDocName1WithExt),
		filesApp.Close,
		bulkpinning.EnableBulkPinningFromSettings(fixt.Chrome, tconn),
	)(ctx); err != nil {
		s.Fatal("Failed to wait for files to be created: ", err)
	}

	// Create another blank Google doc after enabling bulk pinning.
	file2, err := APIClient.CreateBlankGoogleDoc(ctx, testDocName2, []string{"root"})
	if err != nil {
		s.Fatal("Failed to create blank google doc: ", err)
	}
	defer APIClient.RemoveFileByID(cleanupCtx, file2.Id)

	// Launch Files app again as it was previously closed once the file was found
	// to exist.
	filesApp, err = filesapp.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch the Files app: ", err)
	}

	// Verify that both files are displayed as "available offline" and time how
	// long it takes for that to happen.
	pv := perf.NewValues()
	startTime := time.Now()
	if err := uiauto.Combine("wait for files to be pinned",
		filesApp.OpenDrive(),
		filesApp.WithTimeout(filesAppUITimeout).WaitForFile(testDocName2WithExt),
		bulkpinning.AssertFileAvailableOffline(tconn, testDocName1WithExt, filesAppUITimeout),
		bulkpinning.AssertFileAvailableOffline(tconn, testDocName2WithExt, filesAppUITimeout),
	)(ctx); err != nil {
		s.Fatal("Failed to wait for both files to get pinned: ", err)
	}

	pv.Set(perf.Metric{
		Name:      "BulkPinningHostedFilePinning",
		Unit:      "ms",
		Direction: perf.SmallerIsBetter,
	}, float64(time.Since(startTime).Milliseconds()))

	if err := pv.Save(s.OutDir()); err != nil {
		s.Error("Failed saving perf data: ", err)
	}
}
