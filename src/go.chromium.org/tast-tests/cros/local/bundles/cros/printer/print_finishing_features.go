// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package printer

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/printer/pre"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/printer/uitools"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/printpreview"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/printing/ippeveprinter"
	"go.chromium.org/tast-tests/cros/local/printing/lp"
	"go.chromium.org/tast-tests/cros/local/printing/printer"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PrintFinishingFeatures,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Tests that finishing features are available in the UI and are correctly reeceived by a printer",
		Contacts:     []string{"chromeos-commercial-printing@google.com", "project-bolton@google.com", "nedol@google.com"},
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
			"group:paper-io",
			"paper-io_printing",
		},
		// ChromeOS > Software > Commercial (Enterprise) > Printing
		BugComponent: "b:1111614",
		SoftwareDeps: []string{"chrome", "cups"},
		HardwareDeps: hwdep.D(pre.PrinterSkipUnstableModels),
		Data:         []string{"ipp_conf_finishings.txt", "get-jobs-finishings-info.test"},
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
				Timeout:           4 * time.Minute,
			},
		},
		SearchFlags: []*testing.StringPair{
			{
				Key: "feature_id",
				// Set up a printer with advanced features (COM_FOUND_CUJ17_TASK1_WF1).
				Value: "screenplay-487ce3a6-0cb5-4a33-9951-1a8780162b6b",
			},
			{
				Key: "feature_id",
				// Initiate a print job from a printer with advanced features (COM_FOUND_CUJ17_TASK2_WF1).
				Value: "screenplay-8ca42bd6-e9d2-4ce4-a493-1e2c57b9aaca",
			},
		},
	})
}

func PrintFinishingFeatures(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	bt := s.Param().(browser.Type)
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	s.Log("Installing printer")
	if err := printer.ResetCups(ctx); err != nil {
		s.Fatal("Failed to reset cupsd: ", err)
	}

	// `ipp_conf_finishings.txt` is a modified ipp attributes file fetched from
	// ippeveprinter with default parameters so that finishing printing features are supported.
	printer, err := ippeveprinter.Start(ctx,
		ippeveprinter.WithAttributesFile(s.DataPath("ipp_conf_finishings.txt")))
	if err != nil {
		s.Fatal("Failed to start ippeveprinter: ", err)
	}
	defer func(ctx context.Context) {
		if err := printer.Stop(ctx); err != nil {
			s.Fatal("Failed to stop ippeveprinter: ", err)
		}
	}(ctx)

	// Open OS Settings and navigate to the Printing page.
	ui := uiauto.New(tconn)
	if err := uitools.NavigateToPrintersSettingsPage(ctx, tconn, cr, ui); err != nil {
		s.Fatal("Failed to launch Settings page: ", err)
	}

	const printerDisplayName = "IPP Everywhere Printer"
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
	if err := uiauto.Combine("set printer details",
		ui.LeftClick(nameFinder),
		ui.EnsureFocused(nameFinder),
		kb.TypeAction(printerDisplayName),
		ui.LeftClick(addressFinder),
		ui.EnsureFocused(addressFinder),
		kb.TypeAction(fmt.Sprintf("localhost:%d", printer.Port())),
		ui.LeftClick(nodewith.Role("button").Name("Add")),
	)(ctx); err != nil {
		s.Fatal("Failed to set printer details: ", err)
	}

	// Create a browser (either ash or lacros, based on browser type).
	br, closeBrowser, err := browserfixt.SetUp(ctx, cr, bt)
	if err != nil {
		s.Fatal("Failed to launch browser: ", err)
	}
	defer closeBrowser(cleanupCtx)
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree")

	conn, err := br.NewConn(ctx, "chrome://version/")
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

	if err := printpreview.ExpandMoreSettings(ctx, tconn); err != nil {
		s.Fatal("Failed to expand more settings: ", err)
	}

	if err := printpreview.OpenAdvancedSettings(ctx, tconn); err != nil {
		s.Fatal("Failed to open Advanced Settings: ", err)
	}

	for _, advancedSetting := range []string{"Double-gate fold", "Dual punch left", "Staple top right"} {
		if err := printpreview.SetAdvancedSetting(ctx, tconn, advancedSetting, true); err != nil {
			s.Fatal("Failed to set the value of checkbox ", advancedSetting, ": ", err)
		}
	}

	if err := printpreview.CloseAdvancedSettings(ctx, tconn); err != nil {
		s.Fatal("Failed to close Advanced Settings: ", err)
	}

	if err := printpreview.Print(ctx, tconn); err != nil {
		s.Fatal("Failed to print: ", err)
	}

	printerName, err := lp.PrinterNameByURI(ctx, printer.IppURI())
	if err != nil {
		s.Fatal("Failed to find printer: ", err)
	}
	// We assume that the first print job always has ID equal to 1
	jobID := 1
	lpstatID := fmt.Sprintf("%s-%d", printerName, jobID)
	s.Log("Waiting for print job to complete")
	if err = testing.Poll(ctx, func(ctx context.Context) error {
		if done, err := lp.JobCompleted(ctx, printerName, lpstatID); err != nil {
			return testing.PollBreak(err)
		} else if !done {
			return errors.New("Print job has not completed yet")
		}
		s.Log("Print job has completed")
		return nil
	}, nil); err != nil {
		s.Fatal("Print job failed to complete: ", err)
	}

	cmd := testexec.CommandContext(ctx, "ipptool", "-tv", printer.IppURI(), s.DataPath("get-jobs-finishings-info.test"))
	stdout, _, err := cmd.SeparatedOutput()
	// ippeveprinter cleans up print jobs after 60 seconds, so we should be able to see information about the job sent in this test
	if !strings.Contains(string(stdout), "finishings (1setOf enum) = fold-double-gate,punch-dual-left,staple-top-right") {
		s.Fatal("Job description message doesn't contain info about finishing options: ", string(stdout))
	}
}
