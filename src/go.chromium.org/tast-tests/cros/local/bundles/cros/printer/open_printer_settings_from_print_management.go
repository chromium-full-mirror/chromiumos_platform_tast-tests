// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package printer

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/printer/pre"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/printmanagementapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/printpreview"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/printing/usbprinter"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         OpenPrinterSettingsFromPrintManagement,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Test that Printer settings can be navigated to from Print Management with or without print jobs",
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
		SoftwareDeps: []string{"chrome", "cups", "virtual_usb_printer"},
		HardwareDeps: hwdep.D(pre.PrinterSkipUnstableModels),
		Params: []testing.Param{
			{
				Val:     browser.TypeAsh,
				Fixture: "virtualUsbPrinterModulesLoaded",
			},
			{
				Name:              "lacros",
				Val:               browser.TypeLacros,
				ExtraSoftwareDeps: []string{"lacros"},
				Fixture:           "virtualUsbPrinterModulesLoadedWithLacros",
			},
		},
	})
}

func OpenPrinterSettingsFromPrintManagement(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	// Open chrome with print-management-setup-assistance enabled.
	cr, err := chrome.New(ctx, chrome.EnableFeatures("PrintManagementSetupAssistance"))
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	// Close test instance of Chrome.
	defer cr.Close(cleanupCtx)

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

	// Add printer.
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

	// Launch Print Management app.
	s.Log("Open Print Management SWA")
	printManagementApp, err := printmanagementapp.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch Print Management app: ", err)
	}

	// Click "manage printers" button.
	s.Log("Opening Printer settings from empty state")
	settingsApp := ossettings.New(tconn)
	defer settingsApp.Close(cleanupCtx)
	if err := uiauto.Combine("Launch Printer settings from empty state",
		printManagementApp.Focus(),
		printManagementApp.LaunchPrinterSettings(),
	)(ctx); err != nil {
		s.Fatal("Failed to open Printer settings from empty state UI: ", err)
	}

	// Close Printer settings to ensure the second launch is triggered by the
	// app button.
	s.Log("Close settings")
	if err := settingsApp.Close(ctx); err != nil {
		s.Fatal("Failed to close Printer settings: ", err)
	}

	// Create a browser (either ash or lacros, based on browser type).
	s.Log("Open browser and attempt to print")
	br, closeBrowser, err := browserfixt.SetUp(ctx, cr, s.Param().(browser.Type))
	if err != nil {
		s.Fatal("Failed to launch browser: ", err)
	}
	defer closeBrowser(cleanupCtx)

	// Open browser to printable page.
	conn, err := br.NewConn(ctx, chrome.VersionURL)
	if err != nil {
		s.Fatal("Failed to connect to browser: ", err)
	}
	defer conn.Close()

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get the keyboard: ", err)
	}
	defer kb.Close(cleanupCtx)

	// Launch print preview.
	if err := uiauto.Combine("open Print Preview with shortcut Ctrl+P",
		kb.AccelAction("Ctrl+P"),
		printpreview.WaitForPrintPreview(tconn),
	)(ctx); err != nil {
		s.Fatal("Failed to open Print Preview: ", err)
	}

	// Select virtual USB printer.
	s.Log("Selecting printer")
	const printerName = "DavieV Virtual USB Printer (USB)"
	if err := printpreview.SelectPrinter(ctx, tconn, printerName); err != nil {
		s.Fatal("Failed to select printer: ", err)
	}
	if err := printpreview.WaitForPrintPreview(tconn)(ctx); err != nil {
		s.Fatal("Failed to wait for Print Preview: ", err)
	}

	// Print page.
	if err = printpreview.Print(ctx, tconn); err != nil {
		s.Fatal("Failed to print: ", err)
	}

	s.Log("Waiting for print job to complete")
	if err = testing.Poll(ctx, func(ctx context.Context) error {
		out, err := testexec.CommandContext(ctx, "lpstat", "-W", "completed", "-o").Output(testexec.DumpLogOnError)
		if err != nil {
			return err
		}
		if len(out) == 0 {
			return errors.New("Print job has not completed yet")
		}
		testing.ContextLog(ctx, "Print job has completed")
		return nil
	}, nil); err != nil {
		s.Fatal("Print job failed to complete: ", err)
	}

	// Verify Printer settings opened from Print Management app.
	s.Log("Opening Printer settings from non-empty state")
	if err := uiauto.Combine("Opening printer settings from non-empty state",
		printManagementApp.Focus(),
		printManagementApp.VerifyHistoryLabel(),
		printManagementApp.VerifyPrintJob(),
		printManagementApp.LaunchPrinterSettings(),
	)(ctx); err != nil {
		s.Fatal("Failed to launch Printer settings: ", err)
	}
}
