// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testrunners

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/enterpriseconnectors/helpers"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filepicker"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/state"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/screenshot"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"
)

// TestFileAttached tests the correct behavior of the enterprise connectors when attaching a file to a webpage.
// Hereby, it is checked:
// 1. Whether a file is blocked or not
// 2. Whether the correct UI is shown
// 3. Whether the deep scan result is correct (especially relevant for AllowsImmediateDelivery==true)
func TestFileAttached(ctx context.Context, s *testing.State, cr *chrome.Chrome, cryptohomeUsername string) {
	// Verify policy.
	tconnAsh, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}
	devicePolicies, err := policyutil.PoliciesFromDUT(ctx, tconnAsh)
	if err != nil {
		s.Fatal("Failed to get device policies: ", err)
	}
	_, ok := devicePolicies.Chrome["OnFileAttachedEnterpriseConnector"]
	testParams := s.Param().(helpers.TestParams)
	if !ok && testParams.ScansEnabled {
		s.Fatal("Policy isn't set, but should be")
	}
	if ok && !testParams.ScansEnabled {
		s.Fatal("Policy is set, but shouldn't be")
	}

	testFileAttachedForBrowser(ctx, s, cr, cryptohomeUsername)
}

func testFileAttachedForBrowser(ctx context.Context, s *testing.State, cr *chrome.Chrome, cryptohomeUsername string) {
	testParams := s.Param().(helpers.TestParams)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	ui := uiauto.New(tconn)

	// Setup test HTTP server.
	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer server.Close()

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Ensure that there are no windows open.
	if err := ash.CloseAllWindows(ctx, tconn); err != nil {
		s.Fatal("Failed to close all windows: ", err)
	}
	// Ensure that all windows are closed after test.
	defer ash.CloseAllWindows(cleanupCtx, tconn)

	dconn, err := cr.NewConn(ctx, "chrome://policy")
	if err != nil {
		s.Fatal("Failed to connect to chrome: ", err)
	}
	defer dconn.Close()
	defer dconn.CloseTarget(cleanupCtx)

	// Clear Downloads directory.
	downloadsPath, err := cryptohome.DownloadsPath(ctx, cryptohomeUsername)
	if err != nil {
		s.Fatal("Failed to get user's Download path: ", err)
	}
	files, err := ioutil.ReadDir(downloadsPath)
	if err != nil {
		s.Fatal("Failed to get files from Downloads directory")
	}
	for _, file := range files {
		if err := os.RemoveAll(filepath.Join(downloadsPath, file.Name())); err != nil {
			s.Fatal("Failed to remove file: ", file.Name())
		}
	}

	// Need to wait for a valid fcm token, i.e., the proper initialization of the enterprise connectors.
	if testParams.ScansEnabled {
		s.Log("Checking for fcm token")
		if err := helpers.WaitForFCMTokenRegistered(ctx, cr, tconn, server, downloadsPath); err != nil {
			s.Fatal("Failed to wait for FCM token: ", err)
		}
	}

	myFilesPath, err := cryptohome.MyFilesPath(ctx, cryptohomeUsername)
	if err != nil {
		s.Fatal("Failed to get user's MyFiles path: ", err)
	}

	// Create test directory if it does not yet exist.
	testDirPath := filepath.Join(myFilesPath, "test_dir")
	if _, err := os.Stat(testDirPath); os.IsNotExist(err) {
		if err := os.Mkdir(testDirPath, 0755); err != nil {
			s.Fatal("Failed to create test folder: ", err)
		}
		defer os.Remove(testDirPath)
	}

	for _, params := range helpers.GetTestFileParams() {
		if succeeded := s.Run(ctx, params.TestName, func(ctx context.Context, s *testing.State) {
			testFileAttachedForBrowserAndFile(ctx, params, testParams, cr, s, server, testDirPath, ui, tconn)
		}); !succeeded {
			// Stop, if the subtest fails as it might have left the state unusable.
			// It also prevents showing wrong errors on tastboard.
			break
		}
	}
}

