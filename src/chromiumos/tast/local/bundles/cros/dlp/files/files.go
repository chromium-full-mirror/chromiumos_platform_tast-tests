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
	"time"

	"chromiumos/tast/ctxutil"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/filesapp"
	"chromiumos/tast/local/cryptohome"
	"chromiumos/tast/testing"
)

const downloadPage = "/download.html"

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

// DownloadFile downloads a file from the local server and verifies it was saved in Downloads directory.
func DownloadFile(ctx context.Context, tconn *chrome.TestConn, br *browser.Browser, dataFS http.FileSystem) error {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 15*time.Second)
	defer cancel()

	// Setup test HTTP server.
	server := httptest.NewServer(http.FileServer(dataFS))
	defer server.Close()

	// Open the local page with the file to download.
	conn, err := br.NewConn(ctx, server.URL+downloadPage)
	if err != nil {
		return errors.Wrap(err, "failed to open browser")
	}
	defer conn.Close()

	testing.ContextLog(ctx, "Opened the browser")

	// The file name is also the ID of the link element, download it.
	if err := conn.Eval(ctx, `document.getElementById('data.txt').click()`, nil); err != nil {
		return errors.Wrap(err, "failed to execute JS expression")
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
