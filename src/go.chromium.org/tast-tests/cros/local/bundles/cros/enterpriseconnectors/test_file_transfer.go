// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package enterpriseconnectors

import (
	"context"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/enterpriseconnectors/helpers"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         TestFileTransfer,
		LacrosStatus: testing.LacrosVariantUnneeded, // This test tests the files app, which is an ash app, so no lacros needed.
		Desc:         "Enterprise connector test for transferring files between different file systems",
		Timeout:      30 * time.Minute,
		Contacts: []string{
			"cros-enterprise-connectors@google.com",
			"sseckler@google.com",
			"webprotect-eng@google.com",
		},
		BugComponent: "b:1240978",
		SoftwareDeps: []string{
			"chrome",
			"chrome_internal",
		},
		Attr: []string{
			"group:hw_agnostic",
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
		},
		Params: []testing.Param{
			{
				Name:    "scan_enabled_allows_immediate_and_unscannable",
				Fixture: "ashGaiaSignedInProdPolicyWPEnabledAllowExtra",
				Val: helpers.TestParams{
					AllowsImmediateDelivery: true,
					AllowsUnscannableFiles:  true,
					ScansEnabled:            true,
					BrowserType:             browser.TypeAsh,
				},
			},
			{
				Name:    "scan_enabled_blocks_immediate_and_unscannable",
				Fixture: "ashGaiaSignedInProdPolicyWPEnabledBlockExtra",
				Val: helpers.TestParams{
					AllowsImmediateDelivery: false,
					AllowsUnscannableFiles:  false,
					ScansEnabled:            true,
					BrowserType:             browser.TypeAsh,
				},
			},
			{
				Name:    "scan_disabled",
				Fixture: "ashGaiaSignedInProdPolicyWPDisabled",
				Val: helpers.TestParams{
					AllowsImmediateDelivery: true,
					AllowsUnscannableFiles:  true,
					ScansEnabled:            false,
					BrowserType:             browser.TypeAsh,
				},
			},
		},
		Data: []string{
			"download.html", // download.html required for CheckDMTokenRegistered.
			"file_input.html",
			"10ssns.txt",
			"allowed.txt",
			"content.exe",
			"unknown_malware_encrypted.zip",
			"unknown_malware.zip",
		},
	})
}

// TestFileTransfer tests the correct behavior of the enterprise connectors
// when transferring a file across file systems within the files app.
// Hereby, it is checked:
// 1. Whether a file is blocked or not
// 2. Whether the correct UI is shown
// 3. Whether the deep scan result is correct (especially relevant for AllowsImmediateDelivery==true)
func TestFileTransfer(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	// Clear Downloads directory.
	downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
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

	// Verify policy.
	tconnAsh, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}
	devicePolicies, err := policyutil.PoliciesFromDUT(ctx, tconnAsh)
	if err != nil {
		s.Fatal("Failed to get device policies: ", err)
	}
	_, ok := devicePolicies.Chrome["OnFileTransferEnterpriseConnector"]
	testParams := s.Param().(helpers.TestParams)
	if !ok && testParams.ScansEnabled {
		s.Fatal("Policy isn't set, but should be")
	}
	if ok && !testParams.ScansEnabled {
		s.Fatal("Policy is set, but shouldn't be")
	}

	// Setup test HTTP server.
	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer server.Close()

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Ensure that there are no windows open.
	if err := ash.CloseAllWindows(ctx, tconnAsh); err != nil {
		s.Fatal("Failed to close all windows: ", err)
	}
	// Ensure that all windows are closed after test.
	defer ash.CloseAllWindows(cleanupCtx, tconnAsh)

	// Create Browser.
	browserType := testParams.BrowserType
	br, closeBrowser, err := browserfixt.SetUp(ctx, cr, browserType)
	if err != nil {
		s.Fatal("Failed to open the browser: ", err)
	}
	defer closeBrowser(cleanupCtx)

	tconnBrowser, err := br.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to browser's test API: ", err)
	}

	// The browsers sometimes restore some tabs, so we manually close all unneeded tabs.
	closeTabsFunc := browser.CloseAllTabs
	if testParams.BrowserType == browser.TypeLacros {
		// For lacros-Chrome, it should leave a new tab to keep the Chrome process alive.
		closeTabsFunc = browser.ReplaceAllTabsWithSingleNewTab
	}
	if err := closeTabsFunc(ctx, tconnBrowser); err != nil {
		s.Fatal("Failed to close all unneeded tabs: ", err)
	}
	defer closeTabsFunc(cleanupCtx, tconnBrowser)

	dconn, err := br.NewConn(ctx, "chrome://policy")
	if err != nil {
		s.Fatal("Failed to connect to chrome: ", err)
	}
	defer dconn.Close()
	defer dconn.CloseTarget(cleanupCtx)

	// Need to wait for a valid dm token, i.e., the proper initialization of the enterprise connectors.
	if testParams.ScansEnabled {
		s.Log("Checking for dm token")
		downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
		if err != nil {
			s.Fatal("Failed to get user's Download path: ", err)
		}
		if err := helpers.WaitForDMTokenRegistered(ctx, br, tconnAsh, server, downloadsPath); err != nil {
			s.Fatal("Failed to wait for DM token: ", err)
		}
	}

	// Create test directory if it does not yet exist.
	myFilesPath, err := cryptohome.MyFilesPath(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to get user's MyFiles path: ", err)
	}
	testDirPath := filepath.Join(myFilesPath, "test_dir")
	if _, err := os.Stat(testDirPath); os.IsNotExist(err) {
		if err := os.Mkdir(testDirPath, 0755); err != nil {
			s.Fatal("Failed to create test folder: ", err)
		}
		defer os.Remove(testDirPath)
	} else if err != nil {
		s.Fatalf("Failed to stat testDirPath(%s): %s", testDirPath, err)
	}

	// Launch the files app with a formatted USB drive.
	filesApp, closeFilesApp, err := launchFilesAppWithFormattedUsb(ctx, tconnAsh)
	if err != nil {
		s.Fatal("Failed to launch files app: ", err)
	}
	defer closeFilesApp(cleanupCtx)

	for _, testFileParams := range helpers.GetTestFileParams() {
		if succeeded := s.Run(ctx, testFileParams.TestName, func(ctx context.Context, s *testing.State) {
			subTestCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
			defer cancel()
			testFileTransferForFile(subTestCtx, testFileParams, testParams, br, s, testDirPath, tconnAsh, filesApp)
		}); !succeeded {
			// Stop, if the subtest fails as it might have left the state unusable.
			// It also prevents showing wrong errors on tastboard.
			break
		}
	}
}