func testFileAttachedForBrowserAndFile(
	ctx context.Context,
	params helpers.TestFileParams,
	testParams helpers.TestParams,
	cr *chrome.Chrome,
	s *testing.State,
	server *httptest.Server,
	testDirPath string,
	ui *uiauto.Context,
	tconn *chrome.TestConn,
) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	ulFileName := params.FileName

	shouldBlockUpload := false
	if testParams.ScansEnabled {
		if params.IsUnscannable {
			shouldBlockUpload = !testParams.AllowsUnscannableFiles
		} else {
			shouldBlockUpload = params.IsBad
		}
	}

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "dump_on_error")

	dconnSafebrowsing, err := helpers.GetCleanDconnSafebrowsing(ctx, cr)
	if err != nil {
		s.Fatal("Failed to get clean safe browsing page: ", err)
	}
	defer dconnSafebrowsing.Close()
	defer dconnSafebrowsing.CloseTarget(cleanupCtx)

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "dump_on_error_safe_browsing_page")

	dconn, err := cr.NewConn(ctx, server.URL+"/file_input.html")
	if err != nil {
		s.Fatal("Failed to connect to chrome: ", err)
	}
	defer dconn.Close()
	defer dconn.CloseTarget(cleanupCtx)

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "dump_on_error_file_input_page")

	// Create file at test directory of MyFiles.
	testFileLocation := filepath.Join(testDirPath, ulFileName)
	if _, err := os.Stat(testFileLocation); os.IsNotExist(err) {
		if err := fsutil.CopyFile(s.DataPath(ulFileName), testFileLocation); err != nil {
			s.Fatalf("Failed to copy the file to %s: %v", testFileLocation, err)
		}
		defer os.Remove(testFileLocation)
	}

	// Click on <input type="file">.
	fileInputNodeFinder := nodewith.NameStartingWith("Choose File").Role(role.Button).First()
	if err := ui.LeftClick(fileInputNodeFinder)(ctx); err != nil {
		s.Fatal("Failed to press file input button: ", err)
	}

	files, err := filepicker.Find(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get window of picker: ", err)
	}

	// Open file in test_dir.
	// Note: Use 20s timeout to let the picker retry opening the file.
	if err := uiauto.Combine("open file",
		files.OpenDir("test_dir", "test_dir"),
		files.WithTimeout(20*time.Second).OpenFile(ulFileName),
	)(ctx); err != nil {
		s.Fatal("Failed to open file: ", err)
	}
	if err := ui.WithInterval(200 * time.Millisecond).WithTimeout(5 * time.Second).WaitUntilGone(nodewith.Name("Files").HasClass("WebContentsViewAura"))(ctx); err != nil {
		path := filepath.Join(s.OutDir(), fmt.Sprintf("screenshot-failed-to-close-file-picker-%s.png", params.TestName))
		if err := screenshot.CaptureChrome(ctx, cr, path); err != nil {
			s.Log("Failed to capture screenshot: ", err)
		}
		s.Fatal("Failed to wait for File picker to close: ", err)
	}

	// In report-only mode (AllowsImmediateDelivery) or if scanning is disabled, no dialog should be shown.
	if testParams.AllowsImmediateDelivery || !testParams.ScansEnabled {
		// Check that no dialog will be opened.
		if err := ui.EnsureGoneFor(scanningDialogFinder(), 2*time.Second)(ctx); err != nil {
			s.Fatal("Scanning dialog detected, but none was expected: ", err)
		}
	}

	// First test the deep-scanning verdict.
	if testParams.ScansEnabled {
		// If scans are enabled and the content isn't unscannable, we check the deep scanning verdict.
		if err := helpers.WaitForDeepScanningVerdict(ctx, dconnSafebrowsing, helpers.ScanningTimeOut); err != nil {
			s.Fatal("Failed to wait for deep scanning verdict: ", err)
		}
		if !params.IsUnscannable {
			if err := helpers.VerifyDeepScanningVerdict(ctx, dconnSafebrowsing, params.IsBad, params.IsWarn); err != nil {
				s.Fatal("Failed to verify deep scanning verdict: ", err)
			}
		}
	}

	verifyUIForFileAttached(ctx, shouldBlockUpload, params, testParams, cr, s, server, testDirPath, ui)

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		// Ensure file was or was not attached, by checking javascript output.
		var blocked bool
		if err := dconn.Eval(ctx, `document.getElementsByTagName("input")[0].files.length == 0`, &blocked); err != nil {
			s.Fatal("Failed to determine whether file was blocked: ", err)
		}
		if !testParams.AllowsImmediateDelivery && shouldBlockUpload {
			if !blocked {
				// If a file is attached even though it should have been blocked, this is an immediate error.
				return testing.PollBreak(errors.New("file should have been blocked, but wasn't"))
			}
		} else {
			if blocked {
				// Sometimes it takes time to attach a file, so we do polling here.
				return errors.New("file shouldn't have been blocked, but was")
			}
		}
		return nil
	}, &testing.PollOptions{Timeout: 20 * time.Second, Interval: 5 * time.Second}); err != nil {
		s.Fatal("Failed to verify whether file was correctly attached or blocked: ", err)
	}
}

func scanningDialogFinder() *nodewith.Finder {
	return nodewith.HasClass("DialogClientView").First()
}

func verifyUIForFileAttached(
	ctx context.Context,
	shouldBlockUpload bool,
	params helpers.TestFileParams,
	testParams helpers.TestParams,
	cr *chrome.Chrome,
	s *testing.State,
	server *httptest.Server,
	testDirPath string,
	ui *uiauto.Context) {
	// Check whether the scanning dialog is shown correctly.
	if !testParams.AllowsImmediateDelivery && testParams.ScansEnabled {
		if shouldBlockUpload {
			// Check that a blocked verdict is shown.
			blockedLabelTextFinder := nodewith.Role(role.StaticText).Ancestor(scanningDialogFinder()).NameContaining(params.UlBlockLabel).First()
			if err := ui.WithTimeout(5 * time.Second).WaitUntilExists(blockedLabelTextFinder)(ctx); err != nil {
				s.Fatal("Did not show scan blocked message: ", err)
			}

			// Explicitly close the dialog.
			closeButtonFinder := nodewith.Name("Close").Role(role.Button).Ancestor(scanningDialogFinder()).State(state.Focusable, true)
			if err := ui.WithTimeout(5 * time.Second).WaitUntilExists(closeButtonFinder)(ctx); err != nil {
				s.Fatal("Did not show close button for blocked dialog: ", err)
			}
			// Repeatedly do left click to circumvent problems of missed clicks.
			// This check also waits for the dialog to close.
			if err := ui.LeftClickUntil(closeButtonFinder, ui.WithTimeout(time.Second).WaitUntilGone(scanningDialogFinder()))(ctx); err != nil {
				s.Fatal("Failed to close dialog: ", err)
			}
		} else {
			// Check that the dialog will be closed.
			if err := ui.WithTimeout(5 * time.Second).WaitUntilGone(scanningDialogFinder())(ctx); err != nil {
				s.Fatal("Did not close scanning dialog: ", err)
			}
		}
	}
}
