// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package pdfocr provides constants that are used in tests interacting with the
// PDF OCR feature on ChromeOS.
package pdfocr

import (
	"context"
	"fmt"
	"io/fs"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"go.chromium.org/tast-tests/cros/local/a11y"
	"go.chromium.org/tast-tests/cros/local/a11y/tts"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/dlc"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/timing"
)

// Strings used in go.chromium.org/tast-tests/cros/local/bundles/cros/a11y/pdfocr*.go
const (
	// ScreenAI DLC ID.
	ScreenAiDlcID = "screen-ai"
	// Status node message when PDF OCR finished converting image to text. This
	// must be synced with `IDS_PDF_OCR_COMPLETED` defined in the Chromium repo.
	StatusReadyMessage = "This PDF is inaccessible. Text extracted, powered by Google AI"
	// Subpage url in the Settings
	SettingsSubPageURL = "textToSpeech"
	// Toggle menu name in the Settings. This must be synced with
	// `IDS_SETTINGS_PDF_OCR_TITLE` defined in the Chromium repo.
	SettingsToggleName = "Extract text from PDF"
	// Menu entry name in the context menu. This must be synced with
	// `IDS_CONTENT_CONTEXT_PDF_OCR_MENU_OPTION` defined in the Chromium repo.
	ContextMenuName = "Extract text from PDF"
	// Disclaimer message added to the beginning of extracted text. This must be
	// sycned with `IDS_PDF_OCR_RESULT_BEGIN` defined in the Chromium repo.
	DisclaimerMessageStart = "Start of extracted text"
	// Disclaimer message added to the end of extracted text. This must be sycned
	// with `IDS_PDF_OCR_RESULT_END` defined in the Chromium repo.
	DisclaimerMessageEnd = "End of extracted text"
	// Testing PDF's filename
	TestPDFName = "inaccessible-text.pdf"
	// Inaccessible text embedded in an image in the testing PDF file
	TextInPDFImage = "Hello, world!"
	// Three-page document PDF including inaccessible text
	MultiPagePDFName = "inaccessible-text-in-three-page.pdf"
	// JSON file that contains expected texts from `MultiPagePDFName`
	MultiPagePDFExpectedTextJSONName = "inaccessible-text-in-three-page_expected.json"
)

// DlcFailureSetUpData contains necessary objects for PDF OCR tests with dlc
// failure and is returned by SetUpDlcFailure. TDown contains defer functions
// that clean up the testing environment. EnsureDlc contains a defer function
// that checks whether the dlcservice state is restored during cleanup.
type DlcFailureSetUpData struct {
	BackupDir string
	TDown     *a11y.TearDownHelper
}

// SetUpData contains necessary objects for PDF OCR tests and is returned by
// SetUp. TDown contains defer functions that clean up the testing environment.
type SetUpData struct {
	CR     *chrome.Chrome
	Server *httptest.Server
	TConn  *chrome.TestConn
	TDown  *a11y.TearDownHelper
}

// Pages represents a top dictionary object in a JSON file used in
// `PDFOCRMultiPage`.
type Pages struct {
	Pages []Page `json:"pages"`
}

// Page represents a dictionary object within the Pages object in a JSON file
// used in `PDFOCRMultiPage`.
type Page struct {
	Number   int      `json:"number"`
	Expected []string `json:"expected"`
}

// TestStep represents a test step of testing PDF OCR outputs using ChromeVox.
// It consists of a string representing a key command for ChromeVox and a
// tts.SpeechExpectation representing utterances from ChromeVox.
// used in `PDFOCRMultiPage`.
type TestStep struct {
	KeyCommands  []string
	Expectations []tts.SpeechExpectation
}

