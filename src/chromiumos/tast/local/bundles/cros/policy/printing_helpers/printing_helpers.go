// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package printinghelpers includes helper functions for printing tast tests
package printinghelpers

import (
	"context"
	"os"
	"time"

	"chromiumos/tast/common/policy"
	"chromiumos/tast/common/policy/fakedms"
	"chromiumos/tast/ctxutil"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/browser/browserfixt"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/printpreview"
	"chromiumos/tast/local/chrome/uiauto/role"
	"chromiumos/tast/local/input"
	"chromiumos/tast/local/policyutil"
	"chromiumos/tast/local/printing/printer"
	"chromiumos/tast/local/printing/usbprinter"
	"chromiumos/tast/local/strcmp"
	"chromiumos/tast/testing"
)

// SettingValues stores the default value and all available values
// of a certain print setting in print preview.
type SettingValues struct {
	DefaultValue    string
	AvailableValues []string
}

// SubTestCase is used to parameterize printer subtest case
type SubTestCase struct {
	TestName                string
	ExpectedDefaultValue    string
	ExpectedAvailableValues []string
	Policies                []policy.Policy
}

// RunFeatureRestrictionTest sets up a virtual usb printer,
// launches Chrome and checks if the default value and the available values
// of a certain setting are set as expected.
func RunFeatureRestrictionTest(
	ctx context.Context,
	s *testing.State,
	subtestcases []SubTestCase,
	// printerAttributesFilePath set to nil implies that the default printer attributes file will be used
	printerAttributesFilePath *string,
	// This function assumes that the print preview is open.
	// It then fetches and returns the default value and all available values
	// of a certain setting on the print preview window.
	fetchValuesFunc func(ctx context.Context, s *testing.State, tconn *chrome.TestConn) SettingValues) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	s.Log("Installing printer")
	if err := printer.ResetCups(ctx); err != nil {
		s.Fatal("Failed to reset cupsd: ", err)
	}

	var printerAttributesOption usbprinter.Option
	if printerAttributesFilePath != nil {
		// Use custom printer IPP attributes file if provided
		if _, err := os.Stat(*printerAttributesFilePath); err != nil {
			s.Fatal("Failed to find printer attributes file: ", err)
		}
		printerAttributesOption = usbprinter.WithAttributes(*printerAttributesFilePath)
	} else {
		// Use generic IPP attributes otherwise
		printerAttributesOption = usbprinter.WithGenericIPPAttributes()
	}

	printer, err := usbprinter.Start(ctx,
		usbprinter.WithIPPUSBDescriptors(),
		printerAttributesOption,
		usbprinter.WaitUntilConfigured())
	if err != nil {
		s.Fatal("Failed to start IPP-over-USB printer: ", err)
	}
	defer func() {
		if err := printer.Stop(ctx); err != nil {
			s.Error("Failed to stop printer: ", err)
		}
	}()

	// Find a keyboard input source.
	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get the keyboard: ", err)
	}
	defer kb.Close()

	// Connect to Test API to use it with the UI library.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	// This is the default USB printer name defined in ippusb_printer.json config file.
	const printerName = "DavieV Virtual USB Printer (USB)"
	for _, param := range subtestcases {
		s.Run(ctx, param.TestName, func(ctx context.Context, s *testing.State) {
			// Reserve ten seconds for cleanup.
			cleanupCtx := ctx
			ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
			defer cancel()

			// Perform cleanup.
			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Fatal("Failed to clean up: ", err)
			}

			// Update policies.
			if err := policyutil.ServeAndVerify(ctx, fdms, cr, param.Policies); err != nil {
				s.Fatal("Failed to update policies: ", err)
			}

			// Setup browser based on the chrome type.
			br, closeBrowser, err := browserfixt.SetUp(ctx, cr, s.Param().(browser.Type))
			if err != nil {
				s.Fatal("Failed to setup chrome: ", err)
			}
			defer closeBrowser(cleanupCtx)
			// The UI tree must be dumped before closing the browser.
			defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree_"+param.TestName)

			// Open a new tab. The print dialog fails to open when invoking CTRL+P
			// directly after calling `browserfixt.SetUp`, likely because the page
			// isn't fully loaded yet. It also fails to open on about:blank pages, but
			// works fine on chrome://newtab; see crbug.com/1290797.
			conn, err := br.NewConn(ctx, chrome.NewTabURL)
			if err != nil {
				s.Fatal("Failed to connect to chrome: ", err)
			}
			defer conn.Close()

			if err := uiauto.Combine("open Print Preview with a shortcut",
				kb.AccelAction("Ctrl+P"),
				printpreview.WaitForPrintPreview(tconn))(ctx); err != nil {
				s.Fatal("Failed to open the Print Preview: ", err)
			}
			// We should manually close the print preview, as the "chrome://print" page
			// doesn't exist, but it's stored in the context for some reason
			defer func() {
				if err := uiauto.New(tconn).DoDefault(nodewith.Role(role.Button).NameStartingWith("Cancel"))(cleanupCtx); err != nil {
					s.Error("Failed to close print preview: ", err)
				}
			}()

			if err := printpreview.SelectPrinter(ctx, tconn, printerName); err != nil {
				s.Fatal("Failed to select a printer: ", err)
			}

			settingValues := fetchValuesFunc(ctx, s, tconn)

			if param.ExpectedDefaultValue != settingValues.DefaultValue {
				s.Fatalf(
					"Unexpected default value: got %s; want %s",
					settingValues.DefaultValue,
					param.ExpectedDefaultValue)
			}

			// Compare actual and expected sets of available values.
			if diff := strcmp.SameList(param.ExpectedAvailableValues, settingValues.AvailableValues); diff != "" {
				s.Fatal("Unexpected available values (-want +got) ", diff)
			}
		})
	}
}
