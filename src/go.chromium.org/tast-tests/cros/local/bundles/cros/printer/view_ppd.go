// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package printer

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/printer/ppdindex"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/printer/uitools"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/printing/printer"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ViewPPD,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Tests that a user can view the PPD for an installed printer",
		Contacts:     []string{"cros-peripherals@google.com", "project-bolton@google.com", "nmuggli@google.com"},
		// ChromeOS > Platform > Services > Printing
		BugComponent: "b:167231",
		Attr: []string{
			"group:mainline",
			"informational",
			"group:paper-io",
			"paper-io_printing",
			"group:hw_agnostic",
		},
		Timeout:      2 * time.Minute,
		SoftwareDeps: []string{"chrome", "cros_internal", "cups"},
		Params: []testing.Param{
			{
				Val: browser.TypeAsh,
			},
			{
				Name:              "lacros",
				ExtraSoftwareDeps: []string{"lacros"},
				Val:               browser.TypeLacros,
			},
		},
	})
}

// createPrinter creates a new printer using appsocket protocol so we don't have
// to have an actual printer available.  Printer name, manufacturer, and model
// are all provided by the caller.
func createPrinter(ctx context.Context, s *testing.State, cr *chrome.Chrome, tconn *chrome.TestConn, ui *uiauto.Context, printerName, printerManufacturer, printerModel string) {
	s.Log("Creating printer: ", printerName, ", ", printerManufacturer, ", ", printerModel)

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get the keyboard: ", err)
	}
	defer kb.Close(ctx)

	// Open OS Settings and navigate to the Printing page.
	entryFinder := uitools.PrintersFinder.Ancestor(ossettings.WindowFinder)
	if _, err := ossettings.LaunchAtPageURL(ctx, tconn, cr, uitools.SettingsPageName, ui.Exists(entryFinder)); err != nil {
		s.Fatal("Failed to launch Settings page: ", err)
	}

	// Open add settings dialog
	addPrinterButton := uitools.AddPrinterFinder.Ancestor(ossettings.WindowFinder)
	if err := uiauto.Combine("click add printer button",
		ui.WithTimeout(10*time.Second).WaitUntilExists(entryFinder),
		ui.DoDefault(entryFinder),
		ui.WithTimeout(10*time.Second).WaitUntilExists(addPrinterButton),
	)(ctx); err != nil {
		s.Fatal("Failed to open settings dialog: ", err)
	}

	// Click add printer button
	if err := uiauto.Combine("click add printer button",
		ui.WithTimeout(10*time.Second).WaitUntilEnabled(addPrinterButton),
		ui.DoDefault(addPrinterButton),
	)(ctx); err != nil {
		s.Fatal("Failed to add printer - network offline: ", err)
	}

	// Input basic parameters
	editDialog := uitools.AddPrinterManuallyFinder.Ancestor(ossettings.WindowFinder)
	nameField := uitools.NameFinder.Ancestor(editDialog)
	addressField := uitools.AddressFinder.Ancestor(editDialog)
	protocolField := uitools.ProtocolFinder.Ancestor(editDialog)
	appSocketItem := uitools.AppSocketFinder.Ancestor(editDialog)
	addButton := uitools.AddFinder.Ancestor(editDialog)
	if err := uiauto.Combine("input basic printer parameters",
		ui.WithTimeout(10*time.Second).WaitUntilExists(nameField),
		ui.EnsureFocused(nameField),
		kb.TypeAction(printerName),
		ui.EnsureFocused(addressField),
		kb.TypeAction("localhost"),
		ui.DoDefault(protocolField),
		ui.DoDefault(appSocketItem),
		ui.DoDefault(addButton),
	)(ctx); err != nil {
		s.Fatal("Failed to input basic printer parameters: ", err)
	}

	// Input advanced parameters
	advancedConfigDialog := uitools.AdvancedConfigFinder.Ancestor(ossettings.WindowFinder)
	manufacturerButton := uitools.ManufacturerFinder.Ancestor(advancedConfigDialog)
	manufacturerSelection := nodewith.Role(role.Button).Name(printerManufacturer).Ancestor(advancedConfigDialog)
	modelButton := uitools.ModelFinder.Ancestor(advancedConfigDialog)
	modelSelection := nodewith.Role(role.Button).Name(printerModel).Ancestor(advancedConfigDialog)
	advancedConfigAddButton := uitools.AddFinder.Ancestor(advancedConfigDialog)
	printerEntry := nodewith.HasClass("list-item").NameContaining(printerName).Ancestor(ossettings.WindowFinder)
	if err := uiauto.Combine("input advanced printer parameters",
		ui.WithTimeout(10*time.Second).WaitUntilExists(manufacturerButton),
		ui.EnsureFocused(manufacturerButton),
		kb.TypeAction(printerManufacturer),
		ui.WithTimeout(10*time.Second).WaitUntilExists(manufacturerSelection),
		ui.DoDefault(manufacturerSelection),
		ui.WithTimeout(10*time.Second).WaitUntilGone(manufacturerSelection),
		ui.EnsureFocused(modelButton),
		kb.TypeAction(printerModel),
		ui.WithTimeout(10*time.Second).WaitUntilExists(modelSelection),
		ui.DoDefault(modelSelection),
		ui.WithTimeout(10*time.Second).WaitUntilGone(modelSelection),
		ui.DoDefault(advancedConfigAddButton),
		ui.WithTimeout(time.Minute).WaitUntilExists(printerEntry),
	)(ctx); err != nil {
		s.Fatal("Failed to input advanced printer parameters: ", err)
	}
}

