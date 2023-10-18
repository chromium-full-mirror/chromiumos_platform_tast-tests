// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package files contains functionality shared by tests that
// exercise DLP files restrictions.
package files

import (
	"context"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// DownloadPage is the suffix of the HTML page with the download.
const DownloadPage = "/download.html"

// DlFileName is the name of the downloaded file used for testing.
const DlFileName = "data.txt"

// ClearDownloads lists and deletes all the files in user's Downloads directory.
func ClearDownloads(ctx context.Context, cr *chrome.Chrome) error {
	// Clear Downloads directory.
	downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	if err != nil {
		return errors.Wrap(err, "failed to get user's Download path")
	}
	files, err := ioutil.ReadDir(downloadsPath)
	if err != nil {
		return errors.Wrap(err, "failed to get files from Downloads directory")
	}
	for _, file := range files {
		if err = os.RemoveAll(filepath.Join(downloadsPath, file.Name())); err != nil {
			return errors.Wrapf(err, "failed to remove file: %s", file.Name())
		}
	}
	return nil
}

// InitiateDownload initiates the file download from a local server.
// The caller should choose the save location and verify the download was successful, as needed.
func InitiateDownload(ctx context.Context, br *browser.Browser, server *httptest.Server) error {
	// Open the local page with the file to download.
	conn, err := br.NewConn(ctx, server.URL+DownloadPage)
	if err != nil {
		return errors.Wrap(err, "failed to open browser")
	}
	defer conn.Close()

	testing.ContextLog(ctx, "Opened the browser")

	// The file name is also the ID of the link element, download it.
	if err := conn.Eval(ctx, `document.getElementById('data.txt').click()`, nil); err != nil {
		return errors.Wrap(err, "failed to execute JS expression")
	}

	return nil
}

// DownloadFile downloads a file from the local server and verifies it was saved in Downloads directory.
func DownloadFile(ctx context.Context, tconn *chrome.TestConn, br *browser.Browser, dataFS http.FileSystem) error {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 15*time.Second)
	defer cancel()

	// Setup test HTTP server.
	server := httptest.NewServer(http.FileServer(dataFS))
	defer server.Close()

	if err := InitiateDownload(ctx, br, server); err != nil {
		return errors.Wrap(err, "failed to initiate download")
	}

	// Open the Files app.
	filesApp, err := filesapp.Launch(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to launch the Files App")
	}
	defer filesApp.Close(cleanupCtx)

	// Check that the file was downloaded.
	if err := uiauto.Combine("Ensure file was downloaded",
		filesApp.OpenDownloads(),
		filesApp.WaitForFile(DlFileName),
	)(ctx); err != nil {
		errors.Wrap(err, "File should have been downloaded, but wasn't")
	}

	testing.ContextLog(ctx, "Downloaded the file")

	return nil
}

// IsFileManaged checks if a file is managed based on whether it has an "Admin policy" context menu item. Assumes that Files App is opened in the correct directory.
func IsFileManaged(ctx context.Context, ui *uiauto.Context, tconn *chrome.TestConn, kb *input.KeyboardEventWriter, filename string, isManaged bool) error {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 15*time.Second)
	defer cancel()

	filesApp, err := filesapp.App(ctx, tconn, apps.FilesSWA.ID)
	if err != nil {
		return errors.Wrap(err, "failed to connect to existing Files app")
	}

	// Open the context menu.
	if err := filesApp.OpenContextMenu(filename)(ctx); err != nil {
		return errors.Wrap(err, "failed to open the context menu")
	}

	// Ensure the context menu will be closed.
	defer kb.Accel(cleanupCtx, "Esc")

	// Check the "Admin policy" menu item.
	adminPolicyNode := nodewith.Name("Review admin policy").Role(role.MenuItem)
	if isManaged {
		if err := ui.WaitUntilExists(adminPolicyNode)(ctx); err != nil {
			return errors.Wrap(err, "failed to find admin policy for a file that should be managed")
		}
	} else {
		if err := ui.WaitUntilGone(adminPolicyNode)(ctx); err != nil {
			return errors.Wrap(err, "found admin policy for a file that shouldn't be managed")
		}
	}
	return nil
}

// VerifyWarning verifies expected status of a DLP warning notification.
// If shouldAppear is true, waits for the warning to appear, otherwise ensures it doesn't appear.
func VerifyWarning(ctx context.Context, tconn *chrome.TestConn, shouldAppear bool) error {
	filesApp, err := filesapp.App(ctx, tconn, apps.FilesSWA.ID)
	if err != nil {
		return errors.Wrap(err, "failed to connect to existing Files app")
	}

	proceedButton := nodewith.Role(role.Button).NameRegex(regexp.MustCompile("(Copy|Transfer) anyway"))
	if shouldAppear {
		if err := filesApp.WaitUntilExists(proceedButton)(ctx); err != nil {
			return errors.Wrap(err, "failed to wait for DLP warning")
		}
	} else {
		if err := filesApp.EnsureGoneFor(proceedButton, 10*time.Second)(ctx); err != nil {
			return errors.Wrap(err, "failed to ensure DLP warning gone")
		}
	}
	return nil
}

// AcceptWarningAndVerify accepts the DLP warning notification and verifies that the file was copied.
// Assumes that Files App is opened in the correct directory.
func AcceptWarningAndVerify(ctx context.Context, ui *uiauto.Context, tconn *chrome.TestConn, filename string) error {
	filesApp, err := filesapp.App(ctx, tconn, apps.FilesSWA.ID)
	if err != nil {
		return errors.Wrap(err, "failed to connect to existing Files app")
	}
	proceedButton := nodewith.Role(role.Button).NameRegex(regexp.MustCompile("(Copy|Transfer) anyway"))

	if err := uiauto.Combine("Click proceed button",
		filesApp.WaitUntilExists(proceedButton),
		ui.DoDefault(proceedButton),
		filesApp.WithTimeout(10*time.Second).WaitForFile(filename),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to proceed the warning")
	}
	return nil
}

// CancelWarningAndVerify cancels the DLP warning notification and verifies that the file wasn't copied.
// Assumes that Files App is opened in the correct directory.
func CancelWarningAndVerify(ctx context.Context, ui *uiauto.Context, tconn *chrome.TestConn, filename string) error {
	filesApp, err := filesapp.App(ctx, tconn, apps.FilesSWA.ID)
	if err != nil {
		return errors.Wrap(err, "failed to connect to existing Files app")
	}

	cancelButton := nodewith.Role(role.Button).NameRegex(regexp.MustCompile("Cancel"))

	if err := uiauto.Combine("Click cancel button",
		filesApp.WaitUntilExists(cancelButton),
		ui.DoDefault(cancelButton),
		filesApp.EnsureFileGone(filename, 10*time.Second),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to cancel the warning")
	}
	return nil
}
