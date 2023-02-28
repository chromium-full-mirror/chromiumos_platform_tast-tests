// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dlp

import (
	"context"
	"time"

	"chromiumos/tast/common/fixture"
	"chromiumos/tast/common/pci"
	"chromiumos/tast/common/policy"
	"chromiumos/tast/common/policy/fakedms"
	"chromiumos/tast/ctxutil"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/apps"
	"chromiumos/tast/local/arc"
	"chromiumos/tast/local/bundles/cros/dlp/files"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/ash"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/browser/browserfixt"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/chrome/uiauto/filesapp"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/role"
	"chromiumos/tast/local/input"
	"chromiumos/tast/local/policyutil"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DataLeakPreventionRulesListFilesArc,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test behavior of DataLeakPreventionRulesList policy with file ARC restriction",
		Timeout:      20 * time.Minute,
		Contacts: []string{
			"chromeos-dlp@google.com",
			"aidazolic@google.com",
		},
		BugComponent: "b:892101",
		SoftwareDeps: []string{"chrome"},
		Attr: []string{
			"group:mainline",
			"informational",
		},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DataLeakPreventionRulesList{}, pci.VerifiedFunctionalityOS),
			pci.SearchFlag(&policy.ArcEnabled{}, pci.Served),
		},
		Params: []testing.Param{
			{
				Name:              "arc_container",
				Fixture:           fixture.ChromePolicyLoggedInARC,
				ExtraSoftwareDeps: []string{"android_p"},
			}, {
				Name:              "arc_vm",
				Fixture:           fixture.ChromePolicyLoggedInARC,
				ExtraSoftwareDeps: []string{"android_vm"},
			},
		},
		Data: []string{
			"download.html",
			"data.txt",
		},
	})
}

