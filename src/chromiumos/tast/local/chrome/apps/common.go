// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package apps provides general Chrome app/website utilities.
package apps

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"chromiumos/tast/common/action"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/browser"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/role"
	"chromiumos/tast/local/cryptohome"
	"chromiumos/tast/testing"
)

// AllowPagePermissions checks whether the page has been blocked.
// If the page has been blocked, unblock camera and microphone permissions.
func AllowPagePermissions(tconn *chrome.TestConn) action.Action {
	ui := uiauto.New(tconn)
	blockedButton := nodewith.NameContaining("This page has been blocked").Role(role.Button)
	dialogRE := regexp.MustCompile("(Camera and microphone|Camera|Microphone) blocked")
	dialogWindow := nodewith.NameRegex(dialogRE).Role(role.Window).ClassName("ContentSettingBubbleContents")
	alwaysAllowButton := nodewith.NameContaining("Always allow").Role(role.RadioButton).Ancestor(dialogWindow)
	doneButton := nodewith.Name("Done").Role(role.Button).Focusable().Ancestor(dialogWindow)
	reloadButton := nodewith.Name("Reload").Role(role.Button).Focusable().First()
	accessButton := nodewith.NameContaining("This page is accessing").Role(role.Button)
	allowPermission := uiauto.NamedCombine("allow page permissions",
		ui.LeftClick(blockedButton),
		ui.LeftClick(alwaysAllowButton),
		ui.LeftClick(doneButton),
		ui.LeftClick(reloadButton),
		ui.WaitUntilExists(accessButton),
	)
	return uiauto.IfSuccessThen(ui.WithTimeout(3*time.Second).WaitUntilExists(blockedButton), allowPermission)
}

// DownloadAppServiceInternals downloads app_service_internal.txt and upload it to the log.
func DownloadAppServiceInternals(ctx context.Context, cr *chrome.Chrome, outDir string) error {
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create test API connection")
	}

	downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	if err != nil {
		return errors.Wrap(err, "failed to get user's Download path")
	}

	// The browser is not closed in the end
	// so that the ui tree dumpter, screenshot and screenrecorder could catch it.
	_, err = browser.Launch(ctx, tconn, cr, "chrome://app-service-internals")
	if err != nil {
		return errors.Wrap(err, "failed to launch browser")
	}

	ui := uiauto.New(tconn)
	saveAs := nodewith.Name("Save as .txt").Role(role.Button)
	if err := ui.LeftClick(saveAs)(ctx); err != nil {
		return errors.Wrap(err, "failed to save app services")
	}

	downloadFileName := "app-service-internals.txt"
	downloadFilePath := filepath.Join(downloadsPath, downloadFileName)

	// Check download file.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if _, err := os.Stat(downloadFilePath); os.IsNotExist(err) {
			return errors.New("failed as file does not yet exist")
		}
		return nil
	}, &testing.PollOptions{Timeout: 5 * time.Second}); err != nil {
		return err
	}

	// Copy the downloaded file to log dir.
	input, err := ioutil.ReadFile(downloadFilePath)
	if err != nil {
		return errors.Wrap(err, "failed to read downloaded app-service-internals.txt")
	}

	err = ioutil.WriteFile(filepath.Join(outDir, downloadFileName), input, 0644)
	if err != nil {
		return errors.Wrap(err, "failed to write to the out dir")
	}
	return nil
}
