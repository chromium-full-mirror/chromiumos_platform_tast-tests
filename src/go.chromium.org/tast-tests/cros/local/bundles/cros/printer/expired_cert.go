// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package printer

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/printer/pre"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/printer/uitools"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/printmanagementapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/printpreview"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/printing/ippeveprinter"
	"go.chromium.org/tast-tests/cros/local/printing/printer"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ExpiredCert,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Tests that a printer with an expired SSL certificate fails print jobs and propagates the error to chrome",
		Contacts:     []string{"project-bolton@google.com"},
		Attr: []string{
			"group:paper-io",
			"paper-io_printing",
			"group:mainline",
			"informational",
		},
		// ChromeOS > Platform > Services > Printing
		BugComponent: "b:167231",
		SoftwareDeps: []string{"chrome", "cups"},
		HardwareDeps: hwdep.D(pre.PrinterSkipUnstableModels),
		Data:         []string{"localhost.key", "localhost.crt"},
		Params: []testing.Param{
			{
				Val:     browser.TypeAsh,
				Fixture: "chromeLoggedIn",
			},
			{
				Name:              "lacros",
				Val:               browser.TypeLacros,
				ExtraSoftwareDeps: []string{"lacros"},
				Fixture:           "lacros",
			},
		},
	})
}

// ExpiredCert creates an IPP everywhere printer with an expired ssl
// certificate and attempts to print to it. The print job should fail and
// there should be a notification surfaced telling the user about the expired
// certificate.
func ExpiredCert(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	bt := s.Param().(browser.Type)
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	tmpDir, err := ioutil.TempDir("", "expired-certs")
	if err != nil {
		s.Fatal("Failed to create temporary directory: ", err)
	}
	defer os.RemoveAll(tmpDir)

	// Copy files from data to tmpDir to use with ippeveprinter.
	fsutil.CopyFile(s.DataPath("localhost.crt"), filepath.Join(tmpDir, "localhost.crt"))
	fsutil.CopyFile(s.DataPath("localhost.key"), filepath.Join(tmpDir, "localhost.key"))

	s.Log("Installing printer")
	if err := printer.ResetCups(ctx, /*usePrintscanmgr=*/false); err != nil {
		s.Fatal("Failed to reset cupsd: ", err)
	}

	const printerDisplayName = "expiredCert"

	// Start virtual printer with expired certificate.
	printer, err := ippeveprinter.Start(ctx,
		ippeveprinter.WithAuthenticationEnabled(),
		ippeveprinter.WithKeyPath(tmpDir),
		ippeveprinter.WithHostname("localhost"))
	if err != nil {
		s.Fatal("Failed to start ippeveprinter: ", err)
	}
	defer func(ctx context.Context) {
		if err := printer.Stop(ctx); err != nil {
			s.Fatal("Failed to stop fake printer: ", err)
		}
	}(cleanupCtx)

	// Open OS Settings and navigate to the Printing page.
	ui := uiauto.New(tconn)
	if err := uitools.NavigateToPrintersSettingsPage(ctx, tconn, cr, ui); err != nil {
		s.Fatal("Failed to launch Settings page: ", err)
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get the keyboard: ", err)
	}
	defer kb.Close(ctx)

	// Hide all notifications to prevent them from covering the printer entry.
	if err := ash.CloseNotifications(ctx, tconn); err != nil {
		s.Fatal("Failed to close all notifications: ", err)
	}

	// Open the Add Printers dialog.
	if err := uitools.OpenAddPrinterDialog(ctx, ui); err != nil {
		s.Fatal("Failed to open the Add Printers dialog: ", err)
	}

	nameFinder := nodewith.Role("textField").Name("Name")
	addressFinder := nodewith.Role("textField").Name("Address")

	// Select IPPS for the protocol.
	if err = printpreview.SetDropdown(ctx, tconn, "Protocol", "Internet Printing Protocol (IPPS)"); err != nil {
		s.Fatal("Failed to select IPPS from dropdown menu: ", err)
	}
	if err := uiauto.Combine("set printer details",
		ui.DoDefault(nameFinder),
		ui.EnsureFocused(nameFinder),
		kb.TypeAction(printerDisplayName),
		ui.DoDefault(addressFinder),
		ui.EnsureFocused(addressFinder),
		kb.TypeAction(fmt.Sprintf("localhost:%d", printer.Port())),
		ui.DoDefault(nodewith.Role("button").Name("Add")),
	)(ctx); err != nil {
		s.Fatal("Failed to set printer details: ", err)
	}

	printManager, err := printmanagementapp.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to open Print Management app: ", err)
	}
	defer func(ctx context.Context) {
		if err := printManager.ClearHistory()(ctx); err != nil {
			s.Error("Failed to clear final printing history: ", err)
		}
	}(ctx)

	if err := printManager.ClearHistory()(ctx); err != nil {
		s.Fatal("Failed to clear initial printing history: ", err)
	}

	// Create a browser (either ash or lacros, based on browser type).
	br, closeBrowser, err := browserfixt.SetUp(ctx, cr, bt)
	if err != nil {
		s.Fatal("Failed to launch browser: ", err)
	}
	defer closeBrowser(cleanupCtx)
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree")

	conn, err := br.NewConn(ctx, chrome.VersionURL)
	if err != nil {
		s.Fatal("Failed to connect to browser: ", err)
	}
	defer conn.Close()

	if err := uiauto.Combine("open Print Preview with shortcut Ctrl+P",
		kb.AccelAction("Ctrl+P"),
		printpreview.WaitForPrintPreview(tconn),
	)(ctx); err != nil {
		s.Fatal("Failed to open Print Preview: ", err)
	}

	// Select printer and click Print button.
	s.Log("Selecting printer")
	if err := printpreview.SelectPrinter(ctx, tconn, printerDisplayName); err != nil {
		s.Fatal("Failed to select printer: ", err)
	}

	if err := printpreview.WaitForPrintPreview(tconn)(ctx); err != nil {
		s.Fatal("Failed to wait for Print Preview: ", err)
	}

	if err := printpreview.Print(ctx, tconn); err != nil {
		s.Fatal("Failed to print: ", err)
	}

	// Notification of failure should show up after trying to print.
	sslErrorMessage := "Printer SSL certificate is expired. Restart printer and try again."
	if _, err = ash.WaitForNotification(ctx, tconn, 5*time.Second, ash.WaitTitle(sslErrorMessage)); err != nil {
		s.Fatal("Failed to find notification: ", err)
	}

	// Check print job history to make sure job is marked as failed.
	if err := uiauto.Combine("Check print history for failed job",
		printManager.Focus(),
		printManager.VerifyPrintJob(),
		ui.EnsureFocused(nodewith.Name("Failed - Certificate Expired")),
	)(ctx); err != nil {
		s.Fatal("Failed to find failed print job: ", err)
	}
}
