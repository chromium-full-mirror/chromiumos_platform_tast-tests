// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package filemanager

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/cloudupload"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ms365"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/filesconsts"
	"go.chromium.org/tast-tests/cros/local/onedrive"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         OdfsAccountRestrictions,
		LacrosStatus: testing.LacrosVariantNeeded,
		Desc:         "Verifies that the Enterprise Clippy account restrictions are working",
		BugComponent: "b:1199143",
		Timeout:      5 * time.Minute,
		Contacts: []string{
			"cros-commercial-clippy-eng@google.com",
			"lmasopust@google.com",
		},
		SoftwareDeps: []string{
			"chrome",
			"chrome_internal",
			"drivefs",
		},
		Attr: []string{
			"group:hw_agnostic",
			"group:golden_tier",
		},
		VarDeps: []string{
			"onedrive.accountPool",
		},
		Fixture: "onedriveManaged",
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.MicrosoftOneDriveAccountRestrictions{}, pci.VerifiedValue),
			pci.SearchFlag(&policy.MicrosoftOneDriveMount{}, pci.Served),
			pci.SearchFlag(&policy.MicrosoftOfficeCloudUpload{}, pci.Served),
		},
	})
}

// OdfsAccountRestrictions tests that account restrictions will block the user
// from using restricted accounts for the integration. It also ensures that
// restricting the current OneDrive user will remove an existing ODFS mount.
func OdfsAccountRestrictions(ctx context.Context, s *testing.State) {
	accountPool := s.RequiredVar("onedrive.accountPool")
	data := s.FixtValue().(*onedrive.FixtureData)
	cr := data.Chrome
	tconn := data.TestAPIConn
	fdms := data.FakeDMS()

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	files, err := filesapp.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch Files app: ", err)
	}
	defer files.Close(cleanupCtx)
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "odfs_file_crud")

	// Connect to OneDrive.
	ms365App, err := ms365.App(ctx, tconn, accountPool)
	if err != nil {
		s.Fatal("Failed to get instance of Ms365: ", err)
	}
	if err := files.ConnectToOneDrive(ctx, ms365App); err != nil {
		s.Fatal("Failed to connect to OneDrive: ", err)
	}

	// Restrict accounts to the azure-cros.com domain.
	if err := policyutil.ServeAndRefresh(ctx, fdms, cr, []policy.Policy{
		&policy.MicrosoftOneDriveAccountRestrictions{Val: []string{"azure-cros.com"}},
		&policy.MicrosoftOneDriveMount{Val: "allowed"},
		&policy.MicrosoftOfficeCloudUpload{Val: "allowed"},
	}); err != nil {
		s.Fatal("Failed to update policies: ", err)
	}

	// Confirm that OneDrive disappeared.
	odfsDir := nodewith.Name(filesapp.OneDrive).Role(role.TreeItem).First()
	if err := files.WaitUntilGone(odfsDir)(ctx); err != nil {
		s.Fatal("OneDrive still exists unexpectedly: ", err)
	}

	// Confirm that connecting to OneDrive fails due to the new account restrictions.
	ui := uiauto.New(tconn)
	cloudUpload := cloudupload.App(tconn, filesconsts.OneDrive)
	msSignInWindow := nodewith.Role(role.RootWebArea).Name("Sign in to your account")
	msErrorMessage := nodewith.NameContaining("Sorry, but we’re having trouble signing you in.").Ancestor(msSignInWindow).Role(role.StaticText)
	if err := uiauto.Combine("Try to connect to OneDrive via Files context menu",
		files.ClickMoreMenuItem("Services", "Connect OneDrive"),
		cloudUpload.WaitConnectToOneDriveDialogAndClick(cloudupload.Next),
		ms365App.InputUserName(ms365App.UserName),
		ui.WaitUntilExists(msErrorMessage),
	)(ctx); err != nil {
		s.Fatal("Failed to navigate through the Connect OneDrive flow: ", err)
	}
}
