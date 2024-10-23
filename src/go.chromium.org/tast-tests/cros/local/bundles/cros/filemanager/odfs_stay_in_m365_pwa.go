// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package filemanager

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/chrome/credconfig"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ms365"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/office"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/filemanager"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/onedrive"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type userParamStayInM365 struct {
	username            string
	password            string
	isConsumerMicrosoft bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:           OdfsStayInM365Pwa,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		LacrosStatus:   testing.LacrosVariantUnneeded,
		Desc:           "Verifies that we stay within the M365 PWA when creating a new file or opening an existing file from M365",
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
			"onedrive.accountPool",
			"onedrive.managedusernamemicrosoft",
			"onedrive.managedpassword",
		},
		Fixture: "onedriveManaged",
		Params: []testing.Param{{
			Name: "consumer",
			Val: userParamStayInM365{
				isConsumerMicrosoft: true,
			},
		}, {
			Name: "commercial",
			Val: userParamStayInM365{
				isConsumerMicrosoft: false,
				username:            "onedrive.managedusernamemicrosoft",
				password:            "onedrive.managedpassword",
			},
		}},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.WebAppInstallForceList{}, pci.Served),
			pci.SearchFlag(&policy.MicrosoftOneDriveMount{}, pci.VerifiedFunctionalityOS),
			pci.SearchFlag(&policy.MicrosoftOneDriveAccountRestrictions{}, pci.Served),
			pci.SearchFlag(&policy.MicrosoftOfficeCloudUpload{}, pci.Served),
		},
	})
}

// maybeDismissOneDriveAd will close any potential ad which might pop up when the user opens their files in OneDrive.
func maybeDismissOneDriveAd(ui *uiauto.Context, oneDriveUIAncestor *nodewith.Finder) uiauto.Action {
	oneDriveAdDialog := nodewith.Role(role.AlertDialog).Ancestor(oneDriveUIAncestor)
	oneDriveCloseAdButton := nodewith.Role(role.Button).Name("Close").Ancestor(oneDriveAdDialog)

	return func(ctx context.Context) error {
		if err := ui.WaitUntilExists(oneDriveUIAncestor)(ctx); err != nil {
			return errors.Wrap(err, "failed to find the OneDrive context")
		}

		if err := ui.EnsureGoneFor(oneDriveAdDialog, 5*time.Second)(ctx); err != nil {
			return ui.LeftClickUntil(oneDriveCloseAdButton, ui.Gone(oneDriveAdDialog))(ctx)
		}
		return nil
	}
}

// findFile will first search & select a file row in the list of files and will
// then go through the list by clicking the down button until the target file
// was found.
func findFile(ui *uiauto.Context, oneDriveUIAncestor, targetFile *nodewith.Finder) uiauto.Action {
	myFileList := nodewith.Role(role.Grid).Name("My files").Ancestor(oneDriveUIAncestor)
	fileItem := nodewith.Role(role.Row).Ancestor(myFileList).Offscreen().First()

	return func(ctx context.Context) error {
		if err := ui.WaitUntilExists(myFileList)(ctx); err != nil {
			return errors.Wrap(err, "failed to find my files")
		}

		if err := ui.Exists(targetFile)(ctx); err == nil {
			return nil
		}

		// Set up keyboard.
		kb, err := input.VirtualKeyboard(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get keyboard")
		}
		defer kb.Close(ctx)

		if err = ui.DoDefault(fileItem)(ctx); err != nil {
			return errors.Wrap(err, "failed to select a file in the list of files")
		}

		if err = ui.RetryUntil(kb.AccelAction("down"), ui.Exists(targetFile))(ctx); err != nil {
			return errors.Wrap(err, "failed to find target file")
		}
		return nil
	}
}

