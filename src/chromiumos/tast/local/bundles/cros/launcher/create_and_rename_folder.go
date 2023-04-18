// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package launcher

import (
	"context"
	"time"

	"chromiumos/tast/ctxutil"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/ime"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/chrome/uiauto/launcher"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/input"
	"chromiumos/tast/testing"
	"chromiumos/tast/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CreateAndRenameFolder,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Renaming Folder In Launcher",
		Contacts: []string{
			"cros-system-ui-eng@google.com",
			"seewaifu@chromium.org",
			"tbarzic@chromium.org",
			"chromeos-sw-engprod@google.com",
		},
		BugComponent: "b:1288350",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
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
	textfield := nodewith.ClassName("Textfield").Ancestor(folderView)

	ui := uiauto.New(tconn)
	if err := uiauto.Combine("Rename Folder to NewName",
		launcher.OpenExpandedView(tconn),
		ui.LeftClick(launcher.UnnamedFolderFinder.First()),
		ui.WaitUntilExists(folderView),
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
		ui.FocusAndWait(textfield),
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
