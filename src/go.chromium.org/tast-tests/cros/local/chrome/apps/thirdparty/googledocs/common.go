// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package googledocs

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	longUITimeout      = time.Minute      // Used for situations where UI elements that need more time to appear.
	pageLoadTimeout    = 30 * time.Second // Used for waiting for page to load.
	shortUITimeout     = 5 * time.Second  // Used for situations where UI response are faster.
	saveToDriveTimeout = 5 * time.Second  // saveToDriveTimeout indicates the maximum waiting time for Google to save to Drive.
	retryTimes         = 3                // Used for some operations that need to be retried.
)

// waitForDocumentSaved waits for the document state to "Saved to Drive".
func waitForDocumentSaved(tconn *chrome.TestConn, appName string) action.Action {
	ui := uiauto.New(tconn)
	webArea := nodewith.NameContaining(appName).Role(role.RootWebArea)
	documentSavedState := nodewith.NameContaining("Document status: Saved to Drive").Role(role.Button).Ancestor(webArea)
	return func(ctx context.Context) error {
		startTime := time.Now()
		if err := ui.WithTimeout(saveToDriveTimeout).WaitUntilExists(documentSavedState)(ctx); err != nil {
			unableToLoadDialog := nodewith.Name("Unable to load file").Role(role.Dialog)
			if loadFileErr := ui.Exists(unableToLoadDialog)(ctx); loadFileErr == nil {
				return errors.New("unable to load file")
			}
			testing.ContextLog(ctx, "Failed to wait for document saved within ", saveToDriveTimeout)
		} else {
			testing.ContextLog(ctx, "Saved to drive in ", time.Now().Sub(startTime))
		}
		return nil
	}
}

func waitForFieldTextToBe(tconn *chrome.TestConn, finder *nodewith.Finder, expectedText string) action.Action {
	ui := uiauto.New(tconn)
	return ui.WithInterval(400*time.Millisecond).RetrySilently(5,
		func(ctx context.Context) error {
			nodeInfo, err := ui.Info(ctx, finder)
			if err != nil {
				return err
			}
			if nodeInfo.Value != expectedText {
				return errors.Errorf("failed to validate input value: got: %s; want: %s", nodeInfo.Value, expectedText)
			}
			return nil
		})
}

// DeleteDocsOrSlidesContent used for clean the test environment
func DeleteDocsOrSlidesContent(ctx context.Context, tconn *chrome.TestConn, appType string) error {
	var webArea = nodewith.NameContaining(appType).Role(role.RootWebArea)
	ui := uiauto.New(tconn)
	application := nodewith.Role(role.Application).Ancestor(webArea)
	menu := nodewith.Role(role.Menu).Ancestor(application)
	editButton := nodewith.Name("Edit").Role(role.MenuItem).Ancestor(application)
	selectAllMenuItem := nodewith.NameContaining("Select all").Role(role.MenuItem)
	delete := nodewith.NameContaining("Delete").Role(role.MenuItem)

	if err := uiauto.NamedCombine("delete all content",
		cuj.ExpandMenu(tconn, editButton, menu, 200),
		ui.DoDefaultUntil(selectAllMenuItem, ui.WithTimeout(shortUITimeout).WaitUntilGone(selectAllMenuItem)),
		cuj.ExpandMenu(tconn, editButton, menu, 200),
		ui.DoDefaultUntil(delete, ui.WithTimeout(shortUITimeout).WaitUntilGone(delete)),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to delete all the content")
	}
	return nil
}