// OdfsStayInM365Pwa verifies that the user stays within the M365 PWA when they
// create new files or open existing files from within the PWA using different
// Microsoft account types (consumer account, M365 business standard license).
func OdfsStayInM365Pwa(ctx context.Context, s *testing.State) {
	var creds credconfig.Creds

	data := s.FixtValue().(*onedrive.FixtureData)
	cr := data.Chrome
	tconn := data.TestAPIConn
	fdms := data.FakeDMS()

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

	param := s.Param().(userParamStayInM365)
	if param.isConsumerMicrosoft {
		accountPool := s.RequiredVar("onedrive.accountPool")
		msCreds, err := credconfig.PickRandomCreds(accountPool)
		if err != nil {
			s.Fatal("Failed to get the user/passwd for Microsoft 365: ", err)
		}
		creds = msCreds
	} else {
		creds.User = s.RequiredVar(param.username)
		creds.Pass = s.RequiredVar(param.password)
	}

	files, err := filesapp.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch Files app: ", err)
	}
	defer files.Close(cleanupCtx)
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_stay_in_m365_pwa")

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

	odfsToken, err := files.GetOdfsFuseboxToken(ctx, cr)
	if err != nil {
		s.Fatal("Failed to get ODFS key: ", err)
	}

	// Delete old files from OneDrive which were created by the test but could not be deleted previously.
	defer func(ctx context.Context) {
		dir := filepath.Join(filemanager.FuseboxDirPath, odfsToken)
		dirEntries, err := onedrive.FindEntriesOlderThanHalfAnHour(dir)
		if err != nil {
			s.Log("Failed to list old files: ", err)
		}

		presentationRegex := regexp.MustCompile(`^Presentation.*\.pptx$`)
		for _, f := range dirEntries {
			if !presentationRegex.MatchString(f.Name()) {
				continue
			}

			testing.ContextLog(ctx, "Deleting old file: ", f.Name())
			if err := os.RemoveAll(filepath.Join(dir, f.Name())); err != nil {
				testing.ContextLogf(ctx, "Failed to remove file: %q - %v", f.Name(), err)
			}
		}
	}(cleanupCtx)

	// Create a new file from within the M365 PWA.
	if err := apps.Launch(ctx, tconn, apps.Microsoft365.ID); err != nil {
		s.Fatal("Failed to launch Microsoft 365: ", err)
	}

	m365Window := nodewith.Role(role.Window).NameContaining("Microsoft 365").ClassName("BrowserFrame")
	m365Context := nodewith.Role(role.RootWebArea).NameContaining("Microsoft 365").Ancestor(m365Window)
	appsButton := nodewith.Role(role.ToggleButton).NameContaining("Apps").Ancestor(m365Context).First()
	powerPointLink := nodewith.Role(role.Link).NameContaining("PowerPoint").Ancestor(m365Context).Focusable()
	newPresentationLink := nodewith.Role(role.Link).NameContaining("blank presentation").Focusable().Ancestor(m365Context)

	ui := uiauto.New(tconn).WithTimeout(30 * time.Second)
	if err := uiauto.Combine("Create a new PowerPoint presentation in M365",
		ui.WaitUntilExists(appsButton),
		ui.DoDefault(appsButton),
		ui.WaitUntilExists(powerPointLink),
		ui.DoDefault(powerPointLink),
		ui.WaitUntilExists(newPresentationLink),
		ui.DoDefault(newPresentationLink),
	)(ctx); err != nil {
		s.Fatal("Failed to create a new PowerPoint presentation from M365: ", err)
	}

	// Verify that the document was opened within the M365 PWA and close the window.
	presentationWindowRegex := regexp.MustCompile(`^Presentation.*\.pptx`)
	fileInfo, err := ui.Info(ctx, ms365.Microsoft365WindowFinderWithRegex(presentationWindowRegex))
	if err != nil || fileInfo == nil {
		s.Fatal("Failed to find the new presentation: ", err)
	}

	fileName := presentationWindowRegex.FindString(fileInfo.Name)
	if err := ms365App.WaitForMicrosoft365EditorWindowAndClose(tconn, fileName)(ctx); err != nil {
		s.Fatal("Failed to close M365 window: ", err)
	}

	// Try to remove the just created file from OneDrive during cleanup.
	defer func(ctx context.Context) {
		fullFilePath := filepath.Join(filemanager.FuseboxDirPath, odfsToken, fileName)
		if err := os.RemoveAll(fullFilePath); err != nil {
			s.Log("Failed to remove the fresh file from OneDrive: ", err)
		}
	}(cleanupCtx)

	if apps.Close(ctx, tconn, apps.Microsoft365.ID); err != nil {
		s.Fatal("Failed to close Microsoft 365: ", err)
	}

	// Re-open the file from M365.
	if err := apps.Launch(ctx, tconn, apps.Microsoft365.ID); err != nil {
		s.Fatal("Failed to launch Microsoft 365: ", err)
	}

	oneDriveUIAncestor := m365Context
	if param.isConsumerMicrosoft {
		oneDriveUIAncestor = nodewith.Role(role.Window).NameRegex(regexp.MustCompile("Microsoft 365.*OneDrive")).ClassName("BrowserFrame")
	}
	oneDriveButton := nodewith.Role(role.ToggleButton).NameContaining("OneDrive").Ancestor(m365Context).First()
	myFilesButton := nodewith.Role(role.Link).NameContaining("My files").Ancestor(oneDriveUIAncestor)
	fileNameButton := nodewith.Role(role.StaticText).Name(fileName).Ancestor(oneDriveUIAncestor)
	if err := uiauto.Combine("Open my files in OneDrive",
		ui.WaitUntilExists(oneDriveButton),
		ui.DoDefault(oneDriveButton),
		maybeDismissOneDriveAd(ui, oneDriveUIAncestor),
		ui.WaitUntilExists(myFilesButton),
		ui.DoDefault(myFilesButton),
		findFile(ui, oneDriveUIAncestor, fileNameButton),
		ui.ScrollToVisible(fileNameButton),
		ui.DoDefault(fileNameButton),
	)(ctx); err != nil {
		s.Fatal("Failed to open the just created file in OneDrive: ", err)
	}

	if err := ms365App.WaitForMicrosoft365EditorWindowAndClose(tconn, fileName)(ctx); err != nil {
		s.Fatal("Failed to close M365 window: ", err)
	}
}
