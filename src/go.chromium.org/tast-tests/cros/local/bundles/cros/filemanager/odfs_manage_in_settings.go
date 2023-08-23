// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package filemanager

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/cloudupload"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ms365"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/filesconsts"
	"go.chromium.org/tast-tests/cros/local/onedrive"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         OdfsManageInSettings,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Verifies that the user can connect and disconnect from OneDrive from Settings",
		BugComponent: "b:1199143",
		Timeout:      5 * time.Minute,
		Contacts: []string{
			"chromeos-files-syd@google.com",
			"austinct@chromium.org",
		},
		SoftwareDeps: []string{
			"chrome",
			"chrome_internal",
		},
		Attr: []string{
			"group:mainline",
			"group:hw_agnostic",
			"informational",
		},
		VarDeps: []string{
			"onedrive.accountPool",
		},
		Params: []testing.Param{{
			Fixture: "onedrive",
		}, {
			Name:              "lacros",
			Fixture:           "onedriveLacros",
			ExtraSoftwareDeps: []string{"lacros"},
		}},
	})
}

// OdfsManageInSettings tests that the user can connect and disconnect from
// OneDrive from Settings.
func OdfsManageInSettings(ctx context.Context, s *testing.State) {
	accountPool := s.RequiredVar("onedrive.accountPool")
	data := s.FixtValue().(*onedrive.FixtureData)
	tconn := data.TestAPIConn

	filesApp, err := filesapp.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to open Files App: ", err)
	}

	ms365, err := ms365.App(ctx, tconn, accountPool)
	if err != nil {
		s.Fatal("Failed to get instance of Ms365: ", err)
	}

	settingsApp := ossettings.New(tconn)
	cloudUpload := cloudupload.App(tconn, filesconsts.OneDrive)
	oneDriveDisconnectedLink := nodewith.Name("OneDrive Disconnected").Role(role.Link)
	connectAccountButton := nodewith.Name("Connect account").Role(role.Button)
	if err := uiauto.Combine("Connect to OneDrive via the Files settings page",
		filesApp.ClickMoreMenuItem("Files settings"),
		settingsApp.WaitUntilExists(oneDriveDisconnectedLink),
		settingsApp.LeftClickUntil(oneDriveDisconnectedLink, settingsApp.Exists(connectAccountButton)),
		settingsApp.LeftClick(connectAccountButton),
		cloudUpload.WaitConnectToOneDriveDialogAndClickConnect(),
		ms365.LoginToMicrosoft365(cloudupload.OneDriveConnectedDialog, false /*=skipPassword*/),
		cloudUpload.WaitOneDriveConnectedDialogAndClickClose(),
	)(ctx); err != nil {
		s.Fatal("Failed connect to OneDrive via the Files settings page: ", err)
	}

	if err := settingsApp.Close(ctx); err != nil {
		s.Fatal("Failed to close Settings app: ", err)
	}

	if err := filesApp.OpenOneDrive()(ctx); err != nil {
		s.Fatal("Failed to navigate to OneDrive in Files App: ", err)
	}

	oneDriveConnectedLink := nodewith.NameStartingWith("OneDrive Signed in as").Role(role.Link)
	disconnectButton := nodewith.Name("Disconnect").Role(role.Button)
	if err := uiauto.Combine("Disconnect from OneDrive via the Files settings page",
		filesApp.ClickMoreMenuItem("Files settings"),
		settingsApp.WaitUntilExists(oneDriveConnectedLink),
		settingsApp.LeftClickUntil(oneDriveConnectedLink, settingsApp.Exists(disconnectButton)),
		settingsApp.LeftClick(disconnectButton),
	)(ctx); err != nil {
		s.Fatal("Failed disconnect from OneDrive via the Files settings page: ", err)
	}

	if err := settingsApp.Close(ctx); err != nil {
		s.Fatal("Failed to close Settings app: ", err)
	}

	if err := filesApp.WaitUntilGone(nodewith.Name(filesapp.OneDrive).Role(role.TreeItem))(ctx); err != nil {
		s.Fatal("Failed to wait for the OneDrive mount to disappear: ", err)
	}
}
