// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package filesinternals support for controlling chrome://files-internals page.
package filesinternals

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/internal/driver"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// FilesInternals represents an instance of the chrome://files-internals
type FilesInternals struct {
	ui   *uiauto.Context
	cr   *chrome.Chrome
	kb   *input.KeyboardEventWriter
	conn *driver.Conn
}

// Start opens a chrome window with chrome://files-internals and returns an instance of the FilesInternals.
func Start(ctx context.Context, tconn *chrome.TestConn, cr *chrome.Chrome) (*FilesInternals, error) {
	ui := uiauto.New(tconn)
	kb, err := input.Keyboard(ctx)
	if err != nil {
		return nil, err
	}

	br := cr.Browser()
	conn, err := br.NewTab(ctx, "chrome://files-internals")
	if err != nil {
		return nil, errors.Wrap(err, "failed to open chrome://files-internals tab")
	}
	return &FilesInternals{ui: ui, cr: cr, kb: kb, conn: conn}, nil
}

func (fi *FilesInternals) waitAndCloseFilesApp() uiauto.Action {
	return func(ctx context.Context) error {
		tconn, err := fi.cr.TestAPIConn(ctx)
		if err != nil {
			return err
		}

		filesApp, err := filesapp.App(ctx, tconn, apps.FilesSWA.ID)
		if err != nil {
			return err
		}

		err = filesApp.Close(ctx)
		if err != nil {
			return err
		}
		return nil
	}
}

// ClearAlwaysMoveOneDrive clicks on the chrome://files-internals button to clear the "Always move to OneDrive" prefs.
func (fi *FilesInternals) ClearAlwaysMoveOneDrive() uiauto.Action {
	return uiauto.Combine("Clear always move to OneDrive",
		func(ctx context.Context) error {
			testing.ContextLog(ctx, "ClearAlwaysMoveOneDrive")
			return fi.conn.Eval(ctx, `document.querySelector('#clear-office-always-move-to-onedrive').click()`, nil)
		},
		fi.waitAndCloseFilesApp(),
	)
}

// ClearAlwaysMoveGoogleDrive clicks on the chrome://files-internals button to clear the "Always move to OneDrive" prefs.
func (fi *FilesInternals) ClearAlwaysMoveGoogleDrive() uiauto.Action {
	return uiauto.Combine("Clear always move to Google Drive",
		func(ctx context.Context) error {
			testing.ContextLog(ctx, "ClearAlwaysMoveGoogleDrive")
			return fi.conn.Eval(ctx, `document.querySelector('#clear-office-always-move-to-drive').click()`, nil)
		},
		fi.waitAndCloseFilesApp(),
	)
}

// ClearOfficeFileHandlers clicks on the chrome://files-internals button to clear the "Clear Office file handlers" prefs.
func (fi *FilesInternals) ClearOfficeFileHandlers() uiauto.Action {
	return func(ctx context.Context) error {
		testing.ContextLog(ctx, "ClearOfficeFileHandlers")
		return fi.conn.Eval(ctx, `document.querySelector('#clear-office-file-handlers').click()`, nil)

	}
}