// checkPpd chooses the edit button for the printer specified by the caller and
// clicks on the View PPD button.  It then checks that the appropriate web page
// opens. If eula is non-empty, this will additionally check to make sure the
// correct EULA text is present.
func checkPpd(ctx context.Context, s *testing.State, ui *uiauto.Context,
	printerName, eula string, cr *chrome.Chrome) {
	// Edit the printer and select the View PPD button.
	printerEntry := nodewith.HasClass("list-item").NameContaining(printerName).Ancestor(ossettings.WindowFinder)
	printerButton := nodewith.Role(role.Button).Name("More actions").Ancestor(printerEntry)
	editText := uitools.EditFinder.Ancestor(ossettings.WindowFinder)
	editPrinterDialog := uitools.EditPrinterFinder.Ancestor(ossettings.WindowFinder)
	viewPpdButton := uitools.ViewPpdFinder.Ancestor(editPrinterDialog)
	if err := uiauto.Combine("click Edit Printer, view PPD button",
		ui.WithTimeout(10*time.Second).WaitUntilExists(printerButton),
		ui.DoDefault(printerButton),
		ui.DoDefault(editText),
		ui.WithTimeout(10*time.Second).WaitUntilExists(viewPpdButton),
		ui.DoDefault(viewPpdButton),
	)(ctx); err != nil {
		s.Fatal("Failed to view PPD: ", err)
	}

	// Make sure the tab with the PPD results gets displayed and that it does not
	// contain the error message.
	ppdWindow := uitools.GetPpdWindowFinder(printerName)
	// All PPDs will begin with this line.  Make sure this shows up.
	ppdText := uitools.PpdStartTextFinder.Ancestor(ppdWindow)
	// This error message should not show up.
	errorMsg := uitools.PpdRetrieveErrorFinder.Ancestor(ppdWindow)
	if err := uiauto.Combine("wait for PPD results",
		ui.WithTimeout(time.Minute).WaitUntilExists(ppdWindow),
		ui.WithTimeout(time.Minute).WaitUntilExists(ppdText),
		ui.Gone(errorMsg),
	)(ctx); err != nil {
		s.Fatal("Failed to display PPD results: ", err)
	}
	if len(eula) > 0 {
		eulaText := uitools.GetEulaFinder(eula).Ancestor(ppdWindow)
		if err := ui.WithTimeout(time.Minute).WaitUntilExists(eulaText)(ctx); err != nil {
			s.Fatal("Failed to find license text: ", err)
		}
	}
}

func ViewPPD(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	server := ppdindex.New(s)
	defer server.Stop()

	// Populate our PPD Index with data.  We'll use this same data later to create
	// our printers.  We don't care what the PPD contents are so just use the
	// default.
	manufacturer1 := "Brother"
	model1 := "Brother DCP-1200"
	manufacturer2 := "Xerox"
	model2 := "Xerox B320"
	license2 := "xerox-printing-license"
	server.AddEntry(ppdindex.Entry{Manufacturer: manufacturer1, Model: model1})
	server.AddEntry(ppdindex.Entry{Manufacturer: manufacturer2, Model: model2, License: license2})

	bt := s.Param().(browser.Type)
	cr, err := browserfixt.NewChrome(ctx, bt, lacrosfixt.NewConfig(), chrome.ExtraArgs("--printing-ppd-channel=localhost"))
	if err != nil {
		s.Fatal("Failed to create chrome instance: ", err)
	}
	defer cr.Close(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}
	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	if err := printer.ResetCups(ctx); err != nil {
		s.Fatal("Failed to reset cupsd: ", err)
	}

	ui := uiauto.New(tconn)

	// Test a printer that does not have the EULA
	printerName := "test-printer"
	eula := ""
	createPrinter(ctx, s, cr, tconn, ui, printerName, manufacturer1, model1)
	checkPpd(ctx, s, ui, printerName, eula, cr)

	// Test with a printer that has an EULA
	printerName = "test-printer-with-eula"
	eula = "chrome://os-credits/#" + license2
	createPrinter(ctx, s, cr, tconn, ui, printerName, manufacturer2, model2)
	checkPpd(ctx, s, ui, printerName, eula, cr)
}
