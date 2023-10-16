// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package printer

import (
	"context"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/printer/pre"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/printpreview"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         OpenPrinterSettingsFromPrintPreviewDestinationDialogCros,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Test that Printer settings can be navigated to from Print Preview destination-dialog-cros",
		// ChromeOS > Software > System Services > Peripherals > Printing
		BugComponent: "b:1131981",
		Contacts: []string{
			"cros-peripherals@google.com",
			"project-bolton@google.com",
			"gavinwill@google.com",
			"ashleydp@google.com",
		},
		Attr: []string{
			"group:mainline",
			"informational",
			"group:paper-io",
			"paper-io_printing",
			"group:cq-medium",
		},
		Timeout:      2 * time.Minute,
		SoftwareDeps: []string{"chrome", "cups"},
		HardwareDeps: hwdep.D(pre.PrinterSkipUnstableModels),
		Params: []testing.Param{
			{
				Val:     browser.TypeAsh,
				Fixture: "chromeLoggedInWithPrinterSetupAssistance",
			},
			{
				Name:              "lacros",
				Val:               browser.TypeLacros,
				Fixture:           "lacrosPrinterSetupAssistanceEnabled",
				ExtraSoftwareDeps: []string{"lacros"},
			},
		},
	})
}

func OpenPrinterSettingsFromPrintPreviewDestinationDialogCros(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	// Get chrome with print-preview-setup-assistance flags enabled from fixture.
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	// tconn is the ash TestConn.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect ash test API: ", err)
	}
	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	// Hide all notifications to prevent them from covering the printer entry.
	if err := ash.CloseNotifications(ctx, tconn); err != nil {
		s.Fatal("Failed to close all notifications: ", err)
	}

	// Create a browser (either ash or lacros, based on browser type).
	br, closeBrowser, err := browserfixt.SetUp(ctx, cr, s.Param().(browser.Type))
	if err != nil {
		s.Fatal("Failed to launch browser: ", err)
	}
	defer closeBrowser(cleanupCtx)

	conn, err := br.NewConn(ctx, chrome.VersionURL)
	if err != nil {
		s.Fatal("Failed to connect to browser: ", err)
	}
	defer conn.Close()

	// Open print preview.
	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed get keyboard: ", err)
	}
	if err := uiauto.Combine("open Print Preview with shortcut Ctrl+P",
		kb.AccelAction("Ctrl+P"),
		printpreview.WaitForPrintPreview(tconn),
	)(ctx); err != nil {
		s.Fatal("Failed to open Print Preview: ", err)
	}

	// Open dialog.
	s.Log("Clicking 'See more..' to open dialog")
	if err := printpreview.OpenDestinationDialogCros(ctx, tconn); err != nil {
		s.Fatal("Failed to select printer: ", err)
	}

	// Click "manage printers" button.
	s.Log("Opening Printer settings")
	settingsApp := ossettings.New(tconn)
	defer settingsApp.Close(cleanupCtx)
	ui := uiauto.New(tconn).WithTimeout(10 * time.Second)

	// Button to launch Printer settings will state "Manage" when there are printers
	// in the list and "Manager Printers" in the empty state UI.
	buttonNameRegex := regexp.MustCompile("Manage( Printers)?")
	launchButton := nodewith.NameRegex(buttonNameRegex).Role(role.Button)
	printerSettingsWindow := nodewith.Name("Settings - Printers").Role(role.Window).First()
	if err := uiauto.Combine("find and click manage printers button",
		ui.WaitUntilExists(launchButton),
		ui.DoDefault(launchButton),
		ui.WithTimeout(30*time.Second).WaitUntilExists(printerSettingsWindow),
	)(ctx); err != nil {
		s.Fatal("Failed to open settings: ", err)
	}
}
