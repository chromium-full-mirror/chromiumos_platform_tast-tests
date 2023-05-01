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

	"chromiumos/tast/local/apps"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/filesapp"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/role"
	"chromiumos/tast/local/cryptohome"
	"chromiumos/tast/local/input"
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

// VerifyWarning verifies expected status of a DLP warning dialog.
// If shouldAppear is true, waits for the warning to appear, otherwise ensures it doesn't appear.
func VerifyWarning(ctx context.Context, ui *uiauto.Context, shouldAppear bool) error {
	dialogNode := nodewith.NameRegex(regexp.MustCompile("(Copy|Transfer) confidential file?"))
	if shouldAppear {
		if err := ui.WaitUntilExists(dialogNode)(ctx); err != nil {
			return errors.Wrap(err, "failed to wait for DLP warning")
		}
	} else {
		if err := ui.EnsureGoneFor(dialogNode, 10*time.Second)(ctx); err != nil {
			return errors.Wrap(err, "failed to ensure DLP warning gone")
		}
	}
	return nil
}

// AcceptWarningAndVerify accepts the DLP warning dialog and verifies that the file was copied.
// Assumes that Files App is opened in the correct directory.
func AcceptWarningAndVerify(ctx context.Context, ui *uiauto.Context, tconn *chrome.TestConn, kb *input.KeyboardEventWriter, filename string) error {
	filesApp, err := filesapp.App(ctx, tconn, apps.FilesSWA.ID)
	if err != nil {
		return errors.Wrap(err, "failed to connect to existing Files app")
	}

	// Proceed with the paste.
	if err := kb.Accel(ctx, "Enter"); err != nil {
		return errors.Wrap(err, "failed to hit Enter")
	}

	if err := uiauto.Combine("Ensure file was copied",
		filesApp.WaitForFile(filename),
	)(ctx); err != nil {
		return errors.Wrap(err, "file was not copied while it should")
	}
	return nil
}

// CancelWarningAndVerify cancels the DLP warning dialog and verifies that the file wasn't copied.
// Assumes that Files App is opened in the correct directory.
func CancelWarningAndVerify(ctx context.Context, ui *uiauto.Context, tconn *chrome.TestConn, kb *input.KeyboardEventWriter, filename string) error {
	filesApp, err := filesapp.App(ctx, tconn, apps.FilesSWA.ID)
	if err != nil {
		return errors.Wrap(err, "failed to connect to existing Files app")
	}

	// Cancel the paste.
	if err := kb.Accel(ctx, "Esc"); err != nil {
		return errors.Wrap(err, "failed to hit Esc")
	}

	if err := uiauto.Combine("Ensure file wasn't copied",
		filesApp.EnsureFileGone(filename, 10*time.Second),
	)(ctx); err != nil {
		return errors.Wrap(err, "file was copied while it shouldn't")
	}
	return nil
}
