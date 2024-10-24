// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package filemanager

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/common/chrome/credconfig"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ms365"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/office"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/onedrive"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	outlookURL = "https://outlook.office.com/"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           OdfsOpenFromOutlook,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		LacrosStatus:   testing.LacrosVariantUnneeded,
		Desc:           "Verifies that a file is opened within the M365 PWA if it's opened from a mail in Outlook",
		BugComponent:   "b:1401215", // ChromeOS > Software > Commercial (Enterprise) > Identity > 3P IdP > Enterprise Clippy
		Timeout:        5 * time.Minute,
		Contacts: []string{
			"cros-commercial-clippy-eng@google.com",
			"lmasopust@google.com",
			"emaxx@chromium.org",
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
			"onedrive.managedusernamemicrosoft",
			"onedrive.managedpassword",
		},
		Fixture: "onedriveManaged",
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.WebAppInstallForceList{}, pci.Served),
			pci.SearchFlag(&policy.MicrosoftOneDriveMount{}, pci.VerifiedFunctionalityOS),
			pci.SearchFlag(&policy.MicrosoftOneDriveAccountRestrictions{}, pci.Served),
			pci.SearchFlag(&policy.MicrosoftOfficeCloudUpload{}, pci.Served),
		},
	})
}

// OdfsOpenFromOutlook verifies that links to Microsoft Office files which are
// opened within Outlook will not be opened within a Chrome tab but within a new
// instance of the M365 PWA. For this test to succeed, the test account needs to
// be manually prepared in a way that the Outlook inbox contains 2 mails. one
// with 3 links to office files (Word, Powerpoint, Excel) and one with
// 3 Office file attachments (Word, Powerpoint, Excel).
func OdfsOpenFromOutlook(ctx context.Context, s *testing.State) {
	data := s.FixtValue().(*onedrive.FixtureData)
	cr := data.Chrome
	tconn := data.TestAPIConn
	fdms := data.FakeDMS()

	var creds credconfig.Creds
	creds.User = s.RequiredVar("onedrive.managedusernamemicrosoft")
	creds.Pass = s.RequiredVar("onedrive.managedpassword")

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	// Force-install the M365 PWA and set all Clippy policies appropriately.
	if err := policyutil.ServeAndRefresh(ctx, fdms, cr, []policy.Policy{
		&policy.WebAppInstallForceList{
			Val: []*policy.WebAppInstallForceListValue{
				{
					Url:                    "https://www.microsoft365.com/?from=Homescreen",
					DefaultLaunchContainer: "window",
					CreateDesktopShortcut:  true,
					CustomName:             "",
					FallbackAppName:        "",
					CustomIcon: &policy.WebAppInstallForceListValueCustomIcon{
						Hash: "",
						Url:  "",
					},
				},
			},
		},
		&policy.MicrosoftOneDriveAccountRestrictions{Val: []string{"common"}},
		&policy.MicrosoftOneDriveMount{Val: "automated"},
		&policy.MicrosoftOfficeCloudUpload{Val: "allowed"},
	}); err != nil {
		s.Fatal("Failed to update policies: ", err)
	}
	// Uninstall the force-installed PWA but keeping the ODFS extension installed by serving Clippy policies without the WebAppInstallForceList policy.
	defer policyutil.ServeAndRefresh(cleanupCtx, fdms, cr, []policy.Policy{&policy.MicrosoftOneDriveAccountRestrictions{Val: []string{"common"}},
		&policy.MicrosoftOneDriveMount{Val: "automated"},
		&policy.MicrosoftOfficeCloudUpload{Val: "allowed"}})

	files, err := filesapp.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch Files app: ", err)
	}
	defer files.Close(cleanupCtx)

	// Connect to OneDrive.
	ms365App, err := ms365.AppWithCreds(ctx, tconn, creds)
	if err != nil {
		s.Fatal("Failed to get instance of Ms365: ", err)
	}

	if err := office.ConnectToOneDrive(cr, tconn, files, ms365App)(ctx); err != nil {
		s.Fatal("Failed to connect to OneDrive: ", err)
	}

	if err := files.OpenOneDrive()(ctx); err != nil {
		s.Fatal("Failed to open OneDrive: ", err)
	}

	// Open Outlook.
	conn, err := cr.Browser().NewConn(ctx, outlookURL)
	if err != nil {
		s.Fatal("Failed to open office website: ", err)
	}
	defer conn.Close()
	defer conn.CloseTarget(cleanupCtx)
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_open_from_outlook")

	// Open the most recent Mail from the Inbox which should contain 3 links to Office files.
	outlookContext := nodewith.Role(role.RootWebArea).NameContaining("Outlook")
	messageList := nodewith.Role(role.ListBox).NameContaining("Message").Ancestor(outlookContext)
	linkEmail := nodewith.Role(role.ListBoxOption).NameContaining("Tast Test Outlook Links").Ancestor(messageList)
	linkEmailText := nodewith.Role(role.StaticText).Name("Tast Test Outlook Links:").Ancestor(outlookContext)

	ui := uiauto.New(tconn).WithTimeout(30 * time.Second)
	if err := uiauto.Combine("Open the link email in Outlook",
		ui.WaitUntilExists(outlookContext),
		ui.WaitUntilExists(linkEmail),
		ui.DoDefault(linkEmail),
		ui.WaitUntilExists(linkEmailText),
	)(ctx); err != nil {
		s.Fatal("Failed to open the link email in Outlook: ", err)
	}

	// Open the links in the Mail and wait for M365 to show the file.
	fileNames := []string{"Tast_Outlook_Excel.xlsx", "Tast_Outlook_Powerpoint.pptx", "Tast_Outlook_Word.docx"}
	for _, file := range fileNames {
		emailLink := nodewith.Role(role.Link).Name(file).Ancestor(outlookContext)

		interval := 1 * time.Second
		if err := action.Retry(3, func(ctx context.Context) error {
			// Opening the link with a left click to make sure that we test exactly the same behavior, a user would experience.
			if err = uiauto.Combine("Open the link in the mail",
				ui.WaitUntilExists(emailLink),
				ui.LeftClick(emailLink),
			)(ctx); err != nil {
				return errors.Wrap(err, "failed to open the link in the mail")
			}

			if err := ms365App.WaitForMicrosoft365EditorWindowAndClose(tconn, file)(ctx); err != nil {
				return errors.Wrap(err, "failed to close M365 window")
			}

			return nil
		}, interval)(ctx); err != nil {
			s.Fatal("Failed to click the link after 3 retries: ", err)
		}
	}
}
