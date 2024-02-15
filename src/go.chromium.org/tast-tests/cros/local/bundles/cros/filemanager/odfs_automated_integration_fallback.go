// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package filemanager

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/cloudupload"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ms365"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/filesconsts"
	"go.chromium.org/tast-tests/cros/local/onedrive"
	"go.chromium.org/tast-tests/cros/local/vdi/fixtures"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         OdfsAutomatedIntegrationFallback,
		LacrosStatus: testing.LacrosVariantNeeded,
		Desc:         "Verifies that the Microsoft OneDrive integration fallback flow works in case of an error during the automated integration",
		BugComponent: "b:1401215", // ChromeOS > Software > Commercial (Enterprise) > Identity > 3P IdP > Enterprise Clippy
		Timeout:      5 * time.Minute,
		Contacts: []string{
			"cros-commercial-clippy-eng@google.com",
			"lmasopust@google.com",
		},
		SoftwareDeps: []string{
			"chrome",
			"chrome_internal",
		},
		Attr: []string{
			"group:hw_agnostic",
			"group:golden_tier",
		},
		VarDeps: []string{
			"onedrive.accountPool",
			"ui.signinProfileTestExtensionManifestKey",
		},
		Fixture: fixture.FakeDMS,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.MicrosoftOneDriveMount{}, pci.VerifiedValue),
			pci.SearchFlag(&policy.MicrosoftOneDriveAccountRestrictions{}, pci.VerifiedValue),
			pci.SearchFlag(&policy.MicrosoftOfficeCloudUpload{}, pci.Served),
		},
	})
}

func OdfsAutomatedIntegrationFallback(ctx context.Context, s *testing.State) {
	accountPool := s.RequiredVar("onedrive.accountPool")
	odfsDevPath := onedrive.OdfsUnpackedLocation.Value()
	isOdfsDev := len(odfsDevPath) > 0

	fdms, ok := s.FixtValue().(*fakedms.FakeDMS)
	if !ok {
		s.Fatal("Failed to launch fakeDMS")
	}

	// Apply automated OneDrive policies & remove account restrictions.
	pb := policy.NewBlob()
	pb.PolicyUser = fixtures.Username
	pb.AddPolicies([]policy.Policy{
		&policy.MicrosoftOneDriveMount{Val: "automated"},
		&policy.MicrosoftOfficeCloudUpload{Val: "automated"},
		&policy.MicrosoftOneDriveAccountRestrictions{Val: []string{"common"}},
	})
	if err := fdms.WritePolicyBlob(pb); err != nil {
		s.Fatal("Failed to write policies to FakeDMS: ", err)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	opts := []chrome.Option{
		chrome.LoadSigninProfileExtension(s.RequiredVar("ui.signinProfileTestExtensionManifestKey")),
		chrome.DMSPolicy(fdms.URL),
		chrome.EnableFeatures("UploadOfficeToCloud"),
		chrome.ExtraArgs("--disable-sync", "--vmodule=cloud_upload*=3", "--extension-force-channel=dev"),
		chrome.FakeLogin(chrome.Creds{User: fixtures.Username, Pass: fixtures.Password}),
	}
	if isOdfsDev {
		opts = append(opts, chrome.UnpackedExtension(odfsDevPath))
	}

	// Sign in with a test account.
	cr, err := chrome.New(ctx, opts...)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed creating test API connection: ", err)
	}

	files, err := filesapp.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch Files app: ", err)
	}
	defer files.Close(cleanupCtx)
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "odfs_file_crud")

	// Confirm that OneDrive was created in the re-auth required state.
	if err := files.OpenOneDrive()(ctx); err != nil {
		s.Fatal("Failed to open OneDrive: ", err)
	}

	ms365App, err := ms365.App(ctx, tconn, accountPool)
	if err != nil {
		s.Fatal("Failed to get instance of Ms365: ", err)
	}

	notificationRoot := nodewith.NameContaining("OneDrive setup failed").Role(role.AlertDialog)
	manuallyConnectButton := nodewith.Role(role.Button).Ancestor(notificationRoot).Focusable().Name("Manually connect to OneDrive")
	cloudUpload := cloudupload.App(tconn, filesconsts.OneDrive)
	ui := uiauto.New(tconn)

	if err := uiauto.Combine("Initiate OneDrive setup by clicking on the notification",
		ui.WaitUntilExists(manuallyConnectButton),
		ui.DoDefault(manuallyConnectButton),
		cloudUpload.WaitConnectToOneDriveDialogAndClick(cloudupload.Next),
		ms365App.LoginToMicrosoft365(cloudupload.OneDriveConnectedDialog, false /*=skipPassword*/),
		cloudUpload.WaitOneDriveConnectedDialogAndClickClose(),
	)(ctx); err != nil {
		s.Fatal("Failed to initiate OneDrive setup through notification: ", err)
	}
}