func testFileTransferForFile(
	ctx context.Context,
	testFileParams helpers.TestFileParams,
	testParams helpers.TestParams,
	br *browser.Browser,
	s *testing.State,
	testDirPath string,
	tconnAsh *chrome.TestConn,
	filesApp *filesapp.FilesApp,
) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	keyboard, err := input.VirtualKeyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer keyboard.Close(cleanupCtx)

	fileName := testFileParams.FileName

	shouldBlockTransfer := false
	if testParams.ScansEnabled && !testParams.AllowsImmediateDelivery {
		if testFileParams.IsUnscannable {
			shouldBlockTransfer = !testParams.AllowsUnscannableFiles
		} else {
			shouldBlockTransfer = testFileParams.IsBad
		}
	}
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "dump_on_error")

	dconnSafebrowsing, err := helpers.GetCleanDconnSafebrowsing(ctx, cr, br)
	if err != nil {
		s.Fatal("Failed to get clean safe browsing page: ", err)
	}
	defer dconnSafebrowsing.Close()
	defer dconnSafebrowsing.CloseTarget(cleanupCtx)

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "dump_on_error_safe_browsing_page")

	// Create file at test directory of MyFiles.
	testFileLocation := filepath.Join(testDirPath, fileName)
	if _, err := os.Stat(testFileLocation); os.IsNotExist(err) {
		if err := fsutil.CopyFile(s.DataPath(fileName), testFileLocation); err != nil {
			s.Fatalf("Failed to copy the file to %s: %v", testFileLocation, err)
		}
		defer os.Remove(testFileLocation)
	} else if err != nil {
		s.Fatalf("Failed to stat testFileLocation(%s): %s", testFileLocation, err)
	}

	ui := uiauto.New(tconnAsh)

	if err := ui.FocusAndWait(filesapp.WindowFinder(apps.FilesSWA.ID))(ctx); err != nil {
		s.Fatal("Failed to focus files app: ", err)
	}

	// Act: Copy + paste file to the destination directory.
	if err := filesApp.OpenDir("test_dir", filesapp.FilesTitlePrefix+"test_dir")(ctx); err != nil {
		s.Fatal("Failed to open Downloads folder: ", err)
	}
	if err := filesApp.CopyFileToClipboard(fileName)(ctx); err != nil {
		s.Fatal("Failed to copy downloaded file to the clipboard: ", err)
	}
	if err := filesApp.OpenUSBDriveWithName("UNTITLED")(ctx); err != nil {
		s.Fatal("Failed to open formatted USB drive: ", err)
	}
	if err := filesApp.FileExists(fileName)(ctx); err == nil {
		if err := filesApp.DeleteFileOrFolder(keyboard, fileName)(ctx); err != nil {
			s.Fatal("Failed to delete file before pasting: ", err)
		}
	}
	if err := filesApp.PasteFileFromClipboard(keyboard)(ctx); err != nil {
		s.Fatal("Failed to paste copied file: ", err)
	}

	// Delete potentially copied file.
	defer filesApp.DeleteFileOrFolder(keyboard, fileName)(cleanupCtx)

	// Verify: Verify copy of file, UI and scanning results.

	// First test the deep-scanning verdict. This waits until scanning is complete if any scanning is expected.
	if testParams.ScansEnabled {
		// If scans are enabled and the content isn't unscannable, we check the deep scanning verdict.
		if err := helpers.WaitForDeepScanningVerdict(ctx, dconnSafebrowsing, helpers.ScanningTimeOut); err != nil {
			s.Fatal("Failed to wait for deep scanning verdict: ", err)
		}
		if !testFileParams.IsUnscannable {
			if err := helpers.VerifyDeepScanningVerdict(ctx, dconnSafebrowsing, testFileParams.IsBad); err != nil {
				s.Fatal("Failed to verify deep scanning verdict: ", err)
			}
		}
	}

	// Wait for transfer to complete. This should not take long, as potential scanning already finished at this point.
	if shouldBlockTransfer {
		if err := waitForFileTransferBlocked(ctx, testFileParams, filesApp); err != nil {
			s.Fatal("Failed to verify that file was blocked: ", err)
		}
	} else {
		if err := filesApp.WithTimeout(10 * time.Second).WaitForFile(fileName)(ctx); err != nil {
			s.Fatal("Failed to verify that file was transferred: ", err)
		}
	}
}