func DataLeakPreventionRulesListFilesArc(ctx context.Context, s *testing.State) {
	const (
		bootTimeout = 4 * time.Minute
	)

	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fakeDMS := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	keyboard, err := input.VirtualKeyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer keyboard.Close()

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree_error")

	// Update the policy.
	filesArcWarnPolicies := []policy.Policy{&policy.DataLeakPreventionRulesList{
		Val: []*policy.DataLeakPreventionRulesListValue{
			{
				Name:        "Warn before transferring a confidential file to Play files",
				Description: "User should be warned before transferring a confidential file to Play files",
				Sources: &policy.DataLeakPreventionRulesListValueSources{
					Urls: []string{
						"*",
					},
				},
				Destinations: &policy.DataLeakPreventionRulesListValueDestinations{
					Components: []string{
						"ARC",
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
		&policy.ArcEnabled{Val: true, Stat: policy.StatusSet},
	}

	if err := policyutil.ServeAndVerify(ctx, fakeDMS, cr, filesArcWarnPolicies); err != nil {
		s.Fatal("Failed to serve and verify policies: ", err)
	}

	if err := files.ClearDownloads(ctx, cr); err != nil {
		s.Error("Failed to clear Downloads directory: ", err)
	}

	tconnAsh, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}
	// Ensure that there are no windows open.
	if err := ash.CloseAllWindows(ctx, tconnAsh); err != nil {
		s.Fatal("Failed to close all windows: ", err)
	}
	// Ensure that all windows are closed after test.
	defer ash.CloseAllWindows(cleanupCtx, tconnAsh)

	// Setup Arc.
	a, err := arc.NewWithTimeout(ctx, s.OutDir(), bootTimeout)
	if err != nil {
		s.Fatal("Failed to start ARC by policy: ", err)
	}
	defer a.Close(cleanupCtx)

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

	if err := isFileManaged(ctx, ui, tconnAsh, keyboard, files.DlFileName, true); err != nil {
		s.Error("File isn't managed when it should be: ", err)
	}

	if err := pasteFileToPlayfiles(ctx, ui, tconnAsh, keyboard, files.DlFileName); err != nil {
		s.Fatal("Failed to paste the file to Play files: ", err)
	}

	if err := cancelPaste(ctx, ui, tconnAsh, keyboard, files.DlFileName); err != nil {
		s.Fatal("Failed to cancel the paste: ", err)
	}

	if err := pasteFileToPlayfiles(ctx, ui, tconnAsh, keyboard, files.DlFileName); err != nil {
		s.Fatal("Failed to paste the file to Play files: ", err)
	}

	if err := proceedWithPaste(ctx, ui, tconnAsh, keyboard, files.DlFileName); err != nil {
		s.Fatal("Failed to proceed the paste: ", err)
	}

	if err := isFileManaged(ctx, ui, tconnAsh, keyboard, files.DlFileName, false); err != nil {
		s.Error("File is managed when it shouldn't be: ", err)
	}
}

// pasteFileToPlayfiles pastes a file to Play files/Pictures and checks that a DLP warning dialog appears.
func pasteFileToPlayfiles(ctx context.Context, ui *uiauto.Context, tconn *chrome.TestConn, keyboard *input.KeyboardEventWriter, filename string) error {
	filesApp, err := filesapp.App(ctx, tconn, apps.FilesSWA.ID)
	if err != nil {
		return errors.Wrap(err, "failed to connect to existing Files app")
	}

	return uiauto.Combine("Paste the file to Play files/Pictures",
		filesApp.OpenDownloads(),
		filesApp.CopyFileToClipboard(filename),
		filesApp.OpenPlayfiles(),
		filesApp.ClickContextMenuItem("Pictures", "Paste into folder"),
		ui.WaitUntilExists(nodewith.Name("Copy confidential file?")),
	)(ctx)
}

// proceedWithPaste selects the proceed option in the DLP warning dialog. Assumes that Files App is opened in the correct directory.
func proceedWithPaste(ctx context.Context, ui *uiauto.Context, tconn *chrome.TestConn, keyboard *input.KeyboardEventWriter, filename string) error {
	filesApp, err := filesapp.App(ctx, tconn, apps.FilesSWA.ID)
	if err != nil {
		return errors.Wrap(err, "failed to connect to existing Files app")
	}

	// Proceed with the paste.
	if err := keyboard.Accel(ctx, "Enter"); err != nil {
		return errors.Wrap(err, "failed to hit Enter")
	}

	if err := uiauto.Combine("Ensure file was copied",
		filesApp.OpenFile("Pictures"),
		filesApp.WaitForFile(filename),
	)(ctx); err != nil {
		return errors.Wrap(err, "file was not copied while it should")
	}
	return nil
}

// cancelPaste selects the cancel option in the DLP warning dialog. Assumes that Files App is opened in the correct directory.
func cancelPaste(ctx context.Context, ui *uiauto.Context, tconn *chrome.TestConn, keyboard *input.KeyboardEventWriter, filename string) error {
	filesApp, err := filesapp.App(ctx, tconn, apps.FilesSWA.ID)
	if err != nil {
		return errors.Wrap(err, "failed to connect to existing Files app")
	}

	// Cancel the paste.
	if err := keyboard.Accel(ctx, "Esc"); err != nil {
		return errors.Wrap(err, "failed to hit Esc")
	}

	if err := uiauto.Combine("Ensure file wasn't copied",
		filesApp.OpenFile("Pictures"),
		filesApp.EnsureFileGone(filename, 10*time.Second),
	)(ctx); err != nil {
		return errors.Wrap(err, "file was copied while it shouldn't")
	}
	return nil
}

// isFileManaged checks if a file is managed based on whether it has an "Admin policy" context menu item. Assumes that Files App is opened in the correct directory.
func isFileManaged(ctx context.Context, ui *uiauto.Context, tconn *chrome.TestConn, keyboard *input.KeyboardEventWriter, filename string, isManaged bool) error {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 15*time.Second)
	defer cancel()

	filesApp, err := filesapp.App(ctx, tconn, apps.FilesSWA.ID)
	if err != nil {
		return errors.Wrap(err, "failed to connect to existing Files app")
	}

	// Open the context menu.
	if err := filesApp.OpenContextMenu(filename)(ctx); err != nil {
		return errors.Wrap(err, "failed to open the context menu")
	}

	// Ensure the context menu will be closed.
	defer keyboard.Accel(cleanupCtx, "Esc")

	// Check the "Admin policy" menu item.
	adminPolicyNode := nodewith.Name("Review admin policy").Role(role.MenuItem)
	if isManaged {
		if err := ui.WaitUntilExists(adminPolicyNode)(ctx); err != nil {
			return errors.Wrap(err, "failed to find admin policy for a file that should be managed")
		}
	} else {
		if err := ui.WaitUntilGone(adminPolicyNode)(ctx); err != nil {
			return errors.Wrap(err, "found admin policy for a file that shouldn't be managed")
		}
	}
	return nil
}
