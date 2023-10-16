// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package printer

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/printer/pre"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/printer/uitools"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/printing/usbprinter"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         OsPrinterSettings,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests that a virtual USB printer can be added, edited, and removed from the OS Settings Printer page",
		Contacts:     []string{"cros-peripherals@google.com", "project-bolton@google.com", "gavinwill@google.com"},
		Attr: []string{
			"group:mainline",
			"informational",
			"group:paper-io",
			"paper-io_printing",
			"group:cq-medium",
		},
		// ChromeOS > Software > System Services > Peripherals > Printing
		BugComponent: "b:1131981",
		Timeout:      2 * time.Minute,
		SoftwareDeps: []string{"chrome", "cups"},
		HardwareDeps: hwdep.D(pre.PrinterSkipUnstableModels),
		Fixture:      "virtualUsbPrinterModulesLoaded",
	})
}

func OsPrinterSettings(ctx context.Context, s *testing.State) {
	uiTimeout := 3 * time.Second

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	cr, err := chrome.New(ctx, chrome.EnableFeatures("PrinterSettingsRevamp"))
	if err != nil {
		s.Fatal("Cannot start Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	// tconn is the ash TestConn.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect ash test API: ", err)
	}
	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	s.Log("Installing printer")

	printer, err := usbprinter.Start(ctx,
		usbprinter.WithIPPUSBDescriptors(),
		usbprinter.WithGenericIPPAttributes(),
		usbprinter.WaitUntilConfigured())
	if err != nil {
		s.Fatal("Failed to start IPP-over-USB printer: ", err)
	}
	defer func(ctx context.Context) {
		if err := printer.Stop(ctx); err != nil {
			s.Error("Failed to stop printer: ", err)
		}
	}(cleanupCtx)

	// Open OS Settings and navigate to the Printing page.
	entryFinder := uitools.PrintersFinder.Ancestor(ossettings.WindowFinder)
	ui := uiauto.New(tconn)
	if err := uitools.NavigateToPrintersSettingsPage(ctx, tconn, cr, ui); err != nil {
		s.Fatal("Failed to launch Settings page: ", err)
	}

	// Hide all notifications to prevent them from covering the printer entry.
	if err := ash.CloseNotifications(ctx, tconn); err != nil {
		s.Fatal("Failed to close all notifications: ", err)
	}

	savePrinterButton := nodewith.HasClass("save-printer-button").Name("Save").Ancestor(ossettings.WindowFinder)
	moreActionsButton := nodewith.HasClass("icon-more-vert").Name("More actions").Ancestor(ossettings.WindowFinder)

	if err := uiauto.Combine("click Settings Printer entry, save printer",
		ui.LeftClick(entryFinder),
		ui.LeftClick(savePrinterButton),
		ui.WithTimeout(uiTimeout).WaitUntilExists(moreActionsButton),
	)(ctx); err != nil {
		s.Fatal("Failed to save virtual USB printer: ", err)
	}

	if err := uiauto.Combine("click more actions button and edit printer",
		ui.LeftClick(moreActionsButton),
		ui.LeftClick(nodewith.HasClass("dropdown-item").Name("Edit")),
	)(ctx); err != nil {
		s.Fatal("Failed to edit saved printer entry: ", err)
	}

	// Set up keyboard.
	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to find keyboard: ", err)
	}
	defer kb.Close(ctx)

	// Focus the printer name text field box.
	printerNameTextField := nodewith.Name("Name").Role(role.TextField).Ancestor(ossettings.WindowFinder)
	if err := ui.DoDefault(printerNameTextField)(ctx); err != nil {
		s.Fatal("Failed to click printer name text field: ", err)
	}

	// Rename the virtual printer and save it.
	const firstPrinterName = "First New Printer"
	if err := uiauto.Combine("Rename printer then save",
		ui.WithTimeout(uiTimeout).WaitUntilExists(printerNameTextField.Focused()),
		kb.AccelAction("Ctrl+A"),
		kb.TypeAction(firstPrinterName),
		ui.DoDefault(nodewith.Name("Save").Role(role.Button)),
	)(ctx); err != nil {
		s.Fatal("Failed to rename printer: ", err)
	}

	// Refresh the OS Settings window so the Available Printer section starts
	// collapsed, then open that section.
	addPrinterButton := nodewith.Name("Add printer manually").Role(role.Button)
	if err := uiauto.Combine("refresh settings",
		kb.AccelAction("Ctrl+R"),
		ui.DoDefault(nodewith.Name("Other available printers").Role(role.Button)),
		ui.WithTimeout(uiTimeout).WaitUntilExists(addPrinterButton),
	)(ctx); err != nil {
		s.Fatal("Failed refreshing settings: ", err)
	}

	// Open the add printer dialog and add a new printer.
	const secondPrinterName = "Second New Printer"
	if err := uiauto.Combine("add printer manually",
		ui.DoDefault(addPrinterButton),
		kb.TypeAction(secondPrinterName),
		kb.AccelAction("Tab"),
		kb.TypeAction("Address"),
		ui.LeftClick(nodewith.Name("Protocol").Role(role.ComboBoxSelect)),
		ui.LeftClick(nodewith.Name("AppSocket (TCP/IP)").Role(role.ListBoxOption)),
		ui.DoDefault(nodewith.Name("Add").Role(role.Button)),
		ui.LeftClick(nodewith.Name("Manufacturer").Role(role.TextField)),
		ui.DoDefault(nodewith.Name("Anitech").Role(role.Button)),
		ui.LeftClick(nodewith.Name("Model").Role(role.TextField)),
		ui.DoDefault(nodewith.Name("Anitech M24").Role(role.Button)),
		ui.DoDefault(nodewith.Name("Add").Role(role.Button)),
		ui.WithTimeout(uiTimeout).WaitUntilExists(nodewith.NameContaining(secondPrinterName)),
	)(ctx); err != nil {
		s.Fatal("Failed to add printer manually: ", err)
	}

	// Remove both saved printers for to clean up for subsequent tests.
	if err := uiauto.Combine("click more actions button and remove both printers",
		ui.DoDefault(moreActionsButton.First()),
		ui.LeftClick(nodewith.HasClass("dropdown-item").Name("Remove")),
		ui.DoDefault(moreActionsButton),
		ui.LeftClick(nodewith.HasClass("dropdown-item").Name("Remove")),
	)(ctx); err != nil {
		s.Fatal("Failed to edit saved printer entry: ", err)
	}
}
