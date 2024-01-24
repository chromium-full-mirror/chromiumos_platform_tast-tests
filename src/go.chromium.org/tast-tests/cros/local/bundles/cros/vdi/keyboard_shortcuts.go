// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package vdi

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/uidetection"
	"go.chromium.org/tast-tests/cros/local/vdi/fixtures"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type keyboardShortcutsParams struct {
	DesktopName   string
	RunDialogKeys string
	StartMenuText []string
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         KeyboardShortcuts,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Test checks that certain keystrokes used for e.g. copy-paste and printing are carried into VDI sessions",
		Contacts: []string{
			"pwa-commercial@google.com",
			"kamilszarek@google.com", // VDI testing infrastructure owner
			"giovax@google.com",      // Test owner
		},
		BugComponent: "b:1198148", // ChromeOS > Software > Commercial (Enterprise) > App Platforms > Virtualization
		Attr:         []string{},
		SoftwareDeps: []string{"chrome"},
		Timeout:      5 * time.Minute,
		SearchFlags: []*testing.StringPair{{
			Key: "feature_id",
			// Use Keyboard shortcuts to speed up clipboard and printing access in VDI: COM_VDI_CUJ7_TASK4_WF1.
			Value: "screenplay-608e694b-2cff-4d77-a96b-2c5afb1e9faf",
		}},
		Requirements: []string{"screenplay-608e694b-2cff-4d77-a96b-2c5afb1e9faf"},
		Params: []testing.Param{
			{
				Name:      "citrix",
				Fixture:   fixture.CitrixLaunched,
				ExtraAttr: []string{"group:vdi_limited"},
				Val: keyboardShortcutsParams{
					DesktopName:   "WindowsServer2019",
					RunDialogKeys: "Search+R",
					StartMenuText: []string{"Citrix", "Workspace"},
				},
			},
			{
				Name:              "lacros_citrix",
				ExtraAttr:         []string{"group:vdi_limited"},
				ExtraSoftwareDeps: []string{"lacros"},
				Fixture:           fixture.LacrosCitrixLaunched,
				Val: keyboardShortcutsParams{
					DesktopName:   "WindowsServer2019",
					RunDialogKeys: "Search+R",
					StartMenuText: []string{"Citrix", "Workspace"},
				},
			},
			{
				Name:    "vmware",
				Fixture: fixture.VmwareLaunched,
				Val: keyboardShortcutsParams{
					DesktopName:   "TD-RDS-DESKTOPS",
					RunDialogKeys: "Ctrl+Search+R",
					StartMenuText: []string{"Most", "used"},
				},
			},
			// TODO(b/269235077) add vmware lacros variant when fixture is available
			{
				Name:      "mgs_citrix",
				Fixture:   fixture.MgsCitrixLaunched,
				ExtraAttr: []string{"group:vdi_limited"},
				Val: keyboardShortcutsParams{
					DesktopName:   "WindowsServer2019",
					RunDialogKeys: "Search+R",
					StartMenuText: []string{"Citrix", "Workspace"},
				},
			},
			{
				Name:              "mgs_lacros_citrix",
				Fixture:           fixture.MgsLacrosCitrixLaunched,
				ExtraAttr:         []string{"group:vdi_limited"},
				ExtraSoftwareDeps: []string{"lacros"},
				Val: keyboardShortcutsParams{
					DesktopName:   "WindowsServer2019",
					RunDialogKeys: "Search+R",
					StartMenuText: []string{"Citrix", "Workspace"},
				},
			},
			{
				Name:    "mgs_vmware",
				Fixture: fixture.MgsVmwareLaunched,
				Val: keyboardShortcutsParams{
					DesktopName:   "TD-RDS-DESKTOPS",
					RunDialogKeys: "Ctrl+Search+R",
					StartMenuText: []string{"Most", "used"},
				},
			},
			// TODO(b/269235077) add vmware lacros MGS  variant when fixture is available
		},
	})
}

