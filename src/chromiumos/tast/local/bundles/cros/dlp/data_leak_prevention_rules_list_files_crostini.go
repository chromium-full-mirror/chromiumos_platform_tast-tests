// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dlp

import (
	"context"
	"time"

	"chromiumos/tast/common/pci"
	"chromiumos/tast/common/policy"
	"chromiumos/tast/ctxutil"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/apps"
	"chromiumos/tast/local/bundles/cros/dlp/files"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/ash"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/browser/browserfixt"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/chrome/uiauto/filesapp"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/crostini"
	"chromiumos/tast/local/input"
	"chromiumos/tast/local/policyutil"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DataLeakPreventionRulesListFilesCrostini,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test behavior of DataLeakPreventionRulesList policy with file Crostini restriction",
		Timeout:      7 * time.Minute,
		Contacts: []string{
			"chromeos-dlp@google.com",
			"aidazolic@google.com",
		},
		// ChromeOS > Software > Commercial (Enterprise) > DLP (Data Loss Prevention)
		BugComponent: "b:892101",
		SoftwareDeps: []string{"chrome", "vm_host", "dlc"},
		HardwareDeps: crostini.CrostiniStable,
		Attr: []string{
			"group:mainline",
			"informational",
		},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DataLeakPreventionRulesList{}, pci.VerifiedFunctionalityOS),
		},
		Fixture: "crostiniBusterPolicy",
		Data: []string{
			"download.html",
			"data.txt",
		},
	})
}

func DataLeakPreventionRulesListFilesCrostini(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(crostini.FixtureData).Chrome
	fakeDMS := s.FixtValue().(crostini.FixtureData).FakeDMS
	keyboard := s.FixtValue().(crostini.FixtureData).KB
	tconnAsh := s.FixtValue().(crostini.FixtureData).Tconn

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree_error")

	// Update the policy.
	filesCrostiniWarnPolicy := []policy.Policy{&policy.DataLeakPreventionRulesList{
		Val: []*policy.DataLeakPreventionRulesListValue{
			{
				Name:        "Warn before transferring a confidential file to Linux files",
				Description: "User should be warned before transferring a confidential file to Linux files",
				Sources: &policy.DataLeakPreventionRulesListValueSources{
					Urls: []string{
						"*",
					},
				},
				Destinations: &policy.DataLeakPreventionRulesListValueDestinations{
					Components: []string{
						"CROSTINI",
					},
				},
				Restrictions: []*policy.DataLeakPreventionRulesListValueRestrictions{
					{
						Class: "FILES",
						Level: "WARN",
					},
				},
			},
		},
	},
	}

	if err := policyutil.ServeAndVerify(ctx, fakeDMS, cr, filesCrostiniWarnPolicy); err != nil {
		s.Fatal("Failed to serve and verify policy: ", err)
	}

	if err := files.ClearDownloads(ctx, cr); err != nil {
		s.Fatal("Failed to clear Downloads directory: ", err)
	}

	// Ensure that there are no windows open.
	if err := ash.CloseAllWindows(ctx, tconnAsh); err != nil {
		s.Fatal("Failed to close all windows: ", err)
	}
	// Ensure that all windows are closed after test.
	defer ash.CloseAllWindows(cleanupCtx, tconnAsh)

	// Create Browser.
	br, closeBrowser, err := browserfixt.SetUp(ctx, cr, browser.TypeAsh)
	if err != nil {
		s.Fatal("Failed to open the browser: ", err)
	}
	defer closeBrowser(cleanupCtx)

	tconnBrowser, err := br.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to browser's test API: ", err)
	}

	// The browsers sometimes restore some tabs, so we manually close all unneeded tabs.
	if err := browser.CloseAllTabs(ctx, tconnBrowser); err != nil {
		s.Fatal("Failed to close all unneeded tabs: ", err)
	}
	defer browser.CloseAllTabs(cleanupCtx, tconnBrowser)

	// Close all prior notifications.
	if err := ash.CloseNotifications(ctx, tconnAsh); err != nil {
		s.Fatal("Failed to close notifications: ", err)
	}

	// Download the file.
	if err := files.DownloadFile(ctx, tconnAsh, br, s.DataFileSystem()); err != nil {
		s.Fatal("Failed to download file: ", err)
	}

	// Open the Files app.
	filesApp, err := filesapp.Launch(ctx, tconnAsh)
	if err != nil {
		s.Fatal("Failed to launch the Files App: ", err)
	}
	defer filesApp.Close(cleanupCtx)

	if err := filesApp.OpenDownloads()(ctx); err != nil {
		s.Fatal("Failed to open Downloads: ", err)
	}

	// Start interacting with the UI.
	ui := uiauto.New(tconnAsh)

	if err := files.IsFileManaged(ctx, ui, tconnAsh, keyboard, files.DlFileName, true); err != nil {
		s.Fatal("File isn't managed when it should be: ", err)
	}

	if err := pasteFileToLinuxFiles(ctx, ui, tconnAsh, keyboard, files.DlFileName); err != nil {
		s.Fatal("Failed to paste the file to Linux files: ", err)
	}

	if err := files.CancelWarningAndVerify(ctx, ui, tconnAsh, keyboard, files.DlFileName); err != nil {
		s.Fatal("Failed to cancel the paste: ", err)
	}

	if err := pasteFileToLinuxFiles(ctx, ui, tconnAsh, keyboard, files.DlFileName); err != nil {
		s.Fatal("Failed to paste the file to Linux files: ", err)
	}

	if err := files.AcceptWarningAndVerify(ctx, ui, tconnAsh, keyboard, files.DlFileName); err != nil {
		s.Fatal("Failed to proceed the paste: ", err)
	}

	if err := files.IsFileManaged(ctx, ui, tconnAsh, keyboard, files.DlFileName, false); err != nil {
		s.Fatal("File is managed when it shouldn't be: ", err)
	}
}

// pasteFileToLinuxFiles pastes a file to Linux files and checks that a DLP warning dialog appears.
func pasteFileToLinuxFiles(ctx context.Context, ui *uiauto.Context, tconn *chrome.TestConn, kb *input.KeyboardEventWriter, filename string) error {
	filesApp, err := filesapp.App(ctx, tconn, apps.FilesSWA.ID)
	if err != nil {
		return errors.Wrap(err, "failed to connect to existing Files app")
	}

	return uiauto.Combine("paste the file to Linux files",
		filesApp.OpenDownloads(),
		filesApp.CopyFileToClipboard(filename),
		filesApp.OpenLinuxFiles(),
		filesApp.PasteFileFromClipboard(kb),
		ui.WaitUntilExists(nodewith.Name("Copy confidential file?")),
	)(ctx)
}
