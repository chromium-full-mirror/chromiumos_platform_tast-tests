// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package apps provides general ChromeOS app utilities.
package apps

import (
	"regexp"
	"time"

	"chromiumos/tast/common/action"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/role"
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
