// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package printer

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/printer/pre"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/checked"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/printmanagementapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/printpreview"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/restriction"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/printing/usbprinter"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         BorderlessPrint,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "End-to-end tests for borderless printing with a virtual USB printer",
		Contacts:     []string{"project-bolton@google.com", "bryancain@google.com"},
		Attr: []string{
			"group:mainline",
			"informational",
			"group:paper-io",
			"paper-io_printing",
		},
		// ChromeOS > Platform > Services > Printing
		BugComponent: "b:167231",
		SoftwareDeps: []string{"chrome", "cups"},
		HardwareDeps: hwdep.D(pre.PrinterSkipUnstableModels),
		Data:         []string{"borderless_attributes_script.textproto"},
		Params: []testing.Param{
			{
				Val:     browser.TypeAsh,
				Fixture: "virtualUsbPrinterModulesLoaded",
			},
			{
				Name:              "lacros",
				Val:               browser.TypeLacros,
				ExtraSoftwareDeps: []string{"lacros"},
				Fixture:           "virtualUsbPrinterModulesLoaded",
			},
		},
	})
}

func BorderlessPrint(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	bt := s.Param().(browser.Type)
	opts := []chrome.Option{chrome.EnableFeatures("EnableBorderlessPrinting")}
	lacrosConfig := lacrosfixt.NewConfig(lacrosfixt.ChromeOptions(chrome.LacrosEnableFeatures("EnableBorderlessPrinting")))
	cr, err := browserfixt.NewChrome(ctx, bt, lacrosConfig, opts...)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(cleanupCtx) // Close our own chrome instance

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
		usbprinter.WaitUntilConfigured(),
		usbprinter.WithMockPrinterScriptPath(s.DataPath("borderless_attributes_script.textproto")))
	if err != nil {
		s.Fatal("Failed to start IPP-over-USB printer: ", err)
	}
	defer func(ctx context.Context) {
		if err := printer.Stop(ctx); err != nil {
			s.Error("Failed to stop printer: ", err)
		}
	}(ctx)

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get the keyboard: ", err)
	}
	defer kb.Close(ctx)

	// Hide all notifications to prevent them from covering the printer entry.
	if err := ash.CloseNotifications(ctx, tconn); err != nil {
		s.Fatal("Failed to close all notifications: ", err)
	}

	// Launch Print Management app.
	printManagementApp, err := printmanagementapp.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch Print Management app: ", err)
	}

	// Be sure we clear the history before we start so when we check the history
	// at the end of our test it will only have print jobs from this test.
	if err := printManagementApp.ClearHistory()(ctx); err != nil {
		s.Fatal("Failed to clear printing history: ", err)
	}

	// Create a browser (either ash or lacros, based on browser type).
	br, closeBrowser, err := browserfixt.SetUp(ctx, cr, bt)
	if err != nil {
		s.Fatal("Failed to launch browser: ", err)
	}
	defer closeBrowser(cleanupCtx)

	conn, err := br.NewConn(ctx, "chrome://version/")
	if err != nil {
		s.Fatal("Failed to connect to broswer: ", err)
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
	if err := printpreview.SelectPrinter(ctx, tconn, printer.VisibleName); err != nil {
		s.Fatal("Failed to select printer: ", err)
	}

	if err := printpreview.ExpandMoreSettings(ctx, tconn); err != nil {
		s.Fatal("Failed to expand 'more settings': ", err)
	}

	// The virtual USB printer does not advertise borderless support for Letter paper, so
	// verify that the borderless option cannot be selected in the UI.
	s.Log("Setting paper size to Letter")
	if err := printpreview.SetDropdown(ctx, tconn, "Paper size", "Letter"); err != nil {
		s.Fatal("Failed to set value of dropdown")
	}
	if err := verifyBorderlessDisabledState(ctx, tconn, false, true); err != nil {
		s.Fatal("Failed to verify 'No border' checkbox state: ", err)
	}

	// The virtual USB printer advertises borderless support for A4 paper, so verify that
	// the borderless option can be selected in the UI.
	s.Log("Setting paper size to A4")
	if err := printpreview.SetDropdown(ctx, tconn, "Paper size", "A4"); err != nil {
		s.Fatal("Failed to set value of dropdown: ", err)
	}
	if err := verifyBorderlessDisabledState(ctx, tconn, false, false); err != nil {
		s.Fatal("Failed to verify 'No border' checkbox state: ", err)
	}

	// Verify that both of these types exist in the dropdown
	s.Log("Setting paper type to plain paper")
	if err := printpreview.SetDropdown(ctx, tconn, "Paper type", "Paper (Plain)"); err != nil {
		s.Fatal("Failed to set value of dropdown")
	}

	s.Log("Setting paper type to photo paper")
	if err := printpreview.SetDropdown(ctx, tconn, "Paper type", "Photo"); err != nil {
		s.Fatal("Failed to set value of dropdown")
	}

	if err := printpreview.SetCheckboxState(ctx, tconn, "No border", true); err != nil {
		s.Fatal("Failed to select checkbox: ", err)
	}

	// Send a print job with borderless enabled, and make sure it completes successfully.
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

	if err := uiauto.Combine("Verify print job",
		printManagementApp.Focus(),
		printManagementApp.VerifyHistoryLabel(),
		printManagementApp.VerifyPrintJob(),
	)(ctx); err != nil {
		s.Fatal("Failed to verify print job: ", err)
	}

	info, err := uiauto.New(tconn).Info(ctx, nodewith.HasClass("list-item flex-center"))
	if err != nil {
		s.Fatal("Failed to retrieve print job status: ", err)
	}
	if strings.Contains(info.Name, "Failed") {
		s.Fatal("Print job failed")
	}
}

func verifyBorderlessDisabledState(ctx context.Context, tconn *chrome.TestConn, expectedChecked, expectedDisabled bool) error {
	checkbox := nodewith.Name("No border").Role(role.CheckBox)
	ui := uiauto.New(tconn)

	// Poll for 5 seconds as an awkward way to avoid a race condition. ui.WaitUntilExists() can't be
	// used here because nodewith/uiauto doesn't support "element disabled" as a search condition.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		info, err := ui.Info(ctx, checkbox)
		if err != nil {
			return err
		}

		checked := (info.Checked == checked.True)
		disabled := (info.Restriction == restriction.Disabled)

		if checked != expectedChecked {
			return errors.Errorf("expected checked=%t, but it is %t", expectedChecked, checked)
		}
		if disabled != expectedDisabled {
			return errors.Errorf("expected disabled=%t, but it is %t", expectedDisabled, disabled)
		}

		return nil
	}, &testing.PollOptions{
		Timeout: 5 * time.Second,
		Interval: 250 * time.Millisecond,
	}); err != nil {
		return err
	}

	return nil
}
