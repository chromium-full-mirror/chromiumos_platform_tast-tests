// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package filemanager

import (
	"context"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/filemanager/bulkpinning"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/drivefs"
	"go.chromium.org/tast-tests/cros/local/filemanager"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/network"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/timing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         BulkPinningHostedAvailableOffline,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Verify that after bulk pinning enabled, hosted files can be opened offline",
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
		TestBedDeps: []string{tbdep.Cbx(true)},
		SearchFlags: []*testing.StringPair{{
			Key:   "feature_id",
			Value: "screenplay-cc8d1514-a3bd-4c33-8a9b-060bbaba9b49",
		}, {
			Key:   "feature_id",
			Value: "screenplay-9390581d-007a-4cc4-a9fe-1dbaed8a5aaf",
		}},
		Timeout: 10 * time.Minute,
		Params: []testing.Param{{
			Name:    "ash",
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
	})
}

var googleDocsBrowserTabFinder = nodewith.Role(role.Tab).NameContaining("Google Docs")

func BulkPinningHostedAvailableOffline(ctx context.Context, s *testing.State) {
	fixt := s.FixtValue().(*drivefs.FixtureData)
	apiClient := fixt.APIClient
	driveFsClient := fixt.DriveFs
	tconn := fixt.TestAPIConn

	// Give the Drive API enough time to remove the file.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()
	defer driveFsClient.SaveLogsOnError(cleanupCtx, s.HasError)
	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	testDocName := filemanager.GenerateTestFileName("t")
	file, err := apiClient.CreateBlankGoogleDoc(ctx, testDocName, []string{"root"})
	if err != nil {
		s.Fatal("Failed to create blank google doc: ", err)
	}
	defer func() {
		if err := apiClient.RemoveFileByID(cleanupCtx, file.Id); err != nil {
			s.Log("Failed to remove file by ID: ", err)
		}
	}()

	br, closeBrowser, err := browserfixt.SetUp(ctx, s.FixtValue().(*drivefs.FixtureData).Chrome, s.Param().(browser.Type))
	if err != nil {
		s.Fatal("Failed to set up browser: ", err)
	}
	defer closeBrowser(cleanupCtx)

	// Verify the Docs service worker gets registered. When LaCrOS is enabled, the
	// service worker will get cached in LaCrOS and the connection to it happens
	// via crosapi.
	if err := verifyDocsServiceWorkerCached(ctx, br); err != nil {
		s.Fatal("Failed verifying Docs service worker is cached: ", err)
	}

	if err := bulkpinning.EnableBulkPinningFromSettings(fixt.Chrome, tconn)(ctx); err != nil {
		s.Fatal("Failed to enable bulk pinning: ", err)
	}

	files, err := filesapp.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch Files app: ", err)
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to create a keyboard: ", err)
	}

	testDocNameWithExt := testDocName + ".gdoc"
	if err := uiauto.Combine("wait for file to available offline",
		waitForFileToReceiveAvailableOffline(files, testDocNameWithExt),
		// Search for the file name to avoid files created during the test run from
		// pushing the node down in the list view. This also needs to be done online
		// as Drive search requires an online connection.
		files.Search(kb, testDocName),
	)(ctx); err != nil {
		s.Fatal("Failed to wait for gdoc to be available offline: ", err)
	}

	if err := network.ExecFuncOnChromeOffline(ctx, openFileAndEnsureTitleContainsDocName(files, tconn, testDocNameWithExt)); err != nil {
		s.Fatal("Failed verifying Google doc is available offline: ", err)
	}
}

// openFileAndEnsureTitleContainsDocName double clicks the file that has been
// searched for then ensures it opens in Google Docs (this action is completed
// whilst the device is offline).
func openFileAndEnsureTitleContainsDocName(files *filesapp.FilesApp, tconn *chrome.TestConn, testDocName string) uiauto.Action {
	ui := uiauto.New(tconn)
	googleDriveWindow := nodewith.Role(role.Tab).NameContaining("Google Docs")
	docsFileName := nodewith.Role(role.StaticText).Ancestor(googleDriveWindow).First()

	return uiauto.Combine("search for doc and double click it",
		// Double click the search result.
		files.DoubleClick(nodewith.Role(role.ListBoxOption).NameContaining(testDocName)),
		// Wait for the Google Drive window to appear.
		ui.WaitUntilExists(docsFileName),
	)
}

// verifyDocsServiceWorkerCached continuously checks
// chrome://serviceworker-interals until the Docs service worker has been
// registered.
func verifyDocsServiceWorkerCached(ctx context.Context, br *browser.Browser) error {
	// Open up docs.google.com to kick off the service worker caching mechanism.
	docsConn, err := br.NewTab(ctx, "https://docs.google.com")
	if err != nil {
		return errors.Wrap(err, "failed to navigate to google docs")
	}
	defer docsConn.Close()
	if docsConn.WaitForExprFailOnErrWithTimeout(ctx, `document.title === 'Google Docs'`, 10*time.Second); err != nil {
		return errors.Wrap(err, "failed waiting for Google docs to change title")
	}
	// Open up the service worker internals page to monitor exactly when the
	// service worker gets properly registered. Once registered the offline
	// functionality should be available.
	conn, err := br.NewTab(ctx, "chrome://serviceworker-internals")
	if err != nil {
		return errors.Wrap(err, "failed to navigate to serviceworker-internals")
	}
	defer conn.Close()
	timingCtx, timer := timing.Start(ctx, "caching_docs_service_worker")
	defer timer.End()
	return conn.WaitForExprFailOnErrWithTimeout(timingCtx, `
		(() => {
			for (const scope of document.querySelectorAll('.serviceworker-scope').values()) {
				if (scope.innerHTML && scope.innerHTML.indexOf("docs.google.com/document") >= 0) {
					return true;
				}
			}
			return false;
		})()
	`, 10*time.Minute)
}

// waitForFileToReceiveAvailableOffline continuously navigates between Downloads
// and Drive until the file receives the Available offline tick.
func waitForFileToReceiveAvailableOffline(files *filesapp.FilesApp, fileName string) uiauto.Action {
	fileRow := nodewith.Role(role.ListBoxOption).NameContaining(fileName)
	availableOfflineIcon := nodewith.ClassNameRegex(regexp.MustCompile("(inline-status|tast-inline-status)")).Name("Available offline").Ancestor(fileRow)

	return func(ctx context.Context) error {
		timingCtx, timer := timing.Start(ctx, "waiting_for_file_to_be_offline")
		defer timer.End()
		return files.WithTimeout(10*time.Minute).RetryUntil(uiauto.Combine("navigate away and back to drive",
			files.OpenDownloads(),
			files.OpenDrive(),
			files.PerformActionAndRetryMaximizedOnFail(files.WaitForFile(fileName)),
		), files.WithTimeout(5*time.Second).WaitUntilExists(availableOfflineIcon))(timingCtx)
	}
}