func KeyboardShortcuts(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	vdi := s.FixtValue().(fixtures.HasVDIConnector).VDIConnector()
	uidetector := s.FixtValue().(fixtures.HasUIDetector).UIDetector()

	defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree")

	// Connect to Test API to use it with the UI library.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	ui := uiauto.New(tconn)

	recycleBin := uidetection.TextBlock([]string{"Recycle", "Bin"})
	isOpened := func(ctx context.Context) error {
		// Finding the Recycle Bin seems to be less flaky than finding toolbar_buttons_icon.png.
		if err := uidetector.WithTimeout(60 * time.Second).WaitUntilExists(recycleBin)(ctx); err != nil {
			return errors.Wrap(err, "failed waiting for the recycle bin to appear")
		}
		return nil
	}

	// Open desktop.
	desktopToOpen := s.Param().(keyboardShortcutsParams).DesktopName
	if err := vdi.SearchAndOpenApplication(ctx, desktopToOpen, isOpened)(ctx); err != nil {
		s.Fatalf("Failed to open %v app: %v", desktopToOpen, err)
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get a keyboard: ", err)
	}
	defer kb.Close(ctx)

	// Move focus on Windows desktop.
	if err := uidetector.LeftClick(recycleBin)(ctx); err != nil {
		s.Fatal("Failed to click the Recycle Bin: ", err)
	}

	// Open the Run dialog box.
	if err := ui.RetryUntil(
		kb.AccelAction(s.Param().(keyboardShortcutsParams).RunDialogKeys),
		uidetector.WithTimeout(30*time.Second).WaitUntilExists(uidetection.Word("Run").First()),
	)(ctx); err != nil {
		s.Error("Failed to invoke opening the run dialog box: ", err)
	}

	// Open Notepad.
	notepadApp := uidetection.TextBlock([]string{"Untitled", "Notepad"}).First()
	if err := ui.RetryUntil(
		uiauto.Combine("open notepad",
			uidetector.LeftClick(uidetection.Word("Run").First()),
			kb.TypeAction("notepad"),
			kb.AccelAction("Enter"),
		),
		uidetector.WithTimeout(30*time.Second).WaitUntilExists(notepadApp),
	)(ctx); err != nil {
		s.Error("Failed to open the Notepad: ", err)
	}

	// Use copy & paste shortcuts inside the Notepad.
	if err := ui.RetryUntil(
		uiauto.Combine("try copy & paste shortcuts inside the Notepad",
			uidetector.LeftClick(notepadApp),
			kb.TypeAction("hello "),
			kb.AccelAction("Ctrl+A"),
			kb.AccelAction("Ctrl+C"),
			kb.AccelAction("Right"),
			kb.AccelAction("Ctrl+V"),
		),
		uiauto.Combine("try to find the pasted thing and clean the Notepad",
			uidetector.WithTimeout(20*time.Second).WaitUntilExists(uidetection.TextBlock([]string{"hello", "hello"}).First()),
		),
	)(ctx); err != nil {
		s.Error("Failed to use copy and paste in Notepad: ", err)
	}

	// Open printing menu.
	if err := ui.RetryUntil(
		kb.AccelAction("Ctrl+P"),
		uidetector.WithTimeout(10*time.Second).WaitUntilExists(uidetection.Word("Print").First()),
	)(ctx); err != nil {
		s.Error("Failed to open the printing menu: ", err)
	}

	// Close printing menu.
	if err := kb.AccelAction("ESC")(ctx); err != nil {
		s.Fatal("Failed to close the printing menu: ", err)
	}

	// TODO(b/267734698): Remove this block when closing windows is a part of the cleanup process.
	// Close Notepad.
	closeBtn := uidetection.Word("Close").First()
	doYouWantToSaveDialog := uidetection.TextBlock([]string{"Do", "you", "want", "to", "save", "changes"})
	if err := ui.RetryUntil(
		uiauto.Combine("close notepad",
			uidetector.RightClick(notepadApp),
			uidetector.WithTimeout(10*time.Second).WaitUntilExists(closeBtn),
			uidetector.LeftClick(closeBtn),
			uidetector.WithTimeout(10*time.Second).WaitUntilExists(doYouWantToSaveDialog),
			kb.AccelAction("Tab"), // Move to "Don't Save" button
			kb.AccelAction("Enter"),
		),
		uidetector.WithTimeout(20*time.Second).Gone(notepadApp),
	)(ctx); err != nil {
		// This would be nice to succeed but is not necessarily a test failure...
		fmt.Println(err)
	}

	// Open Windows Start Menu.
	if err := ui.RetryUntil(
		kb.AccelAction("Ctrl+ESC"),
		uidetector.WithTimeout(20*time.Second).WaitUntilExists(uidetection.TextBlock(s.Param().(keyboardShortcutsParams).StartMenuText)),
	)(ctx); err != nil {
		s.Error("Failed to open the Windows Start Menu: ", err)
	}

	// Close Windows Start menu.
	if err := kb.AccelAction("ESC")(ctx); err != nil {
		s.Fatal("Failed to close the Windows Start menu: ", err)
	}
}