func waitForFileTransferBlocked(
	ctx context.Context,
	testFileParams helpers.TestFileParams,
	filesApp *filesapp.FilesApp,
) error {
	fileName := testFileParams.FileName
	if err := filesApp.EnsureFileGone(fileName, 5*time.Second)(ctx); err != nil {
		return errors.Wrap(err, "failed to verify that file didn't appear")
	}
	return nil
}

func launchFilesAppWithFormattedUsb(ctx context.Context, tconnAsh *chrome.TestConn) (filesApp *filesapp.FilesApp, cancel func(ctx context.Context) error, err error) {
	cleanupCtx := ctx
	ctx, ctxCancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer ctxCancel()

	// Open the Files app to cleanup USB devices. Closed at relaunch or Chrome reset.
	filesApp, err = filesapp.Launch(ctx, tconnAsh)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to launch the Files App")
	}

	// Eject all USB devices if some are still around.
	if err := filesApp.EjectAll()(ctx); err != nil {
		return nil, nil, errors.Wrap(err, "failed to eject")
	}

	// Create the virtual USB device.
	if err := setupVirtualUSBDevice(ctx); err != nil {
		return nil, nil, errors.Wrap(err, "failed to setup virtual USB device")
	}

	success := false

	cleanupFunc := func(ctx context.Context) error {
		// Eject all USB devices before force-unmounting.
		if err := filesApp.EjectAll()(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to eject: ", err)
		}
		if err := filesApp.Close(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to close files app: ", err)
		}
		cleanupVirtualUSBDevice(ctx)
		return nil
	}
	defer func(ctx context.Context) {
		if !success {
			cleanupFunc(ctx)
		}
	}(cleanupCtx)

	if err := filesApp.OpenUSBDrive()(ctx); err != nil {
		return nil, nil, errors.Wrap(err, "failed to open unformatted USB drive")
	}
	if err := filesApp.FormatDevice()(ctx); err != nil {
		return nil, nil, errors.Wrap(err, "failed to format USB drive")
	}
	testing.ContextLog(ctx, "USB drive is formatted")

	success = true
	return filesApp, cleanupFunc, nil
}

// Constants to create a virtual USB drive.
const (
	usbVID          = "dddd"
	usbPID          = "ffff"
	usbManufacturer = "Tast"
	usbProduct      = "VirtualTestUSBDrive"
	usbSerialNumber = "12345"
)

// setupVirtualUSBDevice creates a virtual USB drive.
func setupVirtualUSBDevice(ctx context.Context) error {
	// The file could be absent, so ignoring IsNotExist errors.
	if err := os.Remove("/tmp/backing_file"); err != nil && !os.IsNotExist(err) {
		return errors.Wrap(err, "error when trying to remove /tmp/backing_file")
	}

	if err := testexec.CommandContext(ctx, "modprobe",
		"dummy_hcd").Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "fail to load dummy_hcd module")
	}

	if err := testexec.CommandContext(ctx, "dd", "bs=1024", "count=64", "if=/dev/zero",
		"of=/tmp/backing_file").Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "fail to create temporary backing_file")
	}

	if err := testexec.CommandContext(ctx, "modprobe", "g_mass_storage",
		"file=/tmp/backing_file", "idVendor=0x"+usbVID, "idProduct=0x"+usbPID,
		"iManufacturer="+usbManufacturer, "iProduct="+usbProduct,
		"iSerialNumber="+usbSerialNumber).Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "fail to create virtual USB storage")
	}

	return nil
}

// cleanupVirtualUSBDevice removes previously created virtual USB drive and the backing file.
func cleanupVirtualUSBDevice(ctx context.Context) error {
	testexec.CommandContext(ctx, "modprobe", "g_mass_storage", "-r").Run()

	// The file could be absent, so ignoring IsNotExist errors.
	if err := os.Remove("/tmp/backing_file"); err != nil && !os.IsNotExist(err) {
		return errors.Wrap(err, "error when trying to remove /tmp/backing_file")
	}
	return nil
}
