// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"fmt"

	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/policy/printingtest"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/printpreview"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/restriction"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PrintingMaxSheetsAllowed,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Verify behaviour of PrintingMaxSheetsAllowed Policy",
		Contacts: []string{
			"chromeos-commercial-printing@google.com",
			"nedol@google.com", // Test author
		},
		// ChromeOS > Software > Commercial (Enterprise) > Printing
		BugComponent: "b:1111614",
		SoftwareDeps: []string{"chrome"},
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
			"group:hw_agnostic",
		},
		Params: []testing.Param{{
			Fixture: "virtualUsbPrinterModulesLoadedWithChromePolicyLoggedIn",
			Val:     browser.TypeAsh,
		}, {
			Name:              "lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Fixture:           "virtualUsbPrinterModulesLoadedWithLacrosPolicyLoggedIn",
			Val:               browser.TypeLacros,
		}},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.PrintingMaxSheetsAllowed{}, pci.VerifiedFunctionalityUI),
			{
				Key: "feature_id",
				// Test PrintingMaxSheetsAllowed Policy (COM_FOUND_CUJ9_TASK3_WF1).
				Value: "screenplay-af2592d2-c335-4a0b-8330-a8f494423e58",
			},
		},
	})
}

// fetchMaxSheetsAllowedFromPrintPreview assumes that the print preview dialog is open.
func fetchMaxSheetsAllowedFromPrintPreview(ctx context.Context, s *testing.State, tconn *chrome.TestConn, expectedMaxSheetsAllowed int) printingtest.SettingValues {
	ui := uiauto.New(tconn)

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get the keyboard: ", err)
	}
	defer kb.Close(ctx)

	// Print only the first page so that the number of copies is equal to the total number of pages to be printed.
	if err = printpreview.SetPages(ctx, tconn, "1"); err != nil {
		s.Fatal("Failed to select the first page to print: ", err)
	}

	verifyRestrictionForNumberOfCopies := func(numberOfCopiesToCheck int, expectedRestriction restriction.Restriction) {
		printButton := nodewith.Name("Print").Role(role.Button)
		copiesSpinButton := nodewith.Name("Copies").Role(role.SpinButton)

		setNumberOfCopies := func(numberOfCopiesToSet int) {
			if err := uiauto.Combine("set number of copies",
				ui.DoDefault(copiesSpinButton),
				kb.AccelAction("Ctrl+A"),
				kb.TypeAction(fmt.Sprint(numberOfCopiesToSet)),
			)(ctx); err != nil {
				s.Fatal("Can't set the number of copies")
			}
		}

		setNumberOfCopies(numberOfCopiesToCheck)

		if err := ui.WaitForRestriction(printButton, expectedRestriction)(ctx); err != nil {
			s.Fatalf("Expected restriction %q was never reached for %d copies: %s", expectedRestriction, numberOfCopiesToCheck, err)
		}

		// Change the number of copies back to the default value in order to enable UI elements.
		setNumberOfCopies(1)
	}

	verifyRestrictionForNumberOfCopies(expectedMaxSheetsAllowed, restriction.None)
	verifyRestrictionForNumberOfCopies(expectedMaxSheetsAllowed+1, restriction.Disabled)

	return printingtest.SettingValues{
		DefaultValue:    "",
		AvailableValues: []string{fmt.Sprint(expectedMaxSheetsAllowed)},
	}
}

func PrintingMaxSheetsAllowed(ctx context.Context, s *testing.State) {
	subtestcases := []printingtest.SubTestCase{
		{
			TestName: "100",
			// "Default" policy for the maximal number of sheets allowed to be printed
			// neither exists nor makes sense, thus it's not tested here.
			ExpectedDefaultValue: "",
			// For the sake of simplicity only the maximal number of pages is stored.
			// All smaller positive values are also available, but are not mentioned here.
			ExpectedAvailableValues: []string{"100"},
			Policies: []policy.Policy{
				&policy.PrintingMaxSheetsAllowed{Val: 100},
			},
			FetchValuesFunc: func(ctx context.Context, s *testing.State, tconn *chrome.TestConn) printingtest.SettingValues {
				return fetchMaxSheetsAllowedFromPrintPreview(ctx, s, tconn, 100 /* maxSheetsAllowed */)
			},
		},
		{
			TestName:                "1",
			ExpectedDefaultValue:    "",
			ExpectedAvailableValues: []string{"1"},
			Policies: []policy.Policy{
				&policy.PrintingMaxSheetsAllowed{Val: 1},
			},
			FetchValuesFunc: func(ctx context.Context, s *testing.State, tconn *chrome.TestConn) printingtest.SettingValues {
				return fetchMaxSheetsAllowedFromPrintPreview(ctx, s, tconn, 1 /* maxSheetsAllowed */)
			},
		},
		{
			TestName:                "9999",
			ExpectedDefaultValue:    "",
			ExpectedAvailableValues: []string{"9999"},
			Policies: []policy.Policy{
				&policy.PrintingMaxSheetsAllowed{Val: 9999},
			},
			FetchValuesFunc: func(ctx context.Context, s *testing.State, tconn *chrome.TestConn) printingtest.SettingValues {
				return fetchMaxSheetsAllowedFromPrintPreview(ctx, s, tconn, 9999 /* maxSheetsAllowed */)
			},
		},
	}

	printingtest.RunFeatureRestrictionTest(ctx, s, subtestcases, nil /* printerAttributesFilePath */)
}