// SetUpDlcFailure executes common setup code that simulates the screen-ai DLC
// failure and returns a DlcFailureSetUpData. See the documentation for
// DlcFailureSetUpData for more information.
func SetUpDlcFailure(ctx context.Context) (DlcFailureSetUpData, error) {
	setupData := DlcFailureSetUpData{"", &a11y.TearDownHelper{}}

	if err := upstart.StopJob(ctx, dlc.JobName); err != nil {
		return DlcFailureSetUpData{}, errors.Wrapf(err, "failed to stop %q", dlc.JobName)
	}

	// Ensure the test restores the dlcservice state.
	setupData.TDown.Append(func() error {
		ctx, st := timing.Start(ctx, "cleanUp")
		defer st.End()
		return upstart.EnsureJobRunning(ctx, dlc.JobName)
	})

	// Force a DLC Install failure by moving the preload path to a backup directory.
	preloadPath := filepath.Join(dlc.PreloadDir, ScreenAiDlcID)
	if _, err := os.Stat(preloadPath); errors.Is(err, fs.ErrNotExist) {
		return setupData, errors.Wrapf(err, "%s doesn't exist", preloadPath)
	}
	// backupDlcPath needs to be a permanent location that is not wiped by test's
	// setup/teardown; otherwise, it can introduce flakiness to the test. Also,
	// it needs to be unique and used only by this test setup.
	backupDir := fmt.Sprintf("/var/cache/%s-backup", ScreenAiDlcID)
	backupDlcPath := filepath.Join(backupDir, ScreenAiDlcID)
	if _, err := os.Stat(backupDir); err == nil {
		// backupDir exists, so it needs to be removed. backupDir will be created
		// by moveDir() below.
		if err := os.RemoveAll(backupDir); err != nil {
			return setupData, errors.Wrapf(err, "failed to remove %s", backupDir)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		// Return with errors if os.Stat() failed and raised some errors other than
		// whether or not backupDir exists.
		return setupData, errors.Wrapf(err, "os.Stat() on %s raised unexpected errors", backupDir)
	}

	// fsutil.CopyDir() expects backupDlcPath to not exist; otherwise, it returns an error.
	// fsutil.CopyDir() will create backupDlcPath. Use fsutil.CopyDir() instead of moveDir()
	// as errors from moveDir() may lead to incomplete status of the screen-ai dlc, which can
	// engender flakiness and errors in subsequent tests. fsutil.CopyDir() is more robust in
	// that the screen-ai dlc still exists in the preload path even if fsutil.CopyDir() fails.
	if err := fsutil.CopyDir(preloadPath, backupDlcPath); err != nil {
		return setupData, errors.Wrapf(err, "failed to move from %s to %s", preloadPath, backupDlcPath)
	}
	if err := os.RemoveAll(preloadPath); err != nil {
		return setupData, errors.Wrapf(err, "failed to remove %s", preloadPath)
	}
	setupData.BackupDir = backupDir
	setupData.TDown.Append(func() error {
		return moveDir(backupDlcPath, preloadPath)
	})

	if err := upstart.StartJob(ctx, dlc.JobName); err != nil {
		return setupData, errors.Wrap(err, "failed to start dlcservice")
	}

	return setupData, nil
}

// RefreshDlc refreshes DLC by moving the screen-ai DLC to the preload directory.
func RefreshDlc(ctx context.Context, backupDir string) error {
	if err := upstart.StopJob(ctx, dlc.JobName); err != nil {
		return errors.Wrapf(err, "failed to stop %q", dlc.JobName)
	}

	// Move the screen-ai dlc from the backup directory to the preload directory.
	preloadPath := filepath.Join(dlc.PreloadDir, ScreenAiDlcID)
	backupDlcPath := filepath.Join(backupDir, ScreenAiDlcID)

	if _, err := os.Stat(backupDlcPath); errors.Is(err, fs.ErrNotExist) {
		return errors.Wrapf(err, "failed to find %s", backupDlcPath)
	}

	if err := moveDir(backupDlcPath, preloadPath); err != nil {
		return errors.Wrapf(err, "failed to move from %s to %s", backupDlcPath, preloadPath)
	}

	if err := upstart.StartJob(ctx, dlc.JobName); err != nil {
		return errors.Wrap(err, "failed to start dlcservice")
	}

	return nil
}

// SetUpHTTPServer sets up a HTTP server that enables file loading and starts a Chrome browser.
func SetUpHTTPServer(ctx, cleanupCtx context.Context, dataFS http.FileSystem, bt browser.Type) (SetUpData, error) {
	setupData := SetUpData{nil, nil, nil, &a11y.TearDownHelper{}}

	// Setup test HTTP server.
	server := httptest.NewServer(http.FileServer(dataFS))
	setupData.Server = server
	setupData.TDown.Append(func() error {
		server.Close()
		return nil
	})

	// Launch browser with the PDF OCR feature flag for both Ash and Lacros Chrome.
	opts := []chrome.Option{chrome.EnableFeatures("PdfOcr")}
	lacrosConfig := lacrosfixt.NewConfig(lacrosfixt.ChromeOptions(chrome.LacrosEnableFeatures("PdfOcr")))
	cr, err := browserfixt.NewChrome(ctx, bt, lacrosConfig, opts...)
	if err != nil {
		return SetUpData{}, errors.Wrap(err, "failed to start Chrome")
	}
	setupData.CR = cr
	setupData.TDown.Append(func() error {
		cr.Close(cleanupCtx)
		return nil
	})

	return setupData, nil
}

// TurnOnFromContextMenu turns on PDF OCR from the Context menu in PDF Viewer.
func TurnOnFromContextMenu(ctx context.Context, ui *uiauto.Context, pdfRoot *nodewith.Finder) error {
	pdfOCRMenuEntry := nodewith.Name(ContextMenuName).Role(role.MenuItemCheckBox)
	if err := uiauto.Combine("Turn on PDF OCR from the Context Menu",
		ui.WithTimeout(5*time.Second).RightClick(pdfRoot),
		ui.WithTimeout(5*time.Second).WaitUntilExists(pdfOCRMenuEntry),
		ui.WithTimeout(5*time.Second).LeftClick(pdfOCRMenuEntry),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to turn on PDF OCR from the Context Menu")
	}

	return nil
}

// ExpectDownloadFailureUtterance waits to receive the download failure utterance.
func ExpectDownloadFailureUtterance(ctx context.Context, sm *tts.SpeechMonitor) error {
	if err := sm.Consume(ctx, []tts.SpeechExpectation{
		tts.NewStringExpectation("Downloading text recognition files"),
		tts.NewRegexExpectation("Can't download text recognition files*"),
	}); err != nil {
		errors.Wrap(err, "failed to check the ChromeVox announcement for PDF OCR dlc failure")
	}

	return nil
}

// moveDir moves a directory from srcDir to dstDir, using fsutil.MoveFile(), and
// returns retErr if any errors occurred. dstDir must not exist. The mode is
// preserved. The owner is also preserved if the EUID is 0.
func moveDir(srcDir, dstDir string) (retErr error) {
	// Create target dir or return error if already present.
	srcStat, err := os.Stat(srcDir)
	if err != nil {
		retErr = errors.Errorf("failed to stat %s", srcDir)
		return
	}
	if _, err := os.Stat(dstDir); os.IsNotExist(err) {
		if err := os.MkdirAll(dstDir, srcStat.Mode()); err != nil {
			retErr = errors.Wrapf(err, "failed to create %s", dstDir)
			return
		}
	} else {
		retErr = errors.Errorf("target dir %s exists", dstDir)
		return
	}

	// Attempt to remove target dir if moving failed.
	defer func() {
		if retErr != nil {
			if err := os.RemoveAll(dstDir); err != nil {
				retErr = errors.Join(retErr, errors.Wrapf(err, "failed to remove %s", dstDir))
			}
		}
	}()

	// Preserve owner attributes.
	if os.Geteuid() == 0 {
		st := srcStat.Sys().(*syscall.Stat_t)
		if err := os.Chown(dstDir, int(st.Uid), int(st.Gid)); err != nil {
			retErr = errors.Wrapf(err, "failed to change owner of dir %s", dstDir)
			return
		}
	}

	// Move dir content.
	entries, err := ioutil.ReadDir(srcDir)
	if err != nil {
		retErr = errors.Wrapf(err, "failed to read dir %s", srcDir)
		return
	}
	for _, entry := range entries {
		srcPath := filepath.Join(srcDir, entry.Name())
		dstPath := filepath.Join(dstDir, entry.Name())
		stat, err := os.Stat(srcPath)
		if err != nil {
			retErr = errors.Errorf("failed to stat %s", srcPath)
			return
		}
		switch stat.Mode() & os.ModeType {
		case os.ModeDir:
			if err := moveDir(srcPath, dstPath); err != nil {
				retErr = errors.Wrapf(err, "failed to move dir %s", srcPath)
				return
			}
		case os.ModeSymlink:
			if link, err := os.Readlink(srcPath); err != nil {
				retErr = errors.Wrapf(err, "failed to read symlink %s", srcPath)
				return
			} else if err := os.Symlink(link, dstPath); err != nil {
				retErr = errors.Wrapf(err, "failed to create symlink %s", dstPath)
				return
			}
		default:
			if err := fsutil.MoveFile(srcPath, dstPath); err != nil {
				retErr = errors.Wrapf(err, "failed to move file %s", srcPath)
				return
			}
		}
	}

	// Remove srcDir after moving dir content.
	if err := os.RemoveAll(srcDir); err != nil {
		retErr = errors.Wrapf(err, "failed to remove %s", srcDir)
		return
	}
	return
}
