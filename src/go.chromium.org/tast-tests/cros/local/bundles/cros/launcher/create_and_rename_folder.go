// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package launcher

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ime"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/launcher"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CreateAndRenameFolder,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Renaming Folder In Launcher",
		Contacts: []string{
			"chromeos-launcher@google.com",
			"chromeos-sw-engprod@google.com",
		},
		BugComponent: "b:1288350",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		SearchFlags: []*testing.StringPair{{
			Key:   "feature_id",
			Value: "screenplay-c6476142-1c4a-4e51-8e9b-bb5eaad4bba7",
		}, {
			Key:   "feature_id",
			Value: "screenplay-0fb6c066-83b5-4a53-a425-ba245fb896d5",
		}},
		Params: []testing.Param{{
			Name: "clamshell_mode",
			Val:  launcher.TestCase{TabletMode: false},
		}, {
			Name:              "tablet_mode",
			Val:               launcher.TestCase{TabletMode: true},
			ExtraHardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		}},
	})
}

// CreateAndRenameFolder tests if launcher handles renaming of folder correctly.
func CreateAndRenameFolder(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	testCase := s.Param().(launcher.TestCase)
	tabletMode := testCase.TabletMode

	cr, err := chrome.New(ctx)
	if err != nil {
		s.Fatal("Chrome login failed: ", err)
	}
	defer cr.Close(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer kb.Close(ctx)

	cleanup, err := launcher.SetUpLauncherTest(ctx, tconn, tabletMode, true /*stabilizeAppCount*/)
	if err != nil {
		s.Fatal("Failed to set up launcher test case: ", err)
	}
	defer cleanup(cleanupCtx)

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree")

	if err := launcher.CreateFolder(ctx, tconn); err != nil {
		s.Fatal("Failed to create folder app: ", err)
	}

	const maxFolderWidth int = 168

	enFolderName := "NewName"
	longFolderName := "Arbitrarily long folder name that fills the text field!!!"
	jpFolderName := "フォルダ"
	japaneseTypeAction := "foruda"

	// Chrome add prefix "Folder " to all folder names in AppListItemView.
	folderWithEnName := nodewith.Name("Folder " + enFolderName).ClassName(launcher.ExpandedItemsClass)
	folderWithJpName := nodewith.Name("Folder " + jpFolderName).ClassName(launcher.ExpandedItemsClass)

	folderView := nodewith.ClassName("AppListFolderView")
	textfield := nodewith.Role(role.TextField).Ancestor(folderView)

	ui := uiauto.New(tconn)
	if err := uiauto.Combine("Rename Folder to NewName",
		launcher.OpenExpandedView(tconn),
		ui.LeftClick(launcher.UnnamedFolderFinder.First()),
		ui.WaitUntilExists(folderView),
		ui.WaitUntilExists(nodewith.HasClass(launcher.ExpandedItemsClass).Ancestor(folderView).Nth(0)),
		ui.FocusAndWait(textfield),
		func(ctx context.Context) error {
			return kb.Type(ctx, enFolderName)
		},
	)(ctx); err != nil {
		s.Fatal("Failed to open the folder view and edit the folder name: ", err)
	}

	// During editing the folder name, the folder name text field should be focused.
	if err := ui.WaitUntilExists(textfield.Focused())(ctx); err != nil {
		s.Fatal("The folder text field is not focused while editing the folder name: ", err)
	}

	if err := uiauto.Combine("Finish editing the folder name",
		kb.AccelAction("Enter"),
		ui.Exists(folderView),
	)(ctx); err != nil {
		s.Fatal("Failed to finish editing the folder name with the folder view open: ", err)
	}

	if err := uiauto.Combine("Close the folder view",
		kb.AccelAction("esc"),
		ui.WaitUntilGone(folderView),
		ui.WaitUntilExists(folderWithEnName),
	)(ctx); err != nil {
		s.Fatal("Failed to close the folder view: ", err)
	}

	if err := uiauto.Combine("Rename the folder to a long name that will be truncated",
		ui.LeftClick(folderWithEnName),
		ui.WaitUntilExists(folderView),
		ui.WaitUntilExists(nodewith.HasClass(launcher.ExpandedItemsClass).Ancestor(folderView).Nth(0)),
		ui.FocusAndWait(textfield),
		func(ctx context.Context) error {
			return kb.Type(ctx, longFolderName)
		},
		kb.AccelAction("Enter"),
	)(ctx); err != nil {
		s.Fatal("Failed to open the folder view and edit the folder name: ", err)
	}

	textLocation, err := ui.Location(ctx, textfield)
	if err != nil {
		s.Fatal("Failed to get the location of the folder name text field: ", err)
	}

	// Verify that the folder name text field width is bounded.
	if textLocation.Width > maxFolderWidth {
		s.Fatal("Failed to properly truncate the long folder name with ellipsis")
	}

	currentImeID, err := ime.CurrentInputMethod(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get the current IME ID: ", err)
	}

	imePrefix, err := ime.Prefix(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get the ime prefix: ", err)
	}
	jpImeID := imePrefix + ime.JapaneseWithUSKeyboard.ID

	// Set up the Japanese keyboard
	if err := ime.AddAndSetInputMethod(ctx, tconn, jpImeID); err != nil {
		s.Fatal("Failed to switch to the Japanese IME: ", err)
	}
	if err := ime.WaitForInputMethodMatches(ctx, tconn, jpImeID, 30*time.Second); err != nil {
		s.Fatal("Failed to switch to the Japanese IME: ", err)
	}

	defer ime.RemoveInputMethod(cleanupCtx, tconn, jpImeID)
	defer ime.SetCurrentInputMethod(cleanupCtx, tconn, currentImeID)

	if err = uiauto.Combine("Rename the folder in Japanese",
		// The previous test actions left the name textfiled in focused, but inactive
		// state - press Enter to activate the folder name textfield.
		kb.AccelAction("Enter"),
		// Clear the current folder name.
		kb.AccelAction("Backspace"),
		kb.TypeAction(japaneseTypeAction),
		// Press tab to change from hiragana to katakana
		kb.AccelAction("tab"),
		// Press enter to end the word selection
		kb.AccelAction("Enter"),
		// Press enter again to leave the folder name editing
		kb.AccelAction("Enter"),
		kb.AccelAction("esc"),
		ui.WaitUntilGone(folderView),
		ui.WaitUntilExists(folderWithJpName),
	)(ctx); err != nil {
		s.Fatal("Failed to set Japanese folder name: ", err)
	}
}
